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

// _Forker pre-calculates the information required to efficiently create a new child scope.
// Instances are cached based on the parent scope's signature and the types of the new definitions.
type _Forker struct {
	// Reducers maps the TypeID of reducer types to their reflect.Type.
	Reducers map[_TypeID]reflect.Type
	// NewValuesTemplate contains template _Value objects (without initializers) for the new definitions.
	NewValuesTemplate []_Value
	// DefKinds stores the reflect.Kind (Func or Ptr) for each new definition.
	DefKinds []reflect.Kind
	// DefNumValues stores the number of values produced by each function definition.
	DefNumValues []int
	// PosesAtSorted maps the original index of a value in NewValuesTemplate to its index in the sorted slice.
	PosesAtSorted []posAtSorted
	// ResetIDs lists TypeIDs (sorted) of values from the parent scope that need invalidation due to overrides or dependency changes.
	ResetIDs []_TypeID // sorted
	// ResetReducers lists info (sorted by marker TypeID) about reducers needing recalculation.
	ResetReducers []reducerInfo // sorted
	// Signature is a hash representing the structural identity of the scope *after* this fork.
	Signature _Hash
	// Key is the cache key for this _Forker, derived from parent signature and new definition types.
	Key _Hash
}

// posAtSorted represents the index of a value within the sorted slice of new values.
type posAtSorted int

// reducerInfo holds metadata for creating reducer marker values.
type reducerInfo struct {
	// typeInfo describes the internal marker type used to store the reduced value.
	typeInfo *_TypeInfo
	// originType is the actual reflect.Type of the reducer.
	originType reflect.Type
	// reducerKind indicates standard or custom reducer.
	reducerKind reducerKind
}

