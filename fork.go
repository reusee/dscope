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

// _Forker holds the pre-calculated information needed to efficiently create
// a new child scope from a parent scope given a specific set of new definitions.
// It's cached based on the parent scope's signature and the types of the new definitions.
type _Forker struct {
	// Reducers maps TypeID to the reflect.Type for all reducer types present in the scope
	// resulting from this fork operation.
	Reducers map[_TypeID]reflect.Type
	// NewValuesTemplate contains _Value objects (without initializers) for the definitions
	// provided in the Fork call, in their original order.
	NewValuesTemplate []_Value
	// DefKinds stores the reflect.Kind (Func or Ptr) for each definition in the Fork call.
	DefKinds []reflect.Kind
	// DefNumValues stores the number of return values for each function definition.
	DefNumValues []int
	// PosesAtSorted maps the original index of a value in NewValuesTemplate to its
	// index in the sorted slice used for the _StackedMap layer.
	PosesAtSorted []posAtSorted
	// ResetIDs lists the TypeIDs (sorted) of values that existed in the parent scope
	// and need to be reset (invalidated) due to this Fork operation.
	ResetIDs []_TypeID // sorted
	// ResetReducers lists information (sorted by marker TypeID) about reducers that
	// need to be recalculated, either because they are newly defined or because
	// their dependencies were reset.
	ResetReducers []reducerInfo // sorted
	// Signature is a hash representing the structural identity of the scope *after*
	// this fork operation is applied. Based on all definition types involved.
	Signature _Hash
	// Key is the cache key used to store and retrieve this _Forker instance.
	// Based on parent scope signature and new definition types.
	Key _Hash
}

// posAtSorted represents the index of a value in the sorted slice of new values.
type posAtSorted int

// reducerInfo holds data needed to create a marker value for a reducer
// that needs (re-)calculation.
type reducerInfo struct {
	typeInfo    *_TypeInfo
	originType  reflect.Type
	reducerKind reducerKind
}

