package dscope

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"slices"
	"strings"

	"github.com/reusee/e5"
)

// _Forker pre-calculates the information required to efficiently create a new
// child scope from a parent scope, given a specific set of new definitions.
// Instances are cached based on the parent scope's signature and the types
// of the new definitions to speed up repeated Fork operations with the same structure.
type _Forker struct {
	// Reducers maps the TypeID of reducer types to their reflect.Type.
	// This includes reducers from the parent scope and any new ones.
	Reducers map[_TypeID]reflect.Type
	// NewValuesTemplate contains template _Value objects (without initializers)
	// for the definitions provided in the Fork call. The order matches the
	// original order of definitions in the Fork call.
	NewValuesTemplate []_Value
	// DefKinds stores the reflect.Kind (Func or Ptr) for each definition passed to Fork.
	DefKinds []reflect.Kind
	// DefNumValues stores the number of values produced by each function definition.
	DefNumValues []int
	// PosesAtSorted maps the original index of a value in NewValuesTemplate
	// to its index in the sorted slice that will be added to the _StackedMap layer.
	PosesAtSorted []posAtSorted
	// ResetIDs lists the TypeIDs (sorted) of values that existed in the parent scope
	// and whose initializers need to be invalidated and potentially re-evaluated
	// in the child scope due to overrides or changes in their dependencies.
	ResetIDs []_TypeID // sorted
	// ResetReducers lists information (sorted by marker TypeID) about reducers
	// whose aggregated value needs to be recalculated. This includes newly defined
	// reducers or existing ones whose dependencies were reset.
	ResetReducers []reducerInfo // sorted
	// Signature is a hash representing the structural identity of the scope *after*
	// this fork operation. It's based on the sorted TypeIDs of all definition types
	// involved in the final scope (parent + new).
	Signature _Hash
	// Key is the cache key used to store and retrieve this _Forker instance in the
	// global `forkers` map. It's derived from the parent scope's signature and
	// the TypeIDs of the new definitions.
	Key _Hash
}

// posAtSorted represents the index of a value within the sorted slice of new values.
type posAtSorted int

// reducerInfo holds metadata needed to create a marker value for a reducer type,
// triggering its (re-)calculation when the marker value is requested.
type reducerInfo struct {
	// typeInfo describes the marker type used internally to store the reduced value.
	typeInfo *_TypeInfo
	// originType is the actual reflect.Type of the reducer.
	originType reflect.Type
	// reducerKind indicates whether it's a standard or custom reducer.
	reducerKind reducerKind
}