// newForker analyzes the parent scope and new definitions to create a _Forker.
// It performs dependency analysis, detects loops, determines resets, and calculates signatures.
func newForker(
	scope Scope,
	defs []any,
	key _Hash, // Cache key
) *_Forker {

	// 1. Process Definitions: Create templates, store metadata, identify overrides.
	newValuesTemplate := make([]_Value, 0, len(defs))
	redefinedIDs := make(map[_TypeID]struct{}) // Set of overridden TypeIDs
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for _, def := range defs {
		defType := reflect.TypeOf(def)
		defValue := reflect.ValueOf(def)
		defKinds = append(defKinds, defType.Kind())

		switch defType.Kind() {
		case reflect.Func:
			// Validate function
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

			// Extract Dependencies
			numIn := defType.NumIn()
			dependencies := make([]_TypeID, 0, numIn)
			for i := range numIn {
				inType := defType.In(i)
				if inType.Implements(typeWrapperType) { // Handle type wrappers
					depTypes := unwrapType(inType)
					for _, depType := range depTypes {
						dependencies = append(dependencies, getTypeID(depType))
					}
				} else {
					dependencies = append(dependencies, getTypeID(inType))
				}
			}

			// Create Value Templates for Outputs
			numOut := defType.NumOut()
			var numValues int
			for i := range numOut {
				t := defType.Out(i)
				id := getTypeID(t)

				newValuesTemplate = append(newValuesTemplate, _Value{
					typeInfo: &_TypeInfo{
						TypeID:       id,
						DefType:      defType,
						Position:     i,
						DefIsMulti:   numOut > 1,
						Dependencies: dependencies,
					},
				})
				numValues++
				if _, ok := scope.values.LoadOne(id); ok {
					redefinedIDs[id] = struct{}{} // Mark override
				}
			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Pointer:
			// Validate pointer
			if defValue.IsNil() {
				_ = throw(we.With(
					e5.Info("%T nil pointer provided", def),
				)(
					ErrBadArgument,
				))
			}

			// Create Value Template
			t := defType.Elem()
			id := getTypeID(t)
			newValuesTemplate = append(newValuesTemplate, _Value{
				typeInfo: &_TypeInfo{
					TypeID:     id,
					DefType:    defType,
					DefIsMulti: false,
				},
			})
			if _, ok := scope.values.LoadOne(id); ok {
				redefinedIDs[id] = struct{}{} // Mark override
			}
			defNumValues = append(defNumValues, 1)

		default:
			_ = throw(we.With(
				e5.Info("%T is not a valid definition", def),
			)(
				ErrBadArgument,
			))
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
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i) // posesAtSorted[original_index] = sorted_index
	}
	sortedNewValuesTemplate := slices.Clone(newValuesTemplate)
	slices.SortFunc(sortedNewValuesTemplate, func(a, b _Value) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

	// 3. Build Conceptual Next Scope & Analyze Dependencies via DFS:
	//    - `valuesTemplate`: Temporary _StackedMap representing the potential child scope.
	//    - Detect loops (`colors`: 0=White, 1=Gray, 2=Black).
	//    - Determine which types need reset (`needsReset`).
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)
	colors := make(map[_TypeID]int)      // For cycle detection
	needsReset := make(map[_TypeID]bool) // Memoization for reset status

	var traverse func(values []_Value, path []_TypeID) (reset bool, err error)
	traverse = func(values []_Value, path []_TypeID) (reset bool, err error) {
		id := values[0].typeInfo.TypeID

		// Cycle Detection & Memoization
		color := colors[id]
		switch color {
		case 1: // Gray: Loop detected
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
		case 2: // Black: Already processed
			return needsReset[id], nil
		}

		colors[id] = 1 // Mark as visiting (Gray)
		defer func() { // Ensure state is updated on return
			if err == nil {
				colors[id] = 2
				needsReset[id] = reset
			}
		}()

		// Base Case: Check if directly redefined in this fork
		if _, ok := redefinedIDs[id]; ok {
			reset = true
		}

		// Recursive Step: Check Dependencies
		for _, value := range values {
			for _, depID := range value.typeInfo.Dependencies {
				if isAlwaysProvided(depID) {
					continue
				}
				depValues, ok := valuesTemplate.Load(depID)
				if !ok {
					return false, we.With(
						e5.Info("dependency not found in definition %v", value.typeInfo.DefType),
						e5.Info("no definition for %v", typeIDToType(depID)),
						e5.Info("path: %+v", scope.path),
					)(
						ErrDependencyNotFound,
					)
				}
				depResets, err := traverse(depValues, append(path, value.typeInfo.TypeID))
				if err != nil {
					return false, err
				}
				reset = reset || depResets // Propagate reset requirement
			}
		}

		return
	}

	// 4. Analyze All Types in Conceptual Scope:
	//    - Populate `needsReset` and detect loops globally via `traverse`.
	//    - Collect `defTypeIDs` for signature.
	//    - Identify `reducers`.
	//    - Check for multiple non-reducer definitions.
	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len()) // For signature
	reducers := make(map[_TypeID]reflect.Type)             // TypeID -> Reducer Type

	if err := valuesTemplate.Range(func(values []_Value) error { // Iterates unique TypeIDs
		if _, err := traverse(values, nil); err != nil {
			return err
		}

		// Collect definition type IDs (sorted insert)
		for _, value := range values {
			defTypeID := getTypeID(value.typeInfo.DefType)
			i, found := slices.BinarySearch(defTypeIDs, defTypeID)
			if !found {
				defTypeIDs = slices.Insert(defTypeIDs, i, defTypeID)
			}
		}

		// Check for multiple definitions & identify reducers
		if len(values) > 1 {
			t := typeIDToType(values[0].typeInfo.TypeID)
			kind := getReducerKind(t)
			if kind == notReducer {
				return we.With(
					e5.Info("%v has multiple definitions", t),
				)(
					ErrBadDefinition,
				)
			}
			reducers[values[0].typeInfo.TypeID] = t
		}

		return nil
	}); err != nil {
		_ = throw(err)
	}

	// 5. Calculate Child Scope Signature: Hash sorted definition type IDs.
	h := sha256.New()
	buf := make([]byte, 0, len(defTypeIDs)*8)
	for _, id := range defTypeIDs {
		buf = binary.NativeEndian.AppendUint64(buf, uint64(id))
	}
	if _, err := h.Write(buf); err != nil {
		panic(err) // Extremely unlikely
	}
	var signature _Hash
	h.Sum(signature[:0])

	// 6. Identify Values Requiring Reset: Collect TypeIDs that need reset AND existed in parent.
	resetIDs := make([]_TypeID, 0, len(needsReset))
	for id, reset := range needsReset {
		if !reset {
			continue
		}
		if _, ok := scope.values.LoadOne(id); ok { // Only reset inherited values
			resetIDs = append(resetIDs, id)
		}
	}
	slices.Sort(resetIDs)

	// 7. Identify Reducers Requiring Recalculation: Collect info for new or reset reducers.
	resetReducers := make([]reducerInfo, 0, len(reducers))
	resetReducerSet := make(map[_TypeID]bool) // Avoid duplicates
	addReducerToReset := func(id _TypeID) {
		if _, ok := resetReducerSet[id]; ok {
			return
		}
		originType, isReducer := reducers[id]
		if !isReducer {
			return
		}
		kind := getReducerKind(originType)
		markType := getReducerMarkType(id) // Internal type for the reduced value
		resetReducers = append(resetReducers, reducerInfo{
			typeInfo: &_TypeInfo{
				TypeID:  getTypeID(markType),
				DefType: reflect.TypeFor[func()](), // Placeholder DefType
			},
			originType:  originType,
			reducerKind: kind,
		})
		resetReducerSet[id] = true
	}

	for _, value := range newValuesTemplate { // Add reducers for new types
		addReducerToReset(value.typeInfo.TypeID)
	}
	for _, id := range resetIDs { // Add reducers for reset types
		addReducerToReset(id)
	}
	slices.SortFunc(resetReducers, func(a, b reducerInfo) int { // Sort by marker type ID
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

	// 8. Return the completed _Forker.
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

// Fork applies the pre-calculated changes from the _Forker to the parent scope, creating a child scope.
func (f *_Forker) Fork(s Scope, defs []any) Scope {

	// 1. Initialize Child Scope Shell.
	scope := Scope{
		signature:   f.Signature,
		forkFuncKey: f.Key,
		reducers:    f.Reducers,
	}

	// 2. Handle Parent Scope Stack: Flatten if deep.
	if s.values != nil && s.values.Height > 16 { // Threshold for flattening
		var flatValues []_Value
		if err := s.values.Range(func(parentValues []_Value) error {
			flatValues = append(flatValues, parentValues...)
			return nil
		}); err != nil {
			_ = throw(err)
		}
		slices.SortFunc(flatValues, func(a, b _Value) int { // Sort flattened values
			return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
		})
		scope.values = &_StackedMap{
			Values: flatValues,
			Height: 1,
		}
	} else {
		scope.values = s.values // Inherit parent stack top
	}

	// 3. Create and Add New Values Layer: Instantiate initializers and values.
	newValues := make([]_Value, len(f.NewValuesTemplate))
	valueIdx := 0
	for defIdx, def := range defs {
		kind := f.DefKinds[defIdx]
		initializer := newInitializer(def, notReducer) // One initializer per definition

		switch kind {
		case reflect.Func:
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
			template := f.NewValuesTemplate[valueIdx]
			sortedIdx := f.PosesAtSorted[valueIdx]
			newValues[sortedIdx] = _Value{
				typeInfo:    template.typeInfo,
				initializer: initializer,
			}
			valueIdx++
		}
	}
	scope.values = scope.values.Append(newValues)

	// 4. Create and Add Reset Values Layer: Contains reset initializers for overridden/affected parent values.
	if len(f.ResetIDs) > 0 {
		resetValues := make([]_Value, 0, len(f.ResetIDs))
		resetInitializers := make(map[int64]*_Initializer) // Track reset initializers for sharing
		for _, id := range f.ResetIDs {
			currentDefs, ok := scope.values.Load(id) // Load definitions from current stack
			if !ok {
				panic("impossible: reset ID not found in scope")
			}
			for _, value := range currentDefs {
				initID := value.initializer.ID
				resetInit, found := resetInitializers[initID]
				if !found {
					resetInit = value.initializer.reset() // Create fresh initializer
					resetInitializers[initID] = resetInit
				}
				resetValues = append(resetValues, _Value{
					typeInfo:    value.typeInfo,
					initializer: resetInit,
				})
			}
		}
		// resetValues are implicitly sorted by type ID.
		scope.values = scope.values.Append(resetValues)
	}

	// 5. Create and Add Reducer Marker Layer: Triggers reducer recalculation.
	if len(f.ResetReducers) > 0 {
		reducerValues := make([]_Value, 0, len(f.ResetReducers))
		for _, info := range f.ResetReducers {
			// Initializer uses original reducer type but stored under marker type ID.
			reducerInitializer := newInitializer(info.originType, info.reducerKind)
			reducerValues = append(reducerValues, _Value{
				typeInfo:    info.typeInfo, // Info for the marker type
				initializer: reducerInitializer,
			})
		}
		// reducerValues are implicitly sorted by marker type ID.
		scope.values = scope.values.Append(reducerValues)
	}

	// 6. Return the fully constructed child scope.
	return scope
}