// newForker pre-calculates the steps needed to transition from a parent scope
// to a child scope given a set of new definitions (`defs`).
// It analyzes dependencies, detects loops, determines necessary value resets,
// and calculates structural signatures for caching.
func newForker(
	scope Scope,
	defs []any,
	key _Hash,
) *_Forker {

	// Process `defs`, creating template `_Value` objects for each provided output type.
	// Also identifies types being redefined in this fork.
	newValuesTemplate := make([]_Value, 0, len(defs))
	redefinedIDs := make(map[_TypeID]struct{})
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for _, def := range defs {
		defType := reflect.TypeOf(def)
		defKinds = append(defKinds, defType.Kind())
		defValue := reflect.ValueOf(def)

		switch defType.Kind() {

		case reflect.Func:
			// Validate function definition
			if defValue.IsNil() {
				_ = throw(we.With(
					e5.Info("%T nil function provided", def),
				)(
					ErrBadArgument,
				))
			}

			numOut := defType.NumOut()
			if numOut == 0 {
				_ = throw(we.With(
					e5.Info("%T returns nothing", def),
				)(
					ErrBadArgument,
				))
			}

			// Extract dependencies, handling type wrappers
			numIn := defType.NumIn()
			dependencies := make([]_TypeID, 0, numIn)
			for i := range numIn {
				inType := defType.In(i)
				if inType.Implements(typeWrapperType) {
					depTypes := unwrapType(inType)
					for _, depType := range depTypes {
						dependencies = append(dependencies, getTypeID(depType))
					}
				} else {
					dependencies = append(dependencies, getTypeID(inType))
				}
			}

			// Create value templates for each output type
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
				// Mark if this type exists in parent (indicates redefinition)
				if _, ok := scope.values.LoadOne(id); ok {
					redefinedIDs[id] = struct{}{}
				}

			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Pointer:
			// Validate pointer definition
			if defValue.IsNil() {
				_ = throw(we.With(
					e5.Info("%T nil pointer provided", def),
				)(
					ErrBadArgument,
				))
			}

			// Create value template for the pointed-to type
			t := defType.Elem()
			id := getTypeID(t)
			newValuesTemplate = append(newValuesTemplate, _Value{
				typeInfo: &_TypeInfo{
					TypeID:     id,
					DefType:    defType,
					DefIsMulti: false,
				},
			})
			// Mark if this type exists in parent (indicates redefinition)
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

	// Sort new value templates by TypeID for storage in _StackedMap.
	// Create mapping from original template index to sorted index.
	type posAtTemplate int
	posesAtTemplate := make([]posAtTemplate, 0, len(newValuesTemplate))
	for i := range newValuesTemplate {
		posesAtTemplate = append(posesAtTemplate, posAtTemplate(i))
	}
	// Sort indices based on the TypeID of the corresponding value template
	slices.SortFunc(posesAtTemplate, func(a, b posAtTemplate) int {
		return cmp.Compare(
			newValuesTemplate[a].typeInfo.TypeID,
			newValuesTemplate[b].typeInfo.TypeID,
		)
	})
	// Create the final mapping: posesAtSorted[original_index] = sorted_index
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i)
	}

	// Create the sorted slice of new value templates
	sortedNewValuesTemplate := slices.Clone(newValuesTemplate)
	slices.SortFunc(sortedNewValuesTemplate, func(a, b _Value) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

	// Create a conceptual view of the next scope's values by appending the new layer.
	// This `valuesTemplate` represents the potential state used for dependency analysis.
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)

	// Traverses the potential dependency graph of the new scope (valuesTemplate).
	// Detects dependency loops using DFS coloring.
	// Determines which existing types need their values reset (`needsReset`). A type needs
	// reset if it's directly redefined OR if any of its dependencies need reset.
	colors := make(map[_TypeID]int)
	needsReset := make(map[_TypeID]bool)

	// analyzeDependencies recursively checks a type and its dependencies.
	// It returns `true` if the type or any dependency requires a reset.
	// It uses the `colors` map for cycle detection and memoization (`needsReset`).
	var traverse func(values []_Value, path []_TypeID) (reset bool, err error)
	traverse = func(values []_Value, path []_TypeID) (reset bool, err error) {
		id := values[0].typeInfo.TypeID

		// Check traversal state (colors) for loop detection and memoization
		color := colors[id]
		switch color {
		case 1:
			return false, we.With(
				e5.Info("found dependency loop in definition %v", values[0].typeInfo.DefType),
				func() e5.WrapFunc {
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
		case 2:
			// already visited
			return needsReset[id], nil
		}

		// Mark as visiting (Gray)
		colors[id] = 1
		// Ensure state is updated on return (Visited/Black and memoized reset status)
		defer func() {
			if err == nil {
				colors[id] = 2
				needsReset[id] = reset
			}
		}()

		// Base case for reset: Is this type directly redefined in this Fork?
		if _, ok := redefinedIDs[id]; ok {
			reset = true
		}

		// Recursive step: Check dependencies.
		// Need to consider all definitions providing this type `id`.
		for _, value := range values {
			for _, depID := range value.typeInfo.Dependencies {
				// Find the definition(s) for depID in the potential new scope's graph (valuesTemplate).
				value2, ok := valuesTemplate.Load(depID)
				if !ok {
					// Dependency not found in parent or new defs. Provide context.
					return false, we.With(
						e5.Info("dependency not found in definition %v", value.typeInfo.DefType),
						e5.Info("no definition for %v", typeIDToType(depID)),
						e5.Info("path: %+v", scope.path),
					)(
						ErrDependencyNotFound,
					)
				}

				// Recursively analyze the dependency.
				depResets, err := traverse(value2, append(path, value.typeInfo.TypeID))
				if err != nil {
					return false, err
				}

				// Reset propagation: If any dependency needs reset, this type also needs reset.
				reset = reset || depResets
			}
		}

		return
	}

	// Iterate through all unique types in the potential new scope (`valuesTemplate`).
	// Run the dependency analysis for each type to populate `needsReset` and detect loops.
	// Collect types that act as reducers.
	// Collect all definition TypeIDs for signature calculation.
	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len())
	reducers := make(map[_TypeID]reflect.Type) // no need to copy scope.reducers here, since valuesTemplate.Range will iterate all types.

	if err := valuesTemplate.Range(func(values []_Value) error {
		// Run analysis starting from this type (if not already visited).
		// This populates `needsReset` and checks for loops involving this type.
		if _, err := traverse(values, nil); err != nil {
			return err
		}

		// Collect definition type IDs for signature calculation.
		// Uses binary search for sorted insertion into defTypeIDs.
		for _, value := range values {
			id := getTypeID(value.typeInfo.DefType)
			i, ok := slices.BinarySearch(defTypeIDs, id)
			if i < len(defTypeIDs) {
				if !ok {
					// insert
					defTypeIDs = slices.Insert(defTypeIDs, i, id)
				}
			} else {
				// append
				defTypeIDs = append(defTypeIDs, id)
			}
		}

		// Check for multiple definitions of non-reducer types and identify reducers.
		if len(values) > 1 {
			t := typeIDToType(values[0].typeInfo.TypeID)
			if getReducerKind(t) == notReducer {
				return we.With(
					e5.Info("%v has multiple definitions", t),
				)(
					ErrBadDefinition,
				)
			}
			// Store reducer type (needed for resetReducers later)
			reducers[values[0].typeInfo.TypeID] = t
		}

		return nil
	}); err != nil {
		_ = throw(err)
	}

	// The signature uniquely identifies the structure of the scope after the fork,
	// based on the sorted list of all involved definition types.
	h := sha256.New() // must be cryptographic hash to avoid collision
	buf := make([]byte, 0, len(defTypeIDs)*8)
	for _, id := range defTypeIDs {
		buf = binary.NativeEndian.AppendUint64(buf, uint64(id))
	}
	if _, err := h.Write(buf); err != nil {
		panic(err)
	}
	var signature _Hash
	h.Sum(signature[:0])

	// Collect TypeIDs that both need reset (from analysis) and existed in the parent scope.
	resetIDs := make([]_TypeID, 0, len(redefinedIDs))

	for id, reset := range needsReset {
		if !reset {
			continue
		}
		// exclude newly defined values (those not present in the parent scope)
		if _, ok := scope.values.LoadOne(id); ok {
			resetIDs = append(resetIDs, id)
		}
	}

	slices.Sort(resetIDs)

	// Identify reducers that need their marker value added/updated in the new scope.
	// This includes reducers for newly defined types and reducers affected by resets.
	resetReducers := make([]reducerInfo, 0, len(reducers))
	resetReducerSet := make(map[_TypeID]bool)
	// Helper to add a reducer to the reset list if it's a valid reducer type.
	addReducerToReset := func(id _TypeID) {
		if _, ok := resetReducerSet[id]; ok {
			return
		}
		t, ok := reducers[id]
		if !ok {
			return
		}
		// Get the special marker type used to store the reduced value.
		markType := getReducerMarkType(t, id)
		resetReducers = append(resetReducers, reducerInfo{
			typeInfo: &_TypeInfo{
				TypeID:  getTypeID(markType),
				DefType: reflect.TypeFor[func()](),
			},
			originType:  t,
			reducerKind: getReducerKind(t),
		})
		resetReducerSet[id] = true
	}

	// Add reducers corresponding to newly provided types (if they are reducers).
	for _, value := range newValuesTemplate {
		addReducerToReset(value.typeInfo.TypeID)
	}
	// Add reducers corresponding to types affected by resets (if they are reducers).
	for _, id := range resetIDs {
		addReducerToReset(id)
	}
	// Sort by the marker type ID for deterministic application in Fork.
	slices.SortFunc(resetReducers, func(a, b reducerInfo) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

	// This object contains all the information needed to quickly apply the fork.
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

// Fork applies the pre-calculated changes defined in the _Forker to create
// a new child scope based on the parent scope `s`.
func (f *_Forker) Fork(s Scope, defs []any) Scope {

	// new scope
	scope := Scope{
		signature:   f.Signature,
		forkFuncKey: f.Key,
		reducers:    f.Reducers,
	}

	// If the parent scope's stack is too deep, flatten it for performance.
	// This copies all values from the parent stack into a single new layer.
	if s.values != nil && s.values.Height > 16 {
		// flatten
		var values []_Value
		if err := s.values.Range(func(ds []_Value) error {
			values = append(values, ds...)
			return nil
		}); err != nil { // NOCOVER
			_ = throw(err)
		}
		slices.SortFunc(values, func(a, b _Value) int {
			return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
		})
		scope.values = &_StackedMap{
			Values: values,
		}
	} else {
		scope.values = s.values
	}

	// Create the actual `_Value` entries for the new definitions provided in `defs`.
	// These will form a new layer on the stack.
	newValues := make([]_Value, len(f.NewValuesTemplate))
	n := 0
	for idx, def := range defs {
		switch f.DefKinds[idx] {
		case reflect.Func:
			initializer := newInitializer(def, notReducer)
			numValues := f.DefNumValues[idx]
			for range numValues {
				info := f.NewValuesTemplate[n]
				newValues[f.PosesAtSorted[n]] = _Value{
					typeInfo:    info.typeInfo,
					initializer: initializer,
				}
				n++
			}
		case reflect.Pointer:
			info := f.NewValuesTemplate[n]
			initializer := newInitializer(def, notReducer)
			newValues[f.PosesAtSorted[n]] = _Value{
				typeInfo:    info.typeInfo,
				initializer: initializer,
			}
			n++
		}
	}
	// Add the layer containing the newly defined values.
	scope.values = scope.values.Append(newValues)

	// If any existing values need to be reset, create entries with fresh initializers.
	if len(f.ResetIDs) > 0 {
		resetValues := make([]_Value, 0, len(f.ResetIDs))
		for _, id := range f.ResetIDs {
			vs, ok := scope.values.Load(id)
			if !ok { // NOCOVER
				panic("impossible")
			}
			for _, value := range vs {
				if value.typeInfo.DefIsMulti {
					// multiple types using the same definition
					found := false
					for _, d := range resetValues {
						if d.initializer.ID == value.initializer.ID {
							found = true
							value.initializer = d.initializer
						}
					}
					if !found {
						value.initializer = value.initializer.reset()
					}
				} else {
					value.initializer = value.initializer.reset()
				}
				resetValues = append(resetValues, value)
			}
		}
		scope.values = scope.values.Append(resetValues) // resetValues is construct from f.ResetIDs which is sorted, so it's OK to append resetValues
	}

	// If any reducers need recalculation, add their marker values.
	if len(f.ResetReducers) > 0 {
		reducerValues := make([]_Value, 0, len(f.ResetReducers))
		for _, info := range f.ResetReducers {
			// Create an initializer specifically for the reducer calculation, using the original type.
			reducerValues = append(reducerValues, _Value{
				typeInfo:    info.typeInfo,
				initializer: newInitializer(info.originType, info.reducerKind),
			})
		}
		scope.values = scope.values.Append(reducerValues) // reducerValues is construct from ResetReducers which is sorted, so it's OK to append reducerValues
	}

	return scope
}
