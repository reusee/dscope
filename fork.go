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

type _Forker struct {
	Reducers          map[_TypeID]reflect.Type
	NewValuesTemplate []_Value
	DefKinds          []reflect.Kind
	DefNumValues      []int
	PosesAtSorted     []posAtSorted
	ResetIDs          []_TypeID     // sorted
	ResetReducers     []reducerInfo // sorted
	Signature         _Hash
	Key               _Hash
}

type posAtSorted int

type reducerInfo struct {
	typeInfo    *_TypeInfo
	originType  reflect.Type
	reducerKind reducerKind
}

func newForker(
	scope Scope,
	defs []any,
	key _Hash,
) *_Forker {

	// collect new values
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
					redefinedIDs[id] = struct{}{}
				}

			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Pointer:
			if defValue.IsNil() {
				_ = throw(we.With(
					e5.Info("%T nil pointer provided", def),
				)(
					ErrBadArgument,
				))
			}
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
				redefinedIDs[id] = struct{}{}
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
		posesAtSorted[j] = posAtSorted(i)
	}

	sortedNewValuesTemplate := slices.Clone(newValuesTemplate)
	slices.SortFunc(sortedNewValuesTemplate, func(a, b _Value) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)

	// loop detection and reset analysis
	colors := make(map[_TypeID]int)
	needsReset := make(map[_TypeID]bool)
	var traverse func(values []_Value, path []_TypeID) (reset bool, err error)
	traverse = func(values []_Value, path []_TypeID) (reset bool, err error) {
		id := values[0].typeInfo.TypeID
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

		colors[id] = 1
		defer func() {
			if err == nil {
				colors[id] = 2
				needsReset[id] = reset
			}
		}()

		// check if directly redefined
		if _, ok := redefinedIDs[id]; ok {
			reset = true
		}

		for _, value := range values {
			for _, depID := range value.typeInfo.Dependencies {
				value2, ok := valuesTemplate.Load(depID)
				if !ok {
					return false, we.With(
						e5.Info("dependency not found in definition %v", value.typeInfo.DefType),
						e5.Info("no definition for %v", typeIDToType(depID)),
						e5.Info("path: %+v", scope.path),
					)(
						ErrDependencyNotFound,
					)
				}
				depResets, err := traverse(value2, append(path, value.typeInfo.TypeID))
				if err != nil {
					return false, err
				}
				reset = reset || depResets
			}
		}

		return
	}

	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len())
	reducers := make(map[_TypeID]reflect.Type) // no need to copy scope.reducers here, since valuesTemplate.Range will iterate all types.
	if err := valuesTemplate.Range(func(values []_Value) error {
		if _, err := traverse(values, nil); err != nil {
			return err
		}

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

		if len(values) > 1 {
			t := typeIDToType(values[0].typeInfo.TypeID)
			if getReducerKind(t) == notReducer {
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

	h := sha256.New() // must be cryptographic hash to avoid collision
	buf := make([]byte, 0, len(defTypeIDs)*8)
	for _, id := range defTypeIDs {
		buf = binary.NativeEndian.AppendUint64(buf, uint64(id))
	}
	if _, err := h.Write(buf); err != nil {
		panic(err)
	}
	signature := *(*_Hash)(h.Sum(nil))

	// reset info
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

	// reducer infos
	resetReducers := make([]reducerInfo, 0, len(reducers))
	resetReducerSet := make(map[_TypeID]bool)
	resetReducer := func(id _TypeID) {
		if _, ok := resetReducerSet[id]; ok {
			return
		}
		t, ok := reducers[id]
		if !ok {
			return
		}
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
	// new reducers
	for _, value := range newValuesTemplate {
		resetReducer(value.typeInfo.TypeID)
	}
	// reset reducers
	for _, id := range resetIDs {
		resetReducer(id)
	}
	slices.SortFunc(resetReducers, func(a, b reducerInfo) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})

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

func (f *_Forker) Fork(s Scope, defs []any) Scope {

	// new scope
	scope := Scope{
		signature:   f.Signature,
		forkFuncKey: f.Key,
		reducers:    f.Reducers,
	}

	// values
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

	// new values
	newValues := make([]_Value, len(f.NewValuesTemplate))
	n := 0
	for idx, def := range defs {
		switch f.DefKinds[idx] {
		case reflect.Func:
			initializer := newInitializer(def, notReducer)
			numValues := f.DefNumValues[idx]
			for i := 0; i < numValues; i++ {
				info := f.NewValuesTemplate[n]
				newValues[f.PosesAtSorted[n]] = _Value{
					typeInfo:    info.typeInfo,
					initializer: initializer,
				}
				n++
			}
		case reflect.Ptr:
			info := f.NewValuesTemplate[n]
			initializer := newInitializer(def, notReducer)
			newValues[f.PosesAtSorted[n]] = _Value{
				typeInfo:    info.typeInfo,
				initializer: initializer,
			}
			n++
		}
	}
	scope.values = scope.values.Append(newValues)

	// reset values
	if len(f.ResetIDs) > 0 {
		resetValues := make([]_Value, 0, len(f.ResetIDs))
		for _, id := range f.ResetIDs {
			vs, ok := scope.values.Load(id)
			if !ok { // NOCOVER
				panic("impossible")
			}
			for _, value := range vs {
				if value.typeInfo.DefIsMulti {
					// multiple types using the same definiton
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

	// reducers
	if len(f.ResetReducers) > 0 {
		reducerValues := make([]_Value, 0, len(f.ResetReducers))
		for _, info := range f.ResetReducers {
			reducerValues = append(reducerValues, _Value{
				typeInfo:    info.typeInfo,
				initializer: newInitializer(info.originType, info.reducerKind),
			})
		}
		scope.values = scope.values.Append(reducerValues) // reducerValues is construct from ResetReducers which is sorted, so it's OK to append reducerValues
	}

	return scope
}
