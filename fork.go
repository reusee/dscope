package dscope

import (
	"cmp"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// TheoryOfScopeFork documents the semantics and typical uses of Fork.
const TheoryOfScopeFork = `
dscope fork theory:
- Fork creates a new branch of the definition lineage: a scope that contains
  every definition of the scope it was forked from, with the new definitions
  layered on top. There is no child-parent relationship between scopes: the
  original and the fork are independent branches, and forking or overriding
  in one never changes the definitions of the other.
- The innermost definition of a type is the effective one: forking a
  definition for an inherited type overrides it, and forking a type the
  original lacks adds it.
- Override triggers fine-grained recomputation: the overridden type and its
  transitive dependents re-evaluate lazily in the new scope, while untouched
  providers keep the cached values they had in the original.
- Fine-grained recomputation is sound only when providers are pure functions
  of their declared dependencies. Providers that reach into the scope
  dynamically through InjectStruct, Fork, or Reset are re-evaluated
  pessimistically whenever a Fork adds definitions.
- Typical uses: add definitions as an application boots, override a dependency
  with a mock or stub in tests, and layer environment-specific variants on a
  common base.
`

// TheoryOfScopeIdentity documents how a scope gets its identity, and why that
// identity must resist collisions.
const TheoryOfScopeIdentity = `
dscope scope identity theory:
- A scope's identity is the sorted set of the types its definitions produce,
  summarized as a 128-bit digest. Definition instances do not enter the
  identity: two scopes built from the same types have the same shape, so they
  share analysis results.
- The identity is a cache key. Two different shapes with one digest would share
  a forker and hand out wrong values, so the digest is 128 bits wide, seeded per
  process, and folds every hashed word through a bijective mixer. An adversary
  cannot compute a colliding shape without the seeds, and the seeds never leave
  the process.
- Signatures and fork keys are process-local. They are never persisted, so a
  digest does not survive a restart and must never be compared across processes.
`

const TheoryOfScopeForkFlatten = `
dscope fork flatten theory:
- Fork can be called any number of times. Repeated forking never grows the
  value stack without bound, so lookups never slow down over time.
- Each Fork appends one sorted layer onto the scope's value stack.
- A Fork whose definitions change no effective value adds no layer: the new
  scope shares the value stack of its base.
- Appending flattens the stack first when it is deeper than an internal
  threshold, and always when the base layer is a lazy reset layer, because a
  search needs a single sorted stack.
- The layer of a Fork holds the definitions the fork adds and the inherited
  types whose recomputation those definitions affect. It marks an affected type
  instead of copying it and refreshes the type on first access; the cache of
  fresh initializers comes into existence at that access. A Fork therefore pays
  neither for the size of the affected set nor for the values the new scope
  never resolves.
- Flattening is transparent: effective values and override semantics are
  preserved. Users never need to compact scopes manually.
`

// @@ai keep flat-layer lookups allocation free

const TheoryOfTypeGranularity = `
dscope type granularity theory:
- Type granularity bounds recomputation precision: the finer the type
  definitions, the narrower the recomputation scope when a value is updated.
- Prefer single-value types over composite types with multiple mutable fields.
  Splitting a composite into per-field types lets an update recompute only the
  providers that depend on the changed field, leaving consumers of the other
  fields cached.
`

// stackHeightLimit bounds the number of layers in a value stack. Fork
// flattens a deeper stack before appending new layers, so that Load never
// binary-searches an unbounded number of layers.
const stackHeightLimit = 16

// forkInlineInitializers is the number of definition initializers a Fork stores
// inside its own allocation. Most Forks add one definition, so the common case
// pays one allocation of exactly the size of its layer and its initializer.
const forkInlineInitializers = 1

// _Forker pre-calculates the information required to efficiently create a new
// scope from a base scope and new definitions. Instances are cached based on
// the base scope's signature and the types of the new definitions.
type _Forker struct {
	// NewValuesTemplate contains template _Value objects (without initializers) for the new definitions.
	NewValuesTemplate []_Value
	// DefKinds stores the reflect.Kind (Func or Ptr) for each new definition.
	DefKinds []reflect.Kind
	// DefNumValues stores the number of values produced by each function definition.
	DefNumValues []int
	// PosesAtSorted maps the original index of a value in NewValuesTemplate to its index in the sorted slice.
	PosesAtSorted []posAtSorted
	// ResetIDs lists TypeIDs (sorted) of values inherited from the base scope that need invalidation due to overrides or dependency changes.
	ResetIDs []_TypeID // sorted
	// Signature is a hash representing the structural identity of the scope *after* this fork.
	Signature   _Hash
	singleFunc1 bool
	singleFunc2 bool
}

// posAtSorted represents the index of a value within the sorted slice of new values.
type posAtSorted int

// _ForkBundle carries the layer of one Fork and the initializer of its first
// definition in one allocation. A Fork that adds more definitions than the
// bundle holds keeps all its initializers in one slice of their own, so the
// bytes a Fork pays stay proportional to the definitions it adds. An initializer
// keeps its identity as the address the layer stores, so a bundled initializer
// behaves like a separately allocated one.
type _ForkBundle struct {
	layer _StackedMap
	inits [forkInlineInitializers]_Initializer
}

type _ForkBundleReset struct {
	layer _StackedMap
	inits [forkInlineInitializers]_Initializer
	reset _ResetState
}

type _ForkBundle2 struct {
	layer _StackedMap
	vals  [2]_Value
	inits [2]_Initializer
}

type _ForkBundle2Reset struct {
	layer _StackedMap
	vals  [2]_Value
	inits [2]_Initializer
	reset _ResetState
}

type _SingleForkerAlloc struct {
	forker   _Forker
	val      [1]_Value
	typeInfo _TypeInfo
}

var (
	singleFuncDefKinds      = []reflect.Kind{reflect.Func}
	singleFuncDefNumValues  = []int{1}
	singleFuncPosesAtSorted = []posAtSorted{0}
)

type _DefOrigin struct {
	defIndex    int
	defType     reflect.Type
	outputIndex int // -1 for pointer definitions
}

func (o _DefOrigin) String() string {
	if o.outputIndex < 0 {
		return fmt.Sprintf("definition #%d (%v)", o.defIndex+1, o.defType)
	}
	return fmt.Sprintf("definition #%d (%v, output %d)", o.defIndex+1, o.defType, o.outputIndex)
}

// validateDefinition rejects malformed definitions with structured errors.
func validateDefinition(def any) {
	if def == nil {
		panic(errWith(ErrBadArgument, "nil definition"))
	}
	validateDefinitionWithType(def, reflect.TypeOf(def))
}

// validateDefinitionWithType is validateDefinition with a pre-computed
// reflect.Type, so a caller that already holds the type does not pay for
// another reflect.TypeOf. A nil type means the definition was an untyped nil:
// reflect.TypeOf returns a nil type for it. The nil-value checks still need
// reflect.ValueOf because they test the value, not the type.
func validateDefinitionWithType(def any, defType reflect.Type) {
	if defType == nil {
		panic(errWith(ErrBadArgument, "nil definition"))
	}
	switch defType.Kind() {
	case reflect.Func:
		if reflect.ValueOf(def).IsNil() {
			panic(errWith(ErrBadArgument, "%T nil function provided", def))
		}
		if defType.NumOut() == 0 {
			panic(errWith(ErrBadArgument, "%T returns nothing", def))
		}
		if defType.IsVariadic() {
			panic(errWith(ErrBadArgument, "%T is variadic, variadic provider functions are not supported", def))
		}
	case reflect.Pointer:
		if reflect.ValueOf(def).IsNil() {
			panic(errWith(ErrBadArgument, "%T nil pointer provided", def))
		}
	default:
		panic(errWith(ErrBadArgument, "%T is not a valid definition", def))
	}
}

// checkDuplicateOutput panics with ErrBadDefinition when a type produced by a
// new definition was already produced by an earlier definition in the same
// Fork call, naming both conflicting definitions.
func checkDuplicateOutput(origins map[_TypeID]_DefOrigin, t reflect.Type, id _TypeID, origin _DefOrigin) {
	if first, ok := origins[id]; ok {
		panic(errWith(ErrBadDefinition, "%v has multiple definitions in the same Fork call: %s and %s", t, first, origin))
	}
}

// checkDuplicateOutputSlice is checkDuplicateOutput with a linear scan over
// parallel slices instead of a map lookup. A Fork usually adds few definitions,
// so the scan is cheaper than hashing and bucket allocation: no map, no hash
// computation. The ids and origins slices grow together, one entry per output
// type of the definitions processed so far.
func checkDuplicateOutputSlice(ids []_TypeID, origins []_DefOrigin, t reflect.Type, id _TypeID, origin _DefOrigin) {
	for i, existing := range ids {
		if existing == id {
			panic(errWith(ErrBadDefinition, "%v has multiple definitions in the same Fork call: %s and %s", t, origins[i], origin))
		}
	}
}

// newForkBundle allocates the layer of a Fork and the initializer of its first
// definition together. base is the layer the new layer sits on and numValues the
// number of values the fork adds.
func newForkBundle(base *_StackedMap, numValues int) *_ForkBundle {
	bundle := new(_ForkBundle)
	initStackedMapLayer(&bundle.layer, base, numValues)
	return bundle
}

func newForker(
	scope Scope,
	defs []any,
) *_Forker {
	return newForkerWithTypes(scope, defs, nil)
}

// newForkerWithTypes is newForker with pre-computed definition types, so a
// Fork that already collected the types of its definitions does not pay for
// reflect.TypeOf a second time. A nil defTypes makes it collect the types
// itself, preserving the newForker call contract.
func newForkerWithTypes(
	scope Scope,
	defs []any,
	defTypes []reflect.Type,
) *_Forker {

	// Fast path for a single 0-arg, 1-output function definition on an empty base scope.
	if scope.values == nil && len(defs) == 1 {
		var defType reflect.Type
		if defTypes != nil {
			defType = defTypes[0]
		} else {
			defType = reflect.TypeOf(defs[0])
		}
		if defType.Kind() == reflect.Func && defType.NumIn() == 0 && defType.NumOut() == 1 {
			defID := getTypeID(defType)
			outID := getTypeID(defType.Out(0))
			h0 := hashWord(hashWord(hashSeeds[0], 1), uint64(defID))
			h1 := hashWord(hashWord(hashSeeds[1], 1), uint64(defID))
			alloc := new(_SingleForkerAlloc)
			alloc.typeInfo.DefType = defType
			alloc.typeInfo.DefTypeID = defID
			alloc.val[0] = _Value{
				id:       outID,
				typeInfo: &alloc.typeInfo,
			}
			f := &alloc.forker
			f.Signature = _Hash{h0, h1}
			f.NewValuesTemplate = alloc.val[:]
			f.DefKinds = singleFuncDefKinds
			f.DefNumValues = singleFuncDefNumValues
			f.PosesAtSorted = singleFuncPosesAtSorted
			f.singleFunc1 = true
			return f
		}
	}

	if defTypes == nil {
		defTypes = make([]reflect.Type, len(defs))
		for i, def := range defs {
			defTypes[i] = reflect.TypeOf(def)
		}
	}

	// 1. Process Definitions: Create templates, store metadata, identify overrides.
	newValuesTemplate := make([]_Value, 0, len(defs))
	var redefinedIDs []_TypeID // Sorted set of overridden TypeIDs
	var originIDsBuf [8]_TypeID
	var originsBuf [8]_DefOrigin
	newDefOrigins := originIDsBuf[:0]
	origins := originsBuf[:0]
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for defIdx := range defs {
		defType := defTypes[defIdx]
		defTypeID := getTypeID(defType)
		defKinds = append(defKinds, defType.Kind())

		switch defType.Kind() {
		case reflect.Func:

			// Extract Dependencies
			numIn := defType.NumIn()
			var dependencies []_TypeID
			if numIn > 0 {
				dependencies = make([]_TypeID, numIn)
				for i := range numIn {
					dependencies[i] = getTypeID(defType.In(i))
				}
			}

			// Create Value Templates for Outputs
			numOut := defType.NumOut()
			var numValues int
			for i := range numOut {
				t := defType.Out(i)
				id := getTypeID(t)
				origin := _DefOrigin{defIndex: defIdx, defType: defType, outputIndex: i}
				checkDuplicateOutputSlice(newDefOrigins, origins, t, id, origin)

				newValuesTemplate = append(newValuesTemplate, _Value{
					id: id,
					typeInfo: &_TypeInfo{
						DefType:      defType,
						DefTypeID:    defTypeID,
						Position:     i,
						Dependencies: dependencies,
					},
				})
				numValues++
				newDefOrigins = append(newDefOrigins, id)
				origins = append(origins, origin)
				if _, ok := scope.values.Load(id); ok {
					redefinedIDs = append(redefinedIDs, id) // Mark override
				}
			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Pointer:

			// Create Value Template
			t := defType.Elem()
			id := getTypeID(t)
			origin := _DefOrigin{defIndex: defIdx, defType: defType, outputIndex: -1}
			checkDuplicateOutputSlice(newDefOrigins, origins, t, id, origin)

			newValuesTemplate = append(newValuesTemplate, _Value{
				id: id,
				typeInfo: &_TypeInfo{
					DefType:   defType,
					DefTypeID: defTypeID,
				},
			})
			newDefOrigins = append(newDefOrigins, id)
			origins = append(origins, origin)
			if _, ok := scope.values.Load(id); ok {
				redefinedIDs = append(redefinedIDs, id) // Mark override
			}
			defNumValues = append(defNumValues, 1)

		default:
			panic("impossible")
		}
	}
	if len(redefinedIDs) > 1 {
		slices.Sort(redefinedIDs)
	}

	// 2. Sort New Values & Create Index Mapping:
	posesAtSorted := make([]posAtSorted, len(newValuesTemplate))
	var sortedNewValuesTemplate []_Value
	if len(newValuesTemplate) <= 1 {
		sortedNewValuesTemplate = newValuesTemplate
	} else {
		type posAtTemplate int
		posesAtTemplate := make([]posAtTemplate, len(newValuesTemplate))
		for i := range newValuesTemplate {
			posesAtTemplate[i] = posAtTemplate(i)
		}
		slices.SortFunc(posesAtTemplate, func(a, b posAtTemplate) int {
			return cmp.Compare(
				newValuesTemplate[a].id,
				newValuesTemplate[b].id,
			)
		})
		sortedNewValuesTemplate = make([]_Value, len(posesAtTemplate))
		for i, j := range posesAtTemplate {
			posesAtSorted[j] = posAtSorted(i)
			sortedNewValuesTemplate[i] = newValuesTemplate[j]
		}
	}

	// 3. Build Conceptual Next Scope & Analyze Dependencies via DFS:
	type traversalState struct {
		color int
		reset bool
	}
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)
	states := make(map[_TypeID]traversalState, len(newValuesTemplate))

	var path []_TypeID
	var traverse func(value _Value) (reset bool, err error)
	traverse = func(value _Value) (reset bool, err error) {
		id := value.id

		state := states[id]
		switch state.color {

		case 1: // Gray: Loop detected
			buf := new(strings.Builder)
			for _, pathID := range path {
				buf.WriteString(typeIDToType(pathID).String())
				buf.WriteString(" -> ")
			}
			buf.WriteString(typeIDToType(id).String())
			return false, errors.Join(
				fmt.Errorf("found dependency loop in definition %v", value.typeInfo.DefType),
				ErrDependencyLoop,
				fmt.Errorf("path: %s", buf.String()),
			)

		case 2: // Black: Already processed
			return state.reset, nil
		}

		states[id] = traversalState{color: 1} // Mark as visiting (Gray)

		if containsID(redefinedIDs, id) {
			reset = true
		}

		path = append(path, id)
		for _, depID := range value.typeInfo.Dependencies {
			if isAlwaysProvided(depID) {
				if len(newValuesTemplate) > 0 {
					reset = true
				}
				continue
			}
			depValue, ok := valuesTemplate.Load(depID)
			if !ok {
				path = path[:len(path)-1]
				return false, errWith(ErrDependencyNotFound, "dependency not found in definition %v, no definition for %v", value.typeInfo.DefType, typeIDToType(depID))
			}
			depResets, err := traverse(depValue)
			if err != nil {
				path = path[:len(path)-1]
				return false, err
			}
			reset = reset || depResets
		}
		path = path[:len(path)-1]

		states[id] = traversalState{color: 2, reset: reset}
		return
	}

	// 4. Analyze All Types in Conceptual Scope:
	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len())

	for value := range valuesTemplate.IterValues() {
		if _, err := traverse(value); err != nil {
			panic(err)
		}
		defID := value.typeInfo.DefTypeID
		if defID == 0 {
			defID = getTypeID(value.typeInfo.DefType)
		}
		defTypeIDs = append(defTypeIDs, defID)
	}
	slices.Sort(defTypeIDs)
	defTypeIDs = slices.Compact(defTypeIDs)

	// 5. Calculate the New Scope Signature: Hash sorted definition type IDs.
	signature := hashTypeIDs(defTypeIDs)

	// 6. Identify Values Requiring Reset: Collect TypeIDs that need reset AND existed in the base scope.
	var resetIDs []_TypeID
	if scope.values != nil {
		for id, state := range states {
			if !state.reset {
				continue
			}
			if containsID(redefinedIDs, id) {
				continue
			}
			if _, ok := scope.values.Load(id); ok {
				resetIDs = append(resetIDs, id)
			}
		}
		slices.Sort(resetIDs)
	}

	singleFunc1 := len(defKinds) == 1 && len(newValuesTemplate) == 1 && defKinds[0] == reflect.Func
	singleFunc2 := len(defKinds) == 2 && len(newValuesTemplate) == 2 && defKinds[0] == reflect.Func && defKinds[1] == reflect.Func

	// 7. Return the completed _Forker.
	return &_Forker{
		Signature:         signature,
		NewValuesTemplate: newValuesTemplate,
		DefKinds:          defKinds,
		DefNumValues:      defNumValues,
		PosesAtSorted:     posesAtSorted,
		ResetIDs:          resetIDs,
		singleFunc1:       singleFunc1,
		singleFunc2:       singleFunc2,
	}
}

