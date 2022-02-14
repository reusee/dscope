package dscope

import (
	"encoding/binary"
	"hash/maphash"
	"reflect"
	"sort"

	"github.com/reusee/e4"
)

type _Forker struct {
	Signature         [2]uint64
	Key               [2]uint64
	Reducers          map[_TypeID]reflect.Type
	NewValuesTemplate []_Value
	DefKinds          []reflect.Kind
	DefNumValues      []int
	PosesAtSorted     []posAtSorted
	ResetIDs          []_TypeID
	ResetReducers     []reducerInfo
}

type posAtSorted int

type reducerInfo struct {
	*_ValueInfo
	OriginType  reflect.Type
	ReducerType *reflect.Type
}

func newForker(
	scope Scope,
	defs []any,
	key [2]uint64,
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
				throw(we.With(
					e4.Info("%T returns nothing", def),
				)(
					ErrBadArgument,
				))
			}
			var numValues int
			for i := 0; i < numOut; i++ {
				t := defType.Out(i)
				id := getTypeID(t)

				newValuesTemplate = append(newValuesTemplate, _Value{
					_ValueInfo: &_ValueInfo{
						Type:       t,
						TypeID:     id,
						DefType:    defType,
						Position:   i,
						DefIsMulti: numOut > 1,
					},
				})
				numValues++
				if id != scopeTypeID {
					if _, ok := scope.values.LoadOne(id); ok {
						redefinedIDs[id] = struct{}{}
					}
				}

			}
			defNumValues = append(defNumValues, numValues)

		case reflect.Ptr:
			t := defType.Elem()
			id := getTypeID(t)
			newValuesTemplate = append(newValuesTemplate, _Value{
				_ValueInfo: &_ValueInfo{
					Type:       t,
					TypeID:     id,
					DefType:    defType,
					DefIsMulti: false,
				},
			})
			if id != scopeTypeID {
				if _, ok := scope.values.LoadOne(id); ok {
					redefinedIDs[id] = struct{}{}
				}
			}

			defNumValues = append(defNumValues, 1)

		default:
			throw(we.With(
				e4.Info("%T is not a valid definition", def),
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
		return newValuesTemplate[posesAtTemplate[i]].TypeID < newValuesTemplate[posesAtTemplate[j]].TypeID
	})
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i)
	}

	sortedNewValuesTemplate := append(newValuesTemplate[:0:0], newValuesTemplate...)
	sort.Slice(sortedNewValuesTemplate, func(i, j int) bool {
		return sortedNewValuesTemplate[i].TypeID < sortedNewValuesTemplate[j].TypeID
	})
	valuesTemplate := scope.values.Append(sortedNewValuesTemplate)

	colors := make(map[_TypeID]int)
	downstreams := make(map[_TypeID][]_Value)
	var traverse func(values []_Value, path []reflect.Type) error
	traverse = func(values []_Value, path []reflect.Type) error {
		id := values[0].TypeID
		color := colors[id]
		if color == 1 {
			return we.With(
				e4.Info("found dependency loop in definition %v", values[0].DefType),
				e4.Info("path: %+v", path),
			)(
				ErrDependencyLoop,
			)
		} else if color == 2 {
			return nil
		}
		colors[id] = 1
		for _, value := range values {
			if value.DefType.Kind() != reflect.Func {
				continue
			}
			numIn := value.DefType.NumIn()
			for i := 0; i < numIn; i++ {
				requiredType := value.DefType.In(i)
				id2 := getTypeID(requiredType)
				downstreams[id2] = append(
					downstreams[id2],
					value,
				)
				if id2 != scopeTypeID {
					value2, ok := valuesTemplate.Load(id2)
					if !ok {
						return we.With(
							e4.Info("dependency not found in definition %v", value.DefType),
							e4.Info("no definition for %v", requiredType),
						)(
							ErrDependencyNotFound,
						)
					}
					if err := traverse(value2, append(path, value.Type)); err != nil {
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
			id := getTypeID(value.DefType)
			i := sort.Search(len(defTypeIDs), func(i int) bool {
				return id >= defTypeIDs[i]
			})
			if i < len(defTypeIDs) {
				if defTypeIDs[i] == id {
					// existed
				} else {
					defTypeIDs = append(
						defTypeIDs[:i],
						append([]_TypeID{id}, defTypeIDs[i:]...)...,
					)
				}
			} else {
				defTypeIDs = append(defTypeIDs, id)
			}
		}

		if len(values) > 1 {
			if getReducerType(values[0].Type) == nil {
				return we.With(
					e4.Info("%v has multiple definitions", values[0].Type),
				)(
					ErrBadDefinition,
				)
			}
			reducers[values[0].TypeID] = values[0].Type
		}

		return nil
	}); err != nil {
		throw(err)
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
		h.Write(buf)
		h2.Write(buf2)
	}
	signature := [2]uint64{
		h.Sum64(),
		h2.Sum64(),
	}

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
			if _, ok := scope.values.LoadOne(downstream.TypeID); !ok {
				continue
			}
			if _, ok := set[downstream.TypeID]; !ok {
				resetIDs = append(resetIDs, downstream.TypeID)
				set[downstream.TypeID] = struct{}{}
			}
			resetDownstream(downstream.TypeID)
		}
		colors[id] = 1
	}

	redefinedIDs[scopeTypeID] = struct{}{}
	for id := range redefinedIDs {
		resetDownstream(id)
	}

	sort.Slice(resetIDs, func(i, j int) bool {
		return resetIDs[i] < resetIDs[j]
	})

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
		markType := getReducerMarkType(t)
		resetReducers = append(resetReducers, reducerInfo{
			_ValueInfo: &_ValueInfo{
				Type:    markType,
				TypeID:  getTypeID(markType),
				DefType: reflect.TypeOf(func() {}),
			},
			OriginType:  t,
			ReducerType: getReducerType(t),
		})
		resetReducerSet[id] = true
	}
	// new reducers
	for _, value := range newValuesTemplate {
		resetReducer(value.TypeID)
	}
	// reset reducers
	for _, id := range resetIDs {
		resetReducer(id)
	}
	sort.Slice(resetReducers, func(i, j int) bool {
		return resetReducers[i].TypeID < resetReducers[j].TypeID
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
			throw(err)
		}
		sort.Slice(values, func(i, j int) bool {
			return values[i].TypeID < values[j].TypeID
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
			initializer := newInitializer(def, nil)
			numValues := f.DefNumValues[idx]
			for i := 0; i < numValues; i++ {
				info := f.NewValuesTemplate[n]
				newValues[f.PosesAtSorted[n]] = _Value{
					_ValueInfo:   info._ValueInfo,
					_Initializer: initializer,
				}
				n++
			}
		case reflect.Ptr:
			info := f.NewValuesTemplate[n]
			initializer := newInitializer(def, nil)
			newValues[f.PosesAtSorted[n]] = _Value{
				_ValueInfo:   info._ValueInfo,
				_Initializer: initializer,
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
				if value.DefIsMulti {
					// multiple types using the same definiton
					found := false
					for _, d := range resetValues {
						if d.ID == value.ID {
							found = true
							value._Initializer = d._Initializer
						}
					}
					if !found {
						value._Initializer = value._Initializer.reset()
					}
				} else {
					value._Initializer = value._Initializer.reset()
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
				_ValueInfo:   info._ValueInfo,
				_Initializer: newInitializer(info.OriginType, info.ReducerType),
			})
		}
		scope.values = scope.values.Append(reducerValues)
	}

	return scope
}
