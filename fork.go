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
  pessimistically whenever a Fork adds definitions; providers that read
  external state must be re-run with Reset instead.
- Typical uses: add definitions as an application boots, override a dependency
  with a mock or stub in tests, and layer environment-specific variants on a
  common base.
`

const TheoryOfScopeForkFlatten = `
dscope fork flatten theory:
- Fork can be called any number of times. Repeated forking never grows the
  value stack without bound, so lookups never slow down over time.
- Each Fork appends one sorted layer onto the scope's value stack. Unbounded
  layering would degrade lookups, because every lookup binary-searches each
  layer.
- A Fork whose definitions change no effective value adds no layer at all: the
  new scope shares the value stack of its base.
- Appending flattens the stack first when it is deeper than an internal
  threshold, and always when the base layer is a lazy reset layer, because
  binary search needs a single sorted stack.
- The layer of a Fork holds the definitions the fork adds and the inherited
  types whose recomputation those definitions affect. It marks an affected type
  instead of copying it and refreshes the type on first access, so a Fork pays
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
	Signature _Hash
}

// posAtSorted represents the index of a value within the sorted slice of new values.
type posAtSorted int

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
	defValue := reflect.ValueOf(def)
	defType := defValue.Type()
	switch defType.Kind() {
	case reflect.Func:
		if defValue.IsNil() {
			panic(errWith(ErrBadArgument, "%T nil function provided", def))
		}
		if defType.NumOut() == 0 {
			panic(errWith(ErrBadArgument, "%T returns nothing", def))
		}
		if defType.IsVariadic() {
			panic(errWith(ErrBadArgument, "%T is variadic, variadic provider functions are not supported", def))
		}
	case reflect.Pointer:
		if defValue.IsNil() {
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

func newForker(
	scope Scope,
	defs []any,
) *_Forker {

	// Scope.Fork validates every definition before this function runs, so each
	// def here is a non-nil function or pointer.

	// 1. Process Definitions: Create templates, store metadata, identify overrides.
	newValuesTemplate := make([]_Value, 0, len(defs))
	redefinedIDs := make(map[_TypeID]struct{}, len(defs))      // Set of overridden TypeIDs
	newDefOutputIDs := make(map[_TypeID]_DefOrigin, len(defs)) // TypeID -> origin of the first definition producing it
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for defIdx, def := range defs {
		defType := reflect.TypeOf(def)
		defKinds = append(defKinds, defType.Kind())

		switch defType.Kind() {
		case reflect.Func:

			// Extract Dependencies
			numIn := defType.NumIn()
			dependencies := make([]_TypeID, 0, numIn)
			for i := range numIn {
				inType := defType.In(i)
				dependencies = append(dependencies, getTypeID(inType))
			}

			// Create Value Templates for Outputs
			numOut := defType.NumOut()
			var numValues int
			for i := range numOut {
				t := defType.Out(i)
				id := getTypeID(t)
				origin := _DefOrigin{defIndex: defIdx, defType: defType, outputIndex: i}
				checkDuplicateOutput(newDefOutputIDs, t, id, origin)

				newValuesTemplate = append(newValuesTemplate, _Value{
					typeInfo: &_TypeInfo{
						TypeID:       id,
						DefType:      defType,
						Position:     i,
						Dependencies: dependencies,
					},
				})
				numValues++
				newDefOutputIDs[id] = origin
				if _, ok := scope.values.Load(id); ok {
					redefinedIDs[id] = struct{}{} // Mark override
				}
			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Pointer:

			// Create Value Template
			t := defType.Elem()
			id := getTypeID(t)
			origin := _DefOrigin{defIndex: defIdx, defType: defType, outputIndex: -1}
			checkDuplicateOutput(newDefOutputIDs, t, id, origin)

			newValuesTemplate = append(newValuesTemplate, _Value{
				typeInfo: &_TypeInfo{
					TypeID:  id,
					DefType: defType,
				},
			})
			newDefOutputIDs[id] = origin
			if _, ok := scope.values.Load(id); ok {
				redefinedIDs[id] = struct{}{} // Mark override
			}
			defNumValues = append(defNumValues, 1)

		default:
			panic("impossible")
		}
	}

	// 2. Sort New Values & Create Index Mapping:
	type posAtTemplate int
	posesAtTemplate := make([]posAtTemplate, 0, len(newValuesTemplate))
	for i := range newValuesTemplate {
		posesAtTemplate = append(posesAtTemplate, posAtTemplate(i))
	}
	slices.SortFunc(posesAtTemplate, func(a, b posAtTemplate) int {
		return cmp.Compare(
			newValuesTemplate[a].typeInfo.TypeID,
			newValuesTemplate[b].typeInfo.TypeID,
		)
	})
	// Type IDs are unique within a Fork call, so the sorted index order is
	// unambiguous: the sorted template is built from it directly instead of
	// sorting a second copy.
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	sortedNewValuesTemplate := make([]_Value, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i) // posesAtSorted[original_index] = sorted_index
		sortedNewValuesTemplate[i] = newValuesTemplate[j]
	}

	// 3. Build Conceptual Next Scope & Analyze Dependencies via DFS:
	//    - `valuesTemplate`: Temporary _StackedMap representing the potential new scope.
	//    - Detect loops (`color`: 0=White, 1=Gray, 2=Black).
	//    - Determine which types need reset (`reset`).
	type traversalState struct {
		color int
		reset bool
	}
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)
	// One map memoizes the colour and the reset requirement of every type, so a
	// step of the analysis pays one lookup instead of two.
	states := make(map[_TypeID]traversalState, len(newValuesTemplate))

	// path is the ancestor chain of the node being traversed, held in one stack
	// that grows and shrinks with the recursion. It is read only when a loop is
	// reported, so a traversal without loops allocates nothing for it.
	var path []_TypeID
	var traverse func(value _Value) (reset bool, err error)
	traverse = func(value _Value) (reset bool, err error) {
		id := value.typeInfo.TypeID

		// Cycle Detection & Memoization
		state := states[id]
		switch state.color {

		case 1: // Gray: Loop detected
			// The reported path must close the loop: the gray node is
			// revisited here, so repeat it after the ancestor chain.
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

		// Base Case: Check if directly redefined in this fork
		if _, ok := redefinedIDs[id]; ok {
			reset = true
		}

		// Recursive Step: Check Dependencies
		path = append(path, id)
		for _, depID := range value.typeInfo.Dependencies {
			if isAlwaysProvided(depID) {
				// InjectStruct, Fork, and Reset are opaque dependencies: a
				// provider receiving one of them can dynamically pull any type
				// from the scope (InjectStruct injects struct fields, Fork
				// creates new scopes, Reset creates reset scopes). When new
				// definitions are added we must pessimistically assume the
				// opaque dependency depends on them and force a reset so the
				// provider is re-evaluated against the new scope.
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
			reset = reset || depResets // Propagate reset requirement
		}
		path = path[:len(path)-1]

		states[id] = traversalState{color: 2, reset: reset}
		return
	}

	// 4. Analyze All Types in Conceptual Scope:
	//    - Populate `states` and detect loops globally via `traverse`.
	//    - Collect `defTypeIDs` for signature.
	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len()) // For signature

	for value := range valuesTemplate.IterValues() {
		if _, err := traverse(value); err != nil {
			panic(err)
		}

		// Collect definition type IDs
		defTypeIDs = append(defTypeIDs, getTypeID(value.typeInfo.DefType))
	}
	// A type ID repeats when several outputs of one definition share the type,
	// so the collected IDs are sorted and deduplicated in one pass.
	slices.Sort(defTypeIDs)
	defTypeIDs = slices.Compact(defTypeIDs)

	// 5. Calculate the New Scope Signature: Hash sorted definition type IDs.
	signature := hashTypeIDs(nil, defTypeIDs)

	// 6. Identify Values Requiring Reset: Collect TypeIDs that need reset AND existed in the base scope.
	resetIDs := make([]_TypeID, 0, len(states))
	for id, state := range states {
		if !state.reset {
			continue
		}
		if _, ok := redefinedIDs[id]; ok {
			continue
		}
		if _, ok := scope.values.Load(id); ok { // Only reset inherited values
			resetIDs = append(resetIDs, id)
		}
	}
	slices.Sort(resetIDs)

	// 7. Return the completed _Forker.
	return &_Forker{
		Signature:         signature,
		NewValuesTemplate: newValuesTemplate,
		DefKinds:          defKinds,
		DefNumValues:      defNumValues,
		PosesAtSorted:     posesAtSorted,
		ResetIDs:          resetIDs,
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
		// A fork that changes no effective value adds no layer: the fork and
		// its base share every value and its initializer.
		return scope
	}
	if scope.values != nil && scope.values.Height > stackHeightLimit {
		scope.values = scope.values.flatten()
	}

	// 2. Create the values of this fork: Instantiate initializers and values.
	newValues := make([]_Value, len(f.NewValuesTemplate))
	valueIdx := 0
	for defIdx, def := range defs {
		kind := f.DefKinds[defIdx]

		switch kind {
		case reflect.Func:
			initializer := newInitializer(def, false)
			numValues := f.DefNumValues[defIdx]
			for range numValues {
				template := f.NewValuesTemplate[valueIdx]
				sortedIdx := f.PosesAtSorted[valueIdx]
				newValues[sortedIdx] = _Value{
					typeInfo:    template.typeInfo,
					initializer: initializer, // Share initializer for multi-return
				}
				valueIdx++
			}
		case reflect.Pointer:
			initializer := newInitializer(def, true)
			template := f.NewValuesTemplate[valueIdx]
			sortedIdx := f.PosesAtSorted[valueIdx]
			newValues[sortedIdx] = _Value{
				typeInfo:    template.typeInfo,
				initializer: initializer,
			}
			valueIdx++
		}
	}

	// 3. Add the single layer of this fork: it carries the overrides and the new
	//    definitions together with the inherited types whose recomputation they
	//    affect. A marked type holds no value in the layer and gets a fresh
	//    initializer on first access, so the cost of a Fork does not grow with
	//    the dependent set.
	if len(f.ResetIDs) > 0 {
		scope.values = scope.values.AppendRefresh(newValues, f.ResetIDs)
	} else {
		scope.values = scope.values.Append(newValues)
	}

	return scope
}