// Fork applies the pre-calculated changes from the _Forker to a base scope, creating a new scope.
func (f *_Forker) Fork(s Scope, defs []any) Scope {

	// 1. Start from the base scope's value stack.
	scope := Scope{
		signature: f.Signature,
		values:    s.values,
	}
	if len(f.NewValuesTemplate) == 0 && len(f.ResetIDs) == 0 {
		return scope
	}
	base := scope.values
	if base != nil && (base.isLazyReset() || base.Height > stackHeightLimit) {
		base = base.flatten()
	}

	if f.singleFunc1 {
		if len(f.ResetIDs) == 0 {
			bundle := new(_ForkBundle)
			layer := &bundle.layer
			if base != nil {
				layer.Next = base
				layer.Height = base.Height + 1
			} else {
				layer.Height = 1
			}
			bundle.inits[0].Def = defs[0]
			v := f.NewValuesTemplate[0]
			v.initializer = &bundle.inits[0]
			layer.inline[0] = v
			layer.Values = layer.inline[:1]
			scope.values = layer
			return scope
		}
		bundle := new(_ForkBundleReset)
		layer := &bundle.layer
		if base != nil {
			layer.Next = base
			layer.Height = base.Height + 1
		} else {
			layer.Height = 1
		}
		bundle.inits[0].Def = defs[0]
		v := f.NewValuesTemplate[0]
		v.initializer = &bundle.inits[0]
		layer.inline[0] = v
		layer.Values = layer.inline[:1]
		bundle.reset.ids = f.ResetIDs
		layer.reset = &bundle.reset
		scope.values = layer
		return scope
	}

	if f.singleFunc2 {
		if len(f.ResetIDs) == 0 {
			bundle := new(_ForkBundle2)
			layer := &bundle.layer
			if base != nil {
				layer.Next = base
				layer.Height = base.Height + 1
			} else {
				layer.Height = 1
			}
			bundle.inits[0].Def = defs[0]
			bundle.inits[1].Def = defs[1]
			v0 := f.NewValuesTemplate[0]
			v0.initializer = &bundle.inits[0]
			v1 := f.NewValuesTemplate[1]
			v1.initializer = &bundle.inits[1]
			if f.PosesAtSorted[0] == 0 {
				bundle.vals[0] = v0
				bundle.vals[1] = v1
			} else {
				bundle.vals[0] = v1
				bundle.vals[1] = v0
			}
			layer.Values = bundle.vals[:]
			scope.values = layer
			return scope
		}
		bundle := new(_ForkBundle2Reset)
		layer := &bundle.layer
		if base != nil {
			layer.Next = base
			layer.Height = base.Height + 1
		} else {
			layer.Height = 1
		}
		bundle.inits[0].Def = defs[0]
		bundle.inits[1].Def = defs[1]
		v0 := f.NewValuesTemplate[0]
		v0.initializer = &bundle.inits[0]
		v1 := f.NewValuesTemplate[1]
		v1.initializer = &bundle.inits[1]
		if f.PosesAtSorted[0] == 0 {
			bundle.vals[0] = v0
			bundle.vals[1] = v1
		} else {
			bundle.vals[0] = v1
			bundle.vals[1] = v0
		}
		layer.Values = bundle.vals[:]
		bundle.reset.ids = f.ResetIDs
		layer.reset = &bundle.reset
		scope.values = layer
		return scope
	}

	var layer *_StackedMap
	var initializers []_Initializer
	if len(f.DefKinds) <= forkInlineInitializers {
		bundle := newForkBundle(base, len(f.NewValuesTemplate))
		layer = &bundle.layer
		initializers = bundle.inits[:]
	} else {
		layer = newStackedMapLayer(base, len(f.NewValuesTemplate))
		initializers = make([]_Initializer, len(f.DefKinds))
	}

	valueIdx := 0
	for defIdx, def := range defs {
		initializer := &initializers[defIdx]
		switch f.DefKinds[defIdx] {
		case reflect.Func:
			initInitializer(initializer, def, false)
			for range f.DefNumValues[defIdx] {
				template := f.NewValuesTemplate[valueIdx]
				template.initializer = initializer
				layer.Values[f.PosesAtSorted[valueIdx]] = template
				valueIdx++
			}
		case reflect.Pointer:
			initInitializer(initializer, def, true)
			template := f.NewValuesTemplate[valueIdx]
			template.initializer = initializer
			layer.Values[f.PosesAtSorted[valueIdx]] = template
			valueIdx++
		}
	}

	layer.buildIndex()

	if len(f.ResetIDs) > 0 {
		layer.markRefresh(f.ResetIDs)
	}

	scope.values = layer
	return scope
}
