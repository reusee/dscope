package dscope

import (
	"encoding/binary"
	"hash/maphash"
	"math"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/reusee/e5"
)

type _Forker struct {
	Reducers          map[_TypeID]reflect.Type
	NewValuesTemplate []_Value
	DefKinds          []reflect.Kind
	DefNumValues      []int
	PosesAtSorted     []posAtSorted
	ResetIDs          []_TypeID
	ResetReducers     []reducerInfo
	Signature         complex128
	Key               complex128
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
	key complex128,
) *_Forker {

	// collect new values
	var newValuesTemplate []_Value
	redefinedIDs := make(map[_TypeID]struct{})
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for _, def := range defs {
		defType := reflect.TypeOf(def)
		defKinds = append(defKinds, defType.Kind())

		switch defType.Kind() {

		case reflect.Func:
			numOut := defType.NumOut()
			if numOut == 0 {
				_ = throw(we.With(
					e5.Info("%T returns nothing", def),
				)(
					ErrBadArgument,
				))
			}
			var numValues int
			for i := 0; i < numOut; i++ {
				t := defType.Out(i)
				id := getTypeID(t)

				newValuesTemplate = append(newValuesTemplate, _Value{
					typeInfo: &_TypeInfo{
						TypeID:     id,
						DefType:    defType,
						Position:   i,
						DefIsMulti: numOut > 1,
					},
				})
				numValues++
				if _, ok := scope.values.LoadOne(id); ok {
					redefinedIDs[id] = struct{}{}
				}

			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Ptr:
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
	sort.Slice(posesAtTemplate, func(i, j int) bool {
		return newValuesTemplate[posesAtTemplate[i]].typeInfo.TypeID <
			newValuesTemplate[posesAtTemplate[j]].typeInfo.TypeID
	})
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i)
	}

	sortedNewValuesTemplate := slices.Clone(newValuesTemplate)
	sort.Slice(sortedNewValuesTemplate, func(i, j int) bool {
		return sortedNewValuesTemplate[i].typeInfo.TypeID <
			sortedNewValuesTemplate[j].typeInfo.TypeID
	})
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)

	colors := make(map[_TypeID]int)
	downstreams := make(map[_TypeID][]_Value)
	var traverse func(values []_Value, path []_TypeID) error
	traverse = func(values []_Value, path []_TypeID) error {
		id := values[0].typeInfo.TypeID
		color := colors[id]
		switch color {
		case 1:
			return we.With(
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
			return nil
		}
		colors[id] = 1
		for _, value := range values {
			if value.typeInfo.DefType.Kind() != reflect.Func {
				continue
			}
			numIn := value.typeInfo.DefType.NumIn()
			for i := 0; i < numIn; i++ {
				var requiredTypes []reflect.Type
				inType := value.typeInfo.DefType.In(i)
				if inType.Implements(typeWrapperType) {
					requiredTypes = unwrapType(inType)
				} else {
					requiredTypes = []reflect.Type{inType}
				}
				for _, requiredType := range requiredTypes {
					id2 := getTypeID(requiredType)
					downstreams[id2] = append(
						downstreams[id2],
						value,
					)
					value2, ok := valuesTemplate.Load(id2)
					if !ok {
						return we.With(
							e5.Info("dependency not found in definition %v", value.typeInfo.DefType),
							e5.Info("no definition for %v", requiredType),
							e5.Info("path: %+v", scope.path),
						)(
							ErrDependencyNotFound,
						)
					}
					if err := traverse(value2, append(path, value.typeInfo.TypeID)); err != nil {
						return err
					}
				}
			}
		}
		colors[id] = 2
		return nil
	}

	defTypeIDs := make([]_TypeID, 0, valuesTemplate.Len())
	reducers := make(map[_TypeID]reflect.Type)
	if err := valuesTemplate.Range(func(values []_Value) error {
		if err := traverse(values, nil); err != nil {
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

	h := new(maphash.Hash)
	h2 := new(maphash.Hash)
	h.SetSeed(hashSeed)
	h2.SetSeed(hashSeed2)
	buf := make([]byte, 8)
	buf2 := make([]byte, 8)
	for _, id := range defTypeIDs {
		binary.LittleEndian.PutUint64(buf, uint64(id))
		binary.LittleEndian.PutUint64(buf2, uint64(id))
		if _, err := h.Write(buf); err != nil {
			panic(err)
		}
		if _, err := h2.Write(buf2); err != nil {
			panic(err)
		}
	}
	signature := complex(
		math.Float64frombits(h.Sum64()),
		math.Float64frombits(h2.Sum64()),
	)

	// reset info
	set := make(map[_TypeID]struct{})
	var resetIDs []_TypeID
	colors = make(map[_TypeID]int)
	var resetDownstream func(id _TypeID)
	resetDownstream = func(id _TypeID) {
		if colors[id] == 1 {
			return
		}
		for _, downstream := range downstreams[id] {
			if _, ok := scope.values.LoadOne(downstream.typeInfo.TypeID); !ok {
				continue
			}
			if _, ok := set[downstream.typeInfo.TypeID]; !ok {
				resetIDs = append(resetIDs, downstream.typeInfo.TypeID)
				set[downstream.typeInfo.TypeID] = struct{}{}
			}
			resetDownstream(downstream.typeInfo.TypeID)
		}
		colors[id] = 1
	}

	for id := range redefinedIDs {
		resetDownstream(id)
	}

	slices.Sort(resetIDs)

	// reducer infos
	var resetReducers []reducerInfo
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
				DefType: reflect.TypeOf(func() {}),
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
	sort.Slice(resetReducers, func(i, j int) bool {
		return resetReducers[i].typeInfo.TypeID < resetReducers[j].typeInfo.TypeID
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
	if s.values != nil && s.values.Height > 32 {
		// flatten
		var values []_Value
		if err := s.values.Range(func(ds []_Value) error {
			values = append(values, ds...)
			return nil
		}); err != nil { // NOCOVER
			_ = throw(err)
		}
		sort.Slice(values, func(i, j int) bool {
			return values[i].typeInfo.TypeID < values[j].typeInfo.TypeID
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
		scope.values = scope.values.Append(resetValues)
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
		scope.values = scope.values.Append(reducerValues)
	}

	return scope
}
