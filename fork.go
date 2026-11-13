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

const TheoryOfScopeIdentity = `
dscope scope identity theory:
- A scope's identity is its sorted set of effective output mappings, each
  containing the output type, its provider definition type, and the output
  position, summarized as a 128-bit digest. Definition instances do not enter
  the identity: two scopes with the same mappings have the same shape, so they
  share analysis results. Partial overrides can leave different outputs of one
  multi-output provider effective, so definition types alone are not a shape.
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

const (
	stackHeightLimit       = 16
	forkInlineInitializers = 1
)

const (
	forkerKindGeneral uint8 = iota
	forkerKindEmpty
	forkerKindSingleFunc1
	forkerKindSingleFunc2
)

type _DefSpec struct {
	numValues uint16
	isPointer bool
}

type _ForkerMulti struct {
	NewValuesTemplate []_Value
	DefSpecs          []_DefSpec
	PosesAtSorted     []posAtSorted
}

type _Forker struct {
	val0TypeInfo *_TypeInfo
	val1TypeInfo *_TypeInfo
	multi        *_ForkerMulti
	ResetIDs     []_TypeID
	Signature    _Hash
	val0ID       _TypeID
	val1ID       _TypeID
	kind         uint8
	swap2        bool
}

// posAtSorted represents the index of a value within the sorted slice of new values.
type posAtSorted int

// _ForkBundle places layer at offset 0 so returning &bundle.layer needs no
// pointer offset arithmetic.
type _ForkBundle struct {
	layer _StackedMap
	init0 _Initializer
}

type _ForkBundleReset struct {
	layer _StackedMap
	init0 _Initializer
	extra _StackedMapExtra
	reset _ResetState
}

type _ForkBundle2 struct {
	layer _StackedMap
	extra _StackedMapExtra
	val1  [1]_Value
	inits [2]_Initializer
}

type _ForkBundle2Reset struct {
	layer _StackedMap
	extra _StackedMapExtra
	reset _ResetState
	val1  [1]_Value
	inits [2]_Initializer
}

type _DefOrigin struct {
	defIndex    int
	defType     reflect.Type
	outputIndex int
}

func (o _DefOrigin) String() string {
	if o.outputIndex < 0 {
		return fmt.Sprintf("definition #%d (%v)", o.defIndex+1, o.defType)
	}
	return fmt.Sprintf("definition #%d (%v, output %d)", o.defIndex+1, o.defType, o.outputIndex)
}

func throwNilDef(def any, isPtr bool) {
	if isPtr {
		panic(errWith(ErrBadArgument, "%T nil pointer provided", def))
	}
	panic(errWith(ErrBadArgument, "%T nil function provided", def))
}

func validateDefinition(def any) {
	if def == nil {
		panic(errWith(ErrBadArgument, "nil definition"))
	}
	validateDefinitionWithType(def, reflect.TypeOf(def))
}

func validateDefinitionEntry(def any, entry *_TypeIDCacheEntry) {
	flag := uint8(entry.defTypeFlag.Load())
	if flag == defTypeValidFunc {
		if reflect.ValueOf(def).IsNil() {
			throwNilDef(def, false)
		}
		return
	}
	if flag == defTypeValidPtr {
		meta := getOrBuildDefMeta(entry)
		if def == meta.nilPtr {
			throwNilDef(def, true)
		}
		return
	}
	validateDefinitionWithTypeSlow(def, entry)
}

func validateDefinitionWithType(def any, defType reflect.Type) {
	if defType == nil {
		panic(errWith(ErrBadArgument, "nil definition"))
	}
	validateDefinitionEntry(def, getTypeEntry(defType))
}

func validateDefinitionWithTypeSlow(def any, entry *_TypeIDCacheEntry) {
	defType := entry.typ
	switch defType.Kind() {
	case reflect.Func:
		if reflect.ValueOf(def).IsNil() {
			throwNilDef(def, false)
		}
		if defType.NumOut() == 0 {
			entry.defTypeFlag.Store(uint32(defTypeInvalid))
			panic(errWith(ErrBadArgument, "%T returns nothing", def))
		}
		if defType.IsVariadic() {
			entry.defTypeFlag.Store(uint32(defTypeInvalid))
			panic(errWith(ErrBadArgument, "%T is variadic, variadic provider functions are not supported", def))
		}
		entry.defTypeFlag.Store(uint32(defTypeValidFunc))
	case reflect.Pointer:
		if reflect.ValueOf(def).IsNil() {
			throwNilDef(def, true)
		}
		entry.defTypeFlag.Store(uint32(defTypeValidPtr))
	default:
		entry.defTypeFlag.Store(uint32(defTypeInvalid))
		panic(errWith(ErrBadArgument, "%T is not a valid definition", def))
	}
}

func getOrBuildDefMeta(entry *_TypeIDCacheEntry) *_DefTypeMeta {
	if m := entry.defMeta.Load(); m != nil {
		return m
	}
	return buildDefMetaSlow(entry)
}

func buildDefMetaSlow(entry *_TypeIDCacheEntry) *_DefTypeMeta {
	defType := entry.typ
	defID := entry.id
	meta := new(_DefTypeMeta)
	switch defType.Kind() {
	case reflect.Func:
		numIn := defType.NumIn()
		var deps []_TypeID
		if numIn > 0 {
			deps = make([]_TypeID, numIn)
			for i := range numIn {
				deps[i] = getTypeID(defType.In(i))
			}
		}
		numOut := defType.NumOut()
		vals := make([]_Value, numOut)
		infos := make([]_TypeInfo, numOut)
		for i := range numOut {
			outID := getTypeID(defType.Out(i))
			infos[i] = _TypeInfo{
				DefType:      defType,
				DefTypeID:    defID,
				Position:     i,
				Dependencies: deps,
			}
			vals[i] = _Value{
				id:       outID,
				typeInfo: &infos[i],
			}
		}
		meta.values = vals
		if numIn == 0 && numOut == 1 {
			meta.singleFunc1 = true
			outID := uint64(vals[0].id)
			dID := uint64(defID)
			meta.emptySig = _Hash{
				hashWord(hashWord(hashWord(hashWord(hashSeeds[0], 3), outID), dID), 0),
				hashWord(hashWord(hashWord(hashWord(hashSeeds[1], 3), outID), dID), 0),
			}
		}
	case reflect.Pointer:
		meta.isPointer = true
		meta.nilPtr = reflect.Zero(defType).Interface()
		outID := getTypeID(defType.Elem())
		info := &_TypeInfo{
			DefType:   defType,
			DefTypeID: defID,
		}
		meta.values = []_Value{{
			id:       outID,
			typeInfo: info,
		}}
	}
	if entry.defMeta.CompareAndSwap(nil, meta) {
		return meta
	}
	return entry.defMeta.Load()
}

func checkDuplicateOutputSlice(ids []_TypeID, origins []_DefOrigin, t reflect.Type, id _TypeID, origin _DefOrigin) {
	for i, existing := range ids {
		if existing == id {
			panic(errWith(ErrBadDefinition, "%v has multiple definitions in the same Fork call: %s and %s", t, origins[i], origin))
		}
	}
}

func newForkBundle(base *_StackedMap, numValues int) *_ForkBundle {
	bundle := new(_ForkBundle)
	initStackedMapLayer(&bundle.layer, base, numValues)
	return bundle
}

func newForker(scope Scope, defs []any) *_Forker {
	return newForkerWithTypes(scope, defs, nil)
}

func dependencyLoopPath(path []_TypeID, id _TypeID) string {
	start := 0
	for i, pathID := range path {
		if pathID == id {
			start = i
			break
		}
	}
	buf := new(strings.Builder)
	for _, pathID := range path[start:] {
		buf.WriteString(typeIDToType(pathID).String())
		buf.WriteString(" -> ")
	}
	buf.WriteString(typeIDToType(id).String())
	return buf.String()
}

func scopeSignatureSorted(values []_Value) _Hash {
	h0, h1 := hashSeeds[0], hashSeeds[1]
	nWords := uint64(3 * len(values))
	h0 = hashWord(h0, nWords)
	h1 = hashWord(h1, nWords)
	for _, value := range values {
		defID := value.typeInfo.DefTypeID
		if defID == 0 {
			defID = getTypeID(value.typeInfo.DefType)
		}
		idWord := uint64(value.id)
		defWord := uint64(defID)
		posWord := uint64(_TypeID(value.typeInfo.Position))
		h0 = hashWord(hashWord(hashWord(h0, idWord), defWord), posWord)
		h1 = hashWord(hashWord(hashWord(h1, idWord), defWord), posWord)
	}
	return _Hash{h0, h1}
}

func scopeSignature(values []_Value) _Hash {
	slices.SortFunc(values, func(a, b _Value) int {
		return cmp.Compare(a.id, b.id)
	})
	return scopeSignatureSorted(values)
}

func newForkerWithTypes(scope Scope, defs []any, defTypes []reflect.Type) *_Forker {
	if scope.values == nil && len(defs) == 1 {
		var t0 reflect.Type
		if defTypes != nil {
			t0 = defTypes[0]
		} else {
			t0 = reflect.TypeOf(defs[0])
		}
		e0 := getTypeEntry(t0)
		meta := getOrBuildDefMeta(e0)
		if meta.singleFunc1 {
			return &_Forker{
				val0TypeInfo: meta.values[0].typeInfo,
				Signature:    meta.emptySig,
				val0ID:       meta.values[0].id,
				kind:         forkerKindSingleFunc1,
			}
		}
	}

	var entriesBuf [8]*_TypeIDCacheEntry
	var entries []*_TypeIDCacheEntry
	if defTypes != nil {
		if len(defTypes) <= len(entriesBuf) {
			entries = entriesBuf[:len(defTypes)]
		} else {
			entries = make([]*_TypeIDCacheEntry, len(defTypes))
		}
		for i, t := range defTypes {
			entries[i] = getTypeEntry(t)
		}
	} else {
		if len(defs) <= len(entriesBuf) {
			entries = entriesBuf[:len(defs)]
		} else {
			entries = make([]*_TypeIDCacheEntry, len(defs))
		}
		for i, def := range defs {
			entries[i] = getTypeEntry(reflect.TypeOf(def))
		}
	}
	return newForkerWithEntries(scope, entries)
}

func collectEffectiveMeta(base *_StackedMap, sortedNew []_Value, dst []_Value) ([]_Value, _TypeID) {
	var maxID _TypeID
	dst = append(dst, sortedNew...)
	for _, v := range sortedNew {
		if v.id > maxID {
			maxID = v.id
		}
	}
	for cur := base; cur != nil; {
		if cur.isLazyReset() {
			cur = cur.extra.reset.base
			continue
		}
		switch cur.count {
		case 0:
		case 1:
			dst, maxID = appendUniqueMetaValue(dst, cur.val0, maxID)
		case 2:
			dst, maxID = appendUniqueMetaValue(dst, cur.val0, maxID)
			dst, maxID = appendUniqueMetaValue(dst, cur.extra.values[0], maxID)
		default:
			for _, d := range cur.extra.values {
				dst, maxID = appendUniqueMetaValue(dst, d, maxID)
			}
		}
		cur = cur.Next
	}
	slices.SortFunc(dst, func(a, b _Value) int {
		return cmp.Compare(a.id, b.id)
	})
	return dst, maxID
}

func appendUniqueMetaValue(dst []_Value, d _Value, maxID _TypeID) ([]_Value, _TypeID) {
	for i := range dst {
		if dst[i].id == d.id {
			return dst, maxID
		}
	}
	if d.id > maxID {
		maxID = d.id
	}
	return append(dst, d), maxID
}

func findSortedValue(sorted []_Value, id _TypeID) (_Value, bool) {
	left, right := uint(0), uint(len(sorted))
	for left < right {
		mid := (left + right) >> 1
		midID := sorted[mid].id
		if midID > id {
			right = mid
		} else if midID < id {
			left = mid + 1
		} else {
			return sorted[mid], true
		}
	}
	return _Value{}, false
}

func newForkerWithEntries(scope Scope, entries []*_TypeIDCacheEntry) *_Forker {
	if scope.values == nil && len(entries) == 1 {
		meta := getOrBuildDefMeta(entries[0])
		if meta.singleFunc1 {
			return &_Forker{
				val0TypeInfo: meta.values[0].typeInfo,
				Signature:    meta.emptySig,
				val0ID:       meta.values[0].id,
				kind:         forkerKindSingleFunc1,
			}
		}
	}

	var newValsBuf [8]_Value
	newValuesTemplate := newValsBuf[:0]
	var redefinedBuf [8]_TypeID
	redefinedIDs := redefinedBuf[:0]
	var originIDsBuf [8]_TypeID
	var originsBuf [8]_DefOrigin
	newDefOrigins := originIDsBuf[:0]
	origins := originsBuf[:0]
	var specsBuf [8]_DefSpec
	defSpecs := specsBuf[:0]

	for defIdx, entry := range entries {
		defType := entry.typ
		meta := getOrBuildDefMeta(entry)
		if meta.isPointer {
			v := meta.values[0]
			origin := _DefOrigin{defIndex: defIdx, defType: defType, outputIndex: -1}
			checkDuplicateOutputSlice(newDefOrigins, origins, defType.Elem(), v.id, origin)
			newValuesTemplate = append(newValuesTemplate, v)
			newDefOrigins = append(newDefOrigins, v.id)
			origins = append(origins, origin)
			if _, ok := scope.values.loadMeta(v.id); ok {
				redefinedIDs = append(redefinedIDs, v.id)
			}
			defSpecs = append(defSpecs, _DefSpec{numValues: 1, isPointer: true})
		} else {
			for i, v := range meta.values {
				origin := _DefOrigin{defIndex: defIdx, defType: defType, outputIndex: i}
				checkDuplicateOutputSlice(newDefOrigins, origins, defType.Out(i), v.id, origin)
				newValuesTemplate = append(newValuesTemplate, v)
				newDefOrigins = append(newDefOrigins, v.id)
				origins = append(origins, origin)
				if _, ok := scope.values.loadMeta(v.id); ok {
					redefinedIDs = append(redefinedIDs, v.id)
				}
			}
			defSpecs = append(defSpecs, _DefSpec{numValues: uint16(len(meta.values))})
		}
	}
	if len(redefinedIDs) > 1 {
		slices.Sort(redefinedIDs)
	}

	var sortedBuf [8]_Value
	var posesBuf [8]posAtSorted
	var sortedNew []_Value
	var posesAtSorted []posAtSorted
	nNew := len(newValuesTemplate)
	if nNew <= 1 {
		sortedNew = newValuesTemplate
		if nNew == 1 {
			posesAtSorted = posesBuf[:1]
			posesAtSorted[0] = 0
		}
	} else if nNew == 2 {
		posesAtSorted = posesBuf[:2]
		sortedNew = sortedBuf[:2]
		if newValuesTemplate[0].id < newValuesTemplate[1].id {
			posesAtSorted[0], posesAtSorted[1] = 0, 1
			sortedNew[0], sortedNew[1] = newValuesTemplate[0], newValuesTemplate[1]
		} else {
			posesAtSorted[0], posesAtSorted[1] = 1, 0
			sortedNew[0], sortedNew[1] = newValuesTemplate[1], newValuesTemplate[0]
		}
	} else {
		posesAtSorted = make([]posAtSorted, nNew)
		type posAtTemplate int
		posesAtTemplate := make([]posAtTemplate, nNew)
		for i := range nNew {
			posesAtTemplate[i] = posAtTemplate(i)
		}
		slices.SortFunc(posesAtTemplate, func(a, b posAtTemplate) int {
			return cmp.Compare(newValuesTemplate[a].id, newValuesTemplate[b].id)
		})
		sortedNew = make([]_Value, nNew)
		for i, j := range posesAtTemplate {
			posesAtSorted[j] = posAtSorted(i)
			sortedNew[i] = newValuesTemplate[j]
		}
	}

	var effectiveBuf [48]_Value
	effective, maxID := collectEffectiveMeta(scope.values, sortedNew, effectiveBuf[:0])
	resetIDs := analyzeForkDependencies(scope.values, effective, maxID, redefinedIDs, nNew > 0)
	signature := scopeSignatureSorted(effective)

	if nNew == 0 && len(resetIDs) == 0 {
		return &_Forker{
			Signature: signature,
			kind:      forkerKindEmpty,
		}
	}
	if len(defSpecs) == 1 && nNew == 1 && !defSpecs[0].isPointer {
		return &_Forker{
			val0TypeInfo: newValuesTemplate[0].typeInfo,
			ResetIDs:     resetIDs,
			Signature:    signature,
			val0ID:       newValuesTemplate[0].id,
			kind:         forkerKindSingleFunc1,
		}
	}
	if len(defSpecs) == 2 && nNew == 2 && !defSpecs[0].isPointer && !defSpecs[1].isPointer {
		return &_Forker{
			val0TypeInfo: newValuesTemplate[0].typeInfo,
			val1TypeInfo: newValuesTemplate[1].typeInfo,
			ResetIDs:     resetIDs,
			Signature:    signature,
			val0ID:       newValuesTemplate[0].id,
			val1ID:       newValuesTemplate[1].id,
			kind:         forkerKindSingleFunc2,
			swap2:        posesAtSorted[0] != 0,
		}
	}

	heapVals := slices.Clone(newValuesTemplate)
	heapSpecs := slices.Clone(defSpecs)
	heapPoses := slices.Clone(posesAtSorted)
	return &_Forker{
		multi: &_ForkerMulti{
			NewValuesTemplate: heapVals,
			DefSpecs:          heapSpecs,
			PosesAtSorted:     heapPoses,
		},
		ResetIDs:  resetIDs,
		Signature: signature,
		kind:      forkerKindGeneral,
	}
}

func analyzeForkDependencies(
	base *_StackedMap,
	effective []_Value,
	maxID _TypeID,
	redefinedIDs []_TypeID,
	hasNewDefs bool,
) []_TypeID {
	var smallStates [256]uint8
	var largeStates map[_TypeID]uint8
	useSmall := int(maxID) < len(smallStates)
	if !useSmall {
		largeStates = make(map[_TypeID]uint8, len(effective))
	}
	getState := func(id _TypeID) uint8 {
		if useSmall {
			return smallStates[id]
		}
		return largeStates[id]
	}
	setState := func(id _TypeID, st uint8) {
		if useSmall {
			smallStates[id] = st
		} else {
			largeStates[id] = st
		}
	}

	var pathBuf [32]_TypeID
	path := pathBuf[:0]
	var traverse func(value _Value) (bool, error)
	traverse = func(value _Value) (bool, error) {
		id := value.id
		switch getState(id) {
		case 1:
			return false, errors.Join(
				fmt.Errorf("found dependency loop in definition %v", value.typeInfo.DefType),
				ErrDependencyLoop,
				fmt.Errorf("path: %s", dependencyLoopPath(path, id)),
			)
		case 2:
			return false, nil
		case 3:
			return true, nil
		}

		setState(id, 1)
		reset := containsID(redefinedIDs, id)
		path = append(path, id)
		for _, depID := range value.typeInfo.Dependencies {
			if isAlwaysProvided(depID) {
				if hasNewDefs {
					reset = true
				}
				continue
			}
			depValue, ok := findSortedValue(effective, depID)
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
		if reset {
			setState(id, 3)
		} else {
			setState(id, 2)
		}
		return reset, nil
	}

	for _, value := range effective {
		if _, err := traverse(value); err != nil {
			panic(err)
		}
	}

	if base == nil {
		return nil
	}
	var resetIDs []_TypeID
	for _, value := range effective {
		id := value.id
		if getState(id) != 3 || containsID(redefinedIDs, id) {
			continue
		}
		if _, ok := base.loadMeta(id); ok {
			resetIDs = append(resetIDs, id)
		}
	}
	return resetIDs
}

func (f *_Forker) Fork(s Scope, defs []any) Scope {
	if f.kind == forkerKindSingleFunc1 {
		base := s.values
		if base != nil && (base.isLazyReset() || base.Height > stackHeightLimit) {
			base = base.flatten()
		}
		if len(f.ResetIDs) == 0 {
			bundle := new(_ForkBundle)
			layer := &bundle.layer
			if base != nil {
				layer.Next = base
				layer.Height = base.Height + 1
				if hb := base.hot.Load(); hb != nil && hb.id != f.val0ID {
					layer.hot.Store(hb)
				}
			} else {
				layer.Height = 1
			}
			layer.count = 1
			bundle.init0.Def = defs[0]
			layer.val0 = _Value{
				initializer: &bundle.init0,
				typeInfo:    f.val0TypeInfo,
				id:          f.val0ID,
			}
			return Scope{values: layer, signature: f.Signature}
		}
		return f.applySingleFunc1Reset(base, defs[0])
	}
	return f.forkNonSingle1(s, defs)
}

func (f *_Forker) applySingleFunc1Reset(base *_StackedMap, def0 any) Scope {
	bundle := new(_ForkBundleReset)
	layer := &bundle.layer
	if base != nil {
		layer.Next = base
		layer.Height = base.Height + 1
	} else {
		layer.Height = 1
	}
	layer.count = 1
	bundle.init0.Def = def0
	layer.val0 = _Value{
		initializer: &bundle.init0,
		typeInfo:    f.val0TypeInfo,
		id:          f.val0ID,
	}
	bundle.reset.ids = &f.ResetIDs
	bundle.extra.reset = &bundle.reset
	layer.extra = &bundle.extra
	return Scope{values: layer, signature: f.Signature}
}

func (f *_Forker) applySingleFunc2(s Scope, d0, d1 any) Scope {
	base := s.values
	if base != nil && (base.isLazyReset() || base.Height > stackHeightLimit) {
		base = base.flatten()
	}
	if len(f.ResetIDs) == 0 {
		bundle := new(_ForkBundle2)
		layer := &bundle.layer
		if base != nil {
			layer.Next = base
			layer.Height = base.Height + 1
			if hb := base.hot.Load(); hb != nil && hb.id != f.val0ID && hb.id != f.val1ID {
				layer.hot.Store(hb)
			}
		} else {
			layer.Height = 1
		}
		layer.count = 2
		bundle.inits[0].Def = d0
		bundle.inits[1].Def = d1
		v0 := _Value{initializer: &bundle.inits[0], typeInfo: f.val0TypeInfo, id: f.val0ID}
		v1 := _Value{initializer: &bundle.inits[1], typeInfo: f.val1TypeInfo, id: f.val1ID}
		if !f.swap2 {
			layer.val0 = v0
			bundle.val1[0] = v1
		} else {
			layer.val0 = v1
			bundle.val1[0] = v0
		}
		bundle.extra.values = bundle.val1[:]
		layer.extra = &bundle.extra
		return Scope{values: layer, signature: f.Signature}
	}
	return f.applySingleFunc2Reset(base, d0, d1)
}

func (f *_Forker) applySingleFunc2Reset(base *_StackedMap, d0, d1 any) Scope {
	bundle := new(_ForkBundle2Reset)
	layer := &bundle.layer
	if base != nil {
		layer.Next = base
		layer.Height = base.Height + 1
	} else {
		layer.Height = 1
	}
	layer.count = 2
	bundle.inits[0].Def = d0
	bundle.inits[1].Def = d1
	v0 := _Value{initializer: &bundle.inits[0], typeInfo: f.val0TypeInfo, id: f.val0ID}
	v1 := _Value{initializer: &bundle.inits[1], typeInfo: f.val1TypeInfo, id: f.val1ID}
	if !f.swap2 {
		layer.val0 = v0
		bundle.val1[0] = v1
	} else {
		layer.val0 = v1
		bundle.val1[0] = v0
	}
	bundle.extra.values = bundle.val1[:]
	bundle.reset.ids = &f.ResetIDs
	bundle.extra.reset = &bundle.reset
	layer.extra = &bundle.extra
	return Scope{values: layer, signature: f.Signature}
}

func (f *_Forker) forkNonSingle1(s Scope, defs []any) Scope {
	switch f.kind {
	case forkerKindSingleFunc2:
		return f.applySingleFunc2(s, defs[0], defs[1])
	case forkerKindEmpty:
		return Scope{values: s.values, signature: f.Signature}
	default:
		return f.forkGeneral(s, defs)
	}
}

func (f *_Forker) forkGeneral(s Scope, defs []any) Scope {
	base := s.values
	if base != nil && (base.isLazyReset() || base.Height > stackHeightLimit) {
		base = base.flatten()
	}
	m := f.multi
	var nVals int
	var nDefs int
	if m != nil {
		nVals = len(m.NewValuesTemplate)
		nDefs = len(m.DefSpecs)
	}

	var layer *_StackedMap
	if nDefs == 1 {
		bundle := newForkBundle(base, nVals)
		layer = &bundle.layer
		init0 := &bundle.init0
		spec := m.DefSpecs[0]
		initInitializer(init0, defs[0], spec.isPointer)
		for valueIdx := range int(spec.numValues) {
			template := m.NewValuesTemplate[valueIdx]
			template.initializer = init0
			layer.setAt(int(m.PosesAtSorted[valueIdx]), template)
		}
	} else {
		layer = newStackedMapLayer(base, nVals)
		if nDefs > 0 {
			initializers := make([]_Initializer, nDefs)
			valueIdx := 0
			for defIdx, def := range defs {
				spec := m.DefSpecs[defIdx]
				initializer := &initializers[defIdx]
				initInitializer(initializer, def, spec.isPointer)
				for range int(spec.numValues) {
					template := m.NewValuesTemplate[valueIdx]
					template.initializer = initializer
					layer.setAt(int(m.PosesAtSorted[valueIdx]), template)
					valueIdx++
				}
			}
		}
	}

	layer.buildIndex()
	if len(f.ResetIDs) > 0 {
		layer.markRefresh(&f.ResetIDs)
	}
	return Scope{values: layer, signature: f.Signature}
}