// newForker analyzes the parent scope and the new definitions to pre-calculate
// all necessary information for creating the child scope efficiently.
// It performs dependency analysis, detects loops, determines which values need
// resetting, calculates structural signatures, and builds the _Forker object.
func newForker(
	scope Scope,
	defs []any,
	key _Hash, // Cache key based on parent signature and new def types
) *_Forker {

	// 1. Process Definitions:
	//    - Create template `_Value` objects for each output type from `defs`.
	//    - Store metadata (kinds, number of values).
	//    - Identify types being redefined (overridden) in this fork.
	newValuesTemplate := make([]_Value, 0, len(defs))
	redefinedIDs := make(map[_TypeID]struct{}) // Set of TypeIDs being overridden
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for _, def := range defs {
		defType := reflect.TypeOf(def)
		defValue := reflect.ValueOf(def)
		defKinds = append(defKinds, defType.Kind())

		switch defType.Kind() {
		case reflect.Func:
			// --- Function Definition Validation ---
			if defValue.IsNil() {
				_ = throw(we.With(
					e5.Info("%T nil function provided", def),
				)(
					ErrBadArgument,
				))
			}
			if defType.NumOut() == 0 {
				_ = throw(we.With(
					e5.Info("%T returns nothing", def),
				)(
					ErrBadArgument,
				))
			}

			// --- Extract Dependencies ---
			numIn := defType.NumIn()
			dependencies := make([]_TypeID, 0, numIn)
			for i := range numIn {
				inType := defType.In(i)
				// Handle type wrappers (e.g., for optional dependencies)
				if inType.Implements(typeWrapperType) {
					depTypes := unwrapType(inType)
					for _, depType := range depTypes {
						dependencies = append(dependencies, getTypeID(depType))
					}
				} else {
					dependencies = append(dependencies, getTypeID(inType))
				}
			}

			// --- Create Value Templates for Outputs ---
			numOut := defType.NumOut()
			var numValues int
			for i := range numOut {
				t := defType.Out(i)
				id := getTypeID(t)

				newValuesTemplate = append(newValuesTemplate, _Value{
					typeInfo: &_TypeInfo{
						TypeID:       id,
						DefType:      defType, // Type of the defining function/pointer
						Position:     i,       // Index of this type in the def's outputs
						DefIsMulti:   numOut > 1,
						Dependencies: dependencies,
					},
				})
				numValues++
				// Mark if this type overrides one from the parent scope
				if _, ok := scope.values.LoadOne(id); ok {
					redefinedIDs[id] = struct{}{}
				}
			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Pointer:
			// --- Pointer Definition Validation ---
			if defValue.IsNil() {
				_ = throw(we.With(
					e5.Info("%T nil pointer provided", def),
				)(
					ErrBadArgument,
				))
			}

			// --- Create Value Template for Pointed-to Type ---
			t := defType.Elem()
			id := getTypeID(t)
			newValuesTemplate = append(newValuesTemplate, _Value{
				typeInfo: &_TypeInfo{
					TypeID:     id,
					DefType:    defType,
					DefIsMulti: false,
				},
			})
			// Mark if this type overrides one from the parent scope
			if _, ok := scope.values.LoadOne(id); ok {
				redefinedIDs[id] = struct{}{}
			}
			defNumValues = append(defNumValues, 1)

		default:
			// Invalid definition kind
			_ = throw(we.With(
				e5.Info("%T is not a valid definition", def),
			)(
				ErrBadArgument,
			))
		}
	}

	// 2. Sort New Values & Create Index Mapping:
	//    - Sort the template values by TypeID for efficient storage in _StackedMap.
	//    - Create `posesAtSorted` to map original indices to sorted indices.
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
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i) // posesAtSorted[original_index] = sorted_index
	}
	sortedNewValuesTemplate := slices.Clone(newValuesTemplate) // Will be used for valuesTemplate
	slices.SortFunc(sortedNewValuesTemplate, func(a, b _Value) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

	// 3. Build Conceptual Next Scope & Analyze Dependencies:
	//    - Create `valuesTemplate`: a temporary _StackedMap representing the potential child scope.
	//    - Perform DFS traversal (`traverse`) on `valuesTemplate` to:
	//        - Detect dependency loops (using `colors` map: 0=White, 1=Gray, 2=Black).
	//        - Determine which types need their values reset (`needsReset` map). A type
	//          needs reset if it's directly redefined or if any of its dependencies need reset.
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)
	colors := make(map[_TypeID]int)      // For cycle detection
	needsReset := make(map[_TypeID]bool) // Memoization for reset status

	var traverse func(values []_Value, path []_TypeID) (reset bool, err error)
	traverse = func(values []_Value, path []_TypeID) (reset bool, err error) {
		id := values[0].typeInfo.TypeID

		// --- Cycle Detection & Memoization ---
		color := colors[id]
		switch color {
		case 1: // Gray: Visiting this node again in the current path -> loop detected
			return false, we.With(
				e5.Info("found dependency loop in definition %v", values[0].typeInfo.DefType),
				func() e5.WrapFunc { // Build path string lazily
					buf := new(strings.Builder)
					for i, id := range path {
						if i > 0 {
							buf.WriteString(" -> ")
						}
						buf.WriteString(typeIDToType(id).String())
					}
					return e5.Info("path: %s", buf.String())
				}(),
			)(
				ErrDependencyLoop,
			)
		case 2: // Black: Already fully processed
			return needsReset[id], nil
		}

		// Mark as visiting (Gray)
		colors[id] = 1
		// Ensure state is updated on return (Black & memoized reset status)
		defer func() {
			if err == nil {
				colors[id] = 2
				needsReset[id] = reset
			}
		}()

		// --- Base Case for Reset ---
		// Does this Fork directly redefine this type?
		if _, ok := redefinedIDs[id]; ok {
			reset = true
		}

		// --- Recursive Step: Check Dependencies ---
		// Check all definitions providing this type `id`.
		for _, value := range values {
			for _, depID := range value.typeInfo.Dependencies {
				// Find the definition(s) for the dependency `depID` in the potential child scope.
				depValues, ok := valuesTemplate.Load(depID)
				if !ok {
					return false, we.With(
						e5.Info("dependency not found in definition %v", value.typeInfo.DefType),
						e5.Info("no definition for %v", typeIDToType(depID)),
						e5.Info("path: %+v", scope.path), // Show path from original scope call if available
					)(
						ErrDependencyNotFound,
					)
				}

				// Recursively analyze the dependency.
				depResets, err := traverse(depValues, append(path, value.typeInfo.TypeID))
				if err != nil {
					return false, err
				}
				// Propagate reset: If any dependency needs reset, this type also needs reset.
				reset = reset || depResets
			}
		}

		return
	}

	// 4. Analyze All Types in Conceptual Scope:
	//    - Iterate through all unique types in `valuesTemplate`.
	//    - Run `traverse` for each to populate `needsReset` and detect loops globally.
	//    - Collect all definition type IDs (`defTypeIDs`) for signature calculation.
	//    - Identify reducer types (`reducers` map).
	//    - Check for multiple definitions of non-reducer types.
	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len()) // For signature
	reducers := make(map[_TypeID]reflect.Type)             // TypeID -> Reducer Type

	if err := valuesTemplate.Range(func(values []_Value) error { // Iterates unique TypeIDs
		// Run dependency analysis starting from this type.
		if _, err := traverse(values, nil); err != nil {
			return err
		}

		// Collect definition type IDs for signature (sorted insertion).
		for _, value := range values {
			defTypeID := getTypeID(value.typeInfo.DefType)
			i, found := slices.BinarySearch(defTypeIDs, defTypeID)
			if !found {
				defTypeIDs = slices.Insert(defTypeIDs, i, defTypeID)
			}
		}

		// Check for multiple definitions & identify reducers.
		if len(values) > 1 {
			t := typeIDToType(values[0].typeInfo.TypeID)
			kind := getReducerKind(t)
			if kind == notReducer {
				// Multiple definitions for a type that isn't a reducer is an error.
				return we.With(
					e5.Info("%v has multiple definitions", t),
				)(
					ErrBadDefinition,
				)
			}
			reducers[values[0].typeInfo.TypeID] = t // Store reducer type
		}

		return nil
	}); err != nil {
		_ = throw(err) // Convert analysis errors to panics
	}

	// 5. Calculate Child Scope Signature:
	//    - Hash the sorted list of all definition type IDs involved in the child scope.
	h := sha256.New()
	buf := make([]byte, 0, len(defTypeIDs)*8)
	for _, id := range defTypeIDs {
		buf = binary.NativeEndian.AppendUint64(buf, uint64(id))
	}
	if _, err := h.Write(buf); err != nil { // NOCOVER - hash.Write error unlikely
		panic(err)
	}
	var signature _Hash
	h.Sum(signature[:0])

	// 6. Identify Values Requiring Reset:
	//    - Collect TypeIDs that need reset (`needsReset`) AND existed in the parent scope.
	resetIDs := make([]_TypeID, 0, len(needsReset))
	for id, reset := range needsReset {
		if !reset {
			continue
		}
		// Only reset values that were actually inherited from the parent.
		if _, ok := scope.values.LoadOne(id); ok {
			resetIDs = append(resetIDs, id)
		}
	}
	slices.Sort(resetIDs) // Sort for deterministic order in Fork()

	// 7. Identify Reducers Requiring Recalculation:
	//    - Collect `reducerInfo` for reducers associated with newly defined types
	//      or types whose values are being reset.
	resetReducers := make([]reducerInfo, 0, len(reducers))
	resetReducerSet := make(map[_TypeID]bool) // Avoid duplicates
	addReducerToReset := func(id _TypeID) {
		if _, ok := resetReducerSet[id]; ok { // Already added
			return
		}
		originType, isReducer := reducers[id]
		if !isReducer { // Not a reducer type
			return
		}
		kind := getReducerKind(originType)
		markType := getReducerMarkType(originType, id) // Internal type for the reduced value
		resetReducers = append(resetReducers, reducerInfo{
			typeInfo: &_TypeInfo{
				TypeID:  getTypeID(markType),       // TypeID of the marker
				DefType: reflect.TypeFor[func()](), // Placeholder DefType
			},
			originType:  originType, // The actual reducer type
			reducerKind: kind,
		})
		resetReducerSet[id] = true
	}

	// Add reducers for newly defined types (if they are reducers).
	for _, value := range newValuesTemplate {
		addReducerToReset(value.typeInfo.TypeID)
	}
	// Add reducers for reset types (if they are reducers).
	for _, id := range resetIDs {
		addReducerToReset(id)
	}
	// Sort by the marker type ID for deterministic application in Fork().
	slices.SortFunc(resetReducers, func(a, b reducerInfo) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

	// 8. Return the completed _Forker containing all pre-calculated data.
	return &_Forker{
		Signature:         signature,
		Key:               key,
		Reducers:          reducers,
		NewValuesTemplate: newValuesTemplate,
		DefKinds:          defKinds,
		DefNumValues:      defNumValues,
		PosesAtSorted:     posesAtSorted,
		ResetIDs:          resetIDs,
		ResetReducers:     resetReducers,
	}
}

// Fork applies the pre-calculated changes defined in the _Forker receiver (`f`)
// to the parent scope `s`, creating and returning the new child scope.
func (f *_Forker) Fork(s Scope, defs []any) Scope {

	// 1. Initialize Child Scope Shell:
	//    - Copy signature, key, and reducers map from the _Forker.
	scope := Scope{
		signature:   f.Signature,
		forkFuncKey: f.Key,
		reducers:    f.Reducers,
	}

	// 2. Handle Parent Scope Stack:
	//    - If the parent's stack is deep, flatten it into a single layer for performance.
	//    - Otherwise, inherit the parent's stack directly.
	if s.values != nil && s.values.Height > 16 { // Threshold for flattening
		var flatValues []_Value
		// Range iterates unique TypeIDs, giving all values for that ID
		if err := s.values.Range(func(parentValues []_Value) error {
			flatValues = append(flatValues, parentValues...)
			return nil
		}); err != nil { // NOCOVER - Range error unlikely
			_ = throw(err)
		}
		// Sort the flattened values by TypeID for the new layer
		slices.SortFunc(flatValues, func(a, b _Value) int {
			return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
		})
		scope.values = &_StackedMap{
			Values: flatValues,
			Height: 1, // Flattened stack has height 1
		}
	} else {
		scope.values = s.values // Point to the parent's stack top
	}

	// 3. Create and Add New Values Layer:
	//    - Instantiate `_Initializer` objects for each definition in `defs`.
	//    - Create the final `_Value` entries using the templates and initializers.
	//    - Sort these new values according to `PosesAtSorted`.
	//    - Append this sorted slice as a new layer onto the scope's `values` stack.
	newValues := make([]_Value, len(f.NewValuesTemplate))
	valueIdx := 0 // Tracks index in f.NewValuesTemplate
	for defIdx, def := range defs {
		kind := f.DefKinds[defIdx]
		// Create one initializer per definition (func or ptr)
		initializer := newInitializer(def, notReducer) // Reducers handled later

		switch kind {
		case reflect.Func:
			numValues := f.DefNumValues[defIdx]
			for range numValues {
				template := f.NewValuesTemplate[valueIdx]
				sortedIdx := f.PosesAtSorted[valueIdx]
				newValues[sortedIdx] = _Value{
					typeInfo:    template.typeInfo,
					initializer: initializer, // Share initializer for multi-return funcs
				}
				valueIdx++
			}
		case reflect.Pointer:
			template := f.NewValuesTemplate[valueIdx]
			sortedIdx := f.PosesAtSorted[valueIdx]
			newValues[sortedIdx] = _Value{
				typeInfo:    template.typeInfo,
				initializer: initializer,
			}
			valueIdx++
		}
	}
	scope.values = scope.values.Append(newValues) // Add the layer of newly defined values

	// 4. Create and Add Reset Values Layer:
	//    - If `ResetIDs` is not empty, create a new layer containing reset `_Value` entries.
	//    - For each `id` in `ResetIDs`, find its definition(s) in the current `scope.values`.
	//    - Create a *new* `_Initializer` for each unique definition needing reset using `initializer.reset()`.
	//      (Ensure multi-return functions share the same *new* initializer).
	//    - Append this layer containing only the reset values.
	if len(f.ResetIDs) > 0 {
		resetValues := make([]_Value, 0, len(f.ResetIDs))
		resetInitializers := make(map[int64]*_Initializer) // Track reset initializers for shared use
		for _, id := range f.ResetIDs {
			// Load *all* definitions for this ID from the stack (including the new layer)
			currentDefs, ok := scope.values.Load(id)
			if !ok { // NOCOVER - Should be impossible if ResetIDs was calculated correctly
				panic("impossible: reset ID not found in scope")
			}
			for _, value := range currentDefs {
				initID := value.initializer.ID
				// Check if an initializer for this specific definition has already been reset
				resetInit, found := resetInitializers[initID]
				if !found {
					// Create a fresh initializer by calling reset()
					resetInit = value.initializer.reset()
					resetInitializers[initID] = resetInit
				}
				// Add a _Value entry with the reset initializer
				resetValues = append(resetValues, _Value{
					typeInfo:    value.typeInfo,
					initializer: resetInit,
				})
			}
		}
		// resetValues are implicitly sorted because f.ResetIDs is sorted and Load preserves relative order.
		scope.values = scope.values.Append(resetValues)
	}

	// 5. Create and Add Reducer Marker Layer:
	//    - If `ResetReducers` is not empty, create a layer for reducer markers.
	//    - For each `reducerInfo`, create a `_Value` using the marker type's info.
	//    - Create a special `_Initializer` linked to the original reducer type
	//      and its `reducerKind`. This initializer will perform the reduction when requested.
	//    - Append this layer containing only the reducer markers.
	if len(f.ResetReducers) > 0 {
		reducerValues := make([]_Value, 0, len(f.ResetReducers))
		for _, info := range f.ResetReducers {
			// The initializer uses the *original* reducer type (e.g., `acc`) but is stored
			// under the *marker* type's ID (e.g., `func(reducerMark) acc`).
			reducerInitializer := newInitializer(info.originType, info.reducerKind)
			reducerValues = append(reducerValues, _Value{
				typeInfo:    info.typeInfo, // Info for the marker type
				initializer: reducerInitializer,
			})
		}
		// reducerValues are implicitly sorted because f.ResetReducers is sorted by marker TypeID.
		scope.values = scope.values.Append(reducerValues)
	}

	// 6. Return the fully constructed child scope.
	return scope
}
