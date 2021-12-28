package dscope

import (
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"reflect"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/reusee/e4"
	"github.com/reusee/pr"
)

type _Value struct {
	Def        any
	DefName    string
	DefIsMulti bool
	Type       reflect.Type
	Get        _Get
	Kind       reflect.Kind
	Position   int
	TypeID     _TypeID
}

type _Get struct {
	Func func(scope Scope) ([]reflect.Value, error)
	ID   int64
}

type _TypeID int

type Scope struct {
	reducers    map[_TypeID]reflect.Type
	signature   uint64
	forkFuncKey uint64
	values      _StackedMap
	path        *Path
}

var Universe = Scope{}

func New(
	defs ...any,
) Scope {
	return Universe.Fork(defs...)
}

var nextGetID int64 = 42

func cachedGet(
	def any,
	name string,
	_reducerType *reflect.Type,
) _Get {

	var once sync.Once
	var defValues []reflect.Value
	id := atomic.AddInt64(&nextGetID, 1)

	return _Get{
		ID: id,

		Func: func(scope Scope) (ret []reflect.Value, err error) {
			// detect dependency loop
			for p := scope.path.Prev; p != nil; p = p.Prev {
				if p.Type != scope.path.Type {
					continue
				}
				return nil, we.With(
					e4.Info("found dependency loop when calling %T / %s", def, name),
					e4.Info("path: %+v", scope.path),
				)(
					ErrDependencyLoop,
				)
			}

			once.Do(func() {

				// reducer
				if _reducerType != nil {
					typ := def.(reflect.Type)
					typeID := getTypeID(typ)
					values, ok := scope.values.Load(typeID)
					if !ok { // NOCOVER
						panic("impossible")
					}
					pathScope := scope.appendPath(typ)
					vs := make([]reflect.Value, len(values))
					names := make(DefNames, len(values))
					for i, value := range values {
						var values []reflect.Value
						values, err = value.Get.Func(pathScope)
						if err != nil { // NOCOVER
							return
						}
						if value.Position >= len(values) { // NOCOVER
							panic("impossible")
						}
						vs[i] = values[value.Position]
						names[i] = value.DefName
					}
					scope = scope.Fork(&names)
					switch *_reducerType {
					case reducerType:
						defValues = []reflect.Value{
							Reduce(vs),
						}
					case customReducerType:
						defValues = []reflect.Value{
							vs[0].Interface().(CustomReducer).Reduce(scope, vs),
						}
					}
					return
				}

				// non-reducer
				defValue := reflect.ValueOf(def)
				defKind := defValue.Kind()

				if defKind == reflect.Func {
					var result CallResult
					func() {
						defer he(&err, e4.WrapFunc(func(err error) error {
							// use a closure to avoid calling getName eagerly
							return e4.NewInfo("dscope: call %s", getName(name, def, false))(err)
						}))
						defer func() {
							if p := recover(); p != nil {
								pt("func: %s\n", getName(name, def, false))
								panic(p)
							}
						}()
						result = scope.Call(def)
					}()
					defValues = result.Values

				} else if defKind == reflect.Ptr {
					defValues = []reflect.Value{defValue.Elem()}
				}

			})

			return defValues, err
		},
	}
}

func (scope Scope) appendPath(t reflect.Type) Scope {
	path := &Path{
		Prev: scope.path,
		Type: t,
	}
	scope.path = path
	return scope
}

var forkFns sync.Map

var hashSeed = maphash.MakeSeed()

func (scope Scope) Fork(
	defs ...any,
) Scope {

	// get transition signature
	h := new(maphash.Hash)
	h.SetSeed(hashSeed)
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, scope.signature)
	h.Write(buf)
	for _, def := range defs {
		var id _TypeID
		if named, ok := def.(NamedDef); ok {
			id = getTypeID(reflect.TypeOf(named.Def))
		} else {
			id = getTypeID(reflect.TypeOf(def))
		}
		binary.LittleEndian.PutUint64(buf, uint64(id))
		h.Write(buf)
	}
	key := h.Sum64()

	value, ok := forkFns.Load(key)
	if ok {
		return value.(func(Scope, []any) Scope)(scope, defs)
	}

	// collect new values
	var newValuesTemplate []_Value
	redefinedIDs := make(map[_TypeID]struct{})
	defNumValues := make([]int, 0, len(defs))
	defKinds := make([]reflect.Kind, 0, len(defs))
	for _, def := range defs {
		var defValue any
		var defName string
		if named, ok := def.(NamedDef); ok {
			defValue = named.Def
			defName = named.Name
		} else {
			defValue = def
		}
		defType := reflect.TypeOf(defValue)
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
					Kind:       reflect.Func,
					Type:       t,
					TypeID:     id,
					Def:        defValue,
					DefName:    defName,
					DefIsMulti: numOut > 1,
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
				Kind:    reflect.Ptr,
				Type:    t,
				TypeID:  id,
				Def:     defValue,
				DefName: defName,
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
	type posAtSorted int
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i)
	}

	valuesTemplate := append(scope.values[:0:0], scope.values...)
	sortedNewValuesTemplate := append(newValuesTemplate[:0:0], newValuesTemplate...)
	sort.Slice(sortedNewValuesTemplate, func(i, j int) bool {
		return sortedNewValuesTemplate[i].TypeID < sortedNewValuesTemplate[j].TypeID
	})
	valuesTemplate = append(valuesTemplate, sortedNewValuesTemplate)

	colors := make(map[_TypeID]int)
	downstreams := make(map[_TypeID][]_Value)
	var traverse func(values []_Value, path []reflect.Type) error
	traverse = func(values []_Value, path []reflect.Type) error {
		id := values[0].TypeID
		color := colors[id]
		if color == 1 {
			return we.With(
				e4.Info("found dependency loop in definition %T / %v", values[0].Def, values[0].DefName),
				e4.Info("path: %+v", path),
			)(
				ErrDependencyLoop,
			)
		} else if color == 2 {
			return nil
		}
		colors[id] = 1
		for _, value := range values {
			if value.Kind != reflect.Func {
				continue
			}
			defType := reflect.TypeOf(value.Def)
			numIn := defType.NumIn()
			for i := 0; i < numIn; i++ {
				requiredType := defType.In(i)
				id2 := getTypeID(requiredType)
				downstreams[id2] = append(
					downstreams[id2],
					value,
				)
				if id2 != scopeTypeID {
					value2, ok := valuesTemplate.Load(id2)
					if !ok {
						return we.With(
							e4.Info("dependency not found in definition %T / %s", value.Def, value.DefName),
							e4.Info("path: %+v", path),
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
			t := reflect.TypeOf(value.Def)
			id := getTypeID(t)
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
	h.Reset()
	for _, id := range defTypeIDs {
		binary.LittleEndian.PutUint64(buf, uint64(id))
		h.Write(buf)
	}
	signature := h.Sum64()

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
	type ReducerInfo struct {
		TypeID     _TypeID
		Type       reflect.Type
		MarkType   reflect.Type
		MarkTypeID _TypeID
	}

	var resetReducers []ReducerInfo
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
		resetReducers = append(resetReducers, ReducerInfo{
			TypeID:     id,
			Type:       t,
			MarkType:   markType,
			MarkTypeID: getTypeID(markType),
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
		return resetReducers[i].MarkTypeID < resetReducers[j].MarkTypeID
	})

	// fn
	fn := func(s Scope, defs []any) Scope {

		// new scope
		scope := Scope{
			signature:   signature,
			forkFuncKey: key,
			reducers:    reducers,
		}

		// values
		var m _StackedMap
		if len(s.values) > 32 {
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
			m = _StackedMap{values}
		} else {
			m = make(_StackedMap, len(s.values), len(s.values)+3)
			copy(m, s.values)
		}

		// new values
		newValues := make([]_Value, len(newValuesTemplate))
		n := 0
		for idx, def := range defs {
			var defValue any
			var defName string
			if named, ok := def.(NamedDef); ok {
				defValue = named.Def
				defName = named.Name
			} else {
				defValue = def
			}
			switch defKinds[idx] {
			case reflect.Func:
				get := cachedGet(defValue, defName, nil)
				numValues := defNumValues[idx]
				for i := 0; i < numValues; i++ {
					info := newValuesTemplate[n]
					newValues[posesAtSorted[n]] = _Value{
						Kind:       info.Kind,
						Def:        defValue,
						DefName:    defName,
						DefIsMulti: info.DefIsMulti,
						Get:        get,
						Position:   i,
						Type:       info.Type,
						TypeID:     info.TypeID,
					}
					n++
				}
			case reflect.Ptr:
				info := newValuesTemplate[n]
				get := cachedGet(defValue, defName, nil)
				newValues[posesAtSorted[n]] = _Value{
					Kind:     info.Kind,
					Def:      defValue,
					DefName:  defName,
					Get:      get,
					Position: 0,
					Type:     info.Type,
					TypeID:   info.TypeID,
				}
				n++
			}
		}
		m = append(m, newValues)

		// reset values
		if len(resetIDs) > 0 {
			resetValues := make([]_Value, 0, len(resetIDs))
			for _, id := range resetIDs {
				vs, ok := m.Load(id)
				if !ok { // NOCOVER
					panic("impossible")
				}
				for _, value := range vs {
					if value.DefIsMulti {
						// multiple types using the same definiton
						found := false
						for _, d := range resetValues {
							if d.Get.ID == value.Get.ID {
								found = true
								value.Get = d.Get
							}
						}
						if !found {
							value.Get = cachedGet(value.Def, value.DefName,
								// no reducer type in resetIDs
								nil)
						}
					} else {
						value.Get = cachedGet(value.Def, value.DefName,
							// no reducer type in resetIDs
							nil)
					}
					resetValues = append(resetValues, value)
				}
			}
			m = append(m, resetValues)
		}

		// reducers
		if len(resetReducers) > 0 {
			reducerValues := make([]_Value, 0, len(resetReducers))
			for _, info := range resetReducers {
				info := info
				reducerValues = append(reducerValues, _Value{
					Def: func() { // NOCOVER
						panic("should not be called")
					},
					Type:     info.MarkType,
					Get:      cachedGet(info.Type, "", getReducerType(info.Type)),
					Kind:     reflect.Func,
					Position: 0,
					TypeID:   info.MarkTypeID,
				})
			}
			m = append(m, reducerValues)
		}

		scope.values = m

		return scope
	}

	forkFns.Store(key, fn)

	return fn(scope, defs)
}

func (scope Scope) Assign(objs ...any) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Ptr {
			throw(we.With(
				e4.Info("%T is not a pointer", o),
			)(
				ErrBadArgument,
			))
		}
		t := v.Type().Elem()
		value, err := scope.Get(t)
		if err != nil {
			throw(err)
		}
		v.Elem().Set(value)
	}
}

var (
	scopeTypeID = getTypeID(reflect.TypeOf((*Scope)(nil)).Elem())
)

func (scope Scope) get(id _TypeID, t reflect.Type) (
	ret reflect.Value,
	err error,
) {

	// built-ins
	switch id {
	case scopeTypeID:
		return reflect.ValueOf(scope), nil
	}

	if _, ok := scope.reducers[id]; !ok {
		// non-reducer
		value, ok := scope.values.LoadOne(id)
		if !ok {
			return ret, we.With(
				e4.Info("no definition for %v", t),
			)(
				ErrDependencyNotFound,
			)
		}
		var values []reflect.Value
		values, err = value.Get.Func(scope.appendPath(t))
		if err != nil { // NOCOVER
			return ret, err
		}
		if value.Position >= len(values) { // NOCOVER
			panic("impossible")
		}
		return values[value.Position], nil

	} else {
		// reducer
		markType := getReducerMarkType(t)
		return scope.get(
			getTypeID(markType),
			markType,
		)
	}

}

func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	err error,
) {
	return scope.get(getTypeID(t), t)
}

func (scope Scope) Call(fn any) CallResult {
	return scope.CallValue(reflect.ValueOf(fn))
}

func (scope Scope) getArgs(fnType reflect.Type, args []reflect.Value) (int, error) {
	var getArgs func(Scope, []reflect.Value) (int, error)
	if v, ok := getArgsFunc.Load(fnType); !ok {
		var types []reflect.Type
		var ids []_TypeID
		numIn := fnType.NumIn()
		for i := 0; i < numIn; i++ {
			t := fnType.In(i)
			types = append(types, t)
			ids = append(ids, getTypeID(t))
		}
		n := len(ids)
		getArgs = func(scope Scope, args []reflect.Value) (int, error) {
			for i := range ids {
				var err error
				args[i], err = scope.get(ids[i], types[i])
				if err != nil {
					return 0, err
				}
			}
			return n, nil
		}
		getArgsFunc.Store(fnType, getArgs)
	} else {
		getArgs = v.(func(Scope, []reflect.Value) (int, error))
	}
	return getArgs(scope, args)
}

var fnRetTypes sync.Map

const reflectValuesPoolMaxLen = 64

var reflectValuesPool = pr.NewPool(
	int32(runtime.NumCPU()),
	func() any {
		return make([]reflect.Value, reflectValuesPoolMaxLen)
	},
)

func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	var args []reflect.Value
	if nArgs := fnType.NumIn(); nArgs <= reflectValuesPoolMaxLen {
		v, put := reflectValuesPool.Get()
		defer put()
		args = v.([]reflect.Value)
	} else {
		args = make([]reflect.Value, nArgs)
	}
	n, err := scope.getArgs(fnType, args)
	if err != nil {
		throw(err)
	}
	res.Values = fnValue.Call(args[:n])
	v, ok := fnRetTypes.Load(fnType)
	if !ok {
		m := make(map[reflect.Type]int)
		for i := 0; i < fnType.NumOut(); i++ {
			t := fnType.Out(i)
			m[t] = i
		}
		res.positionsByType = m
		fnRetTypes.Store(fnType, m)
	} else {
		res.positionsByType = v.(map[reflect.Type]int)
	}
	return
}

func (scope Scope) FillStruct(ptr any) {
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Ptr ||
		v.Type().Elem().Kind() != reflect.Struct {
		throw(we.With(
			e4.NewInfo("expecting pointer to struct, got %T", ptr),
		)(ErrBadArgument))
	}
	v = v.Elem()
	for i, max := 0, v.NumField(); i < max; i++ {
		scope.Assign(v.Field(i).Addr().Interface())
	}
}

var getArgsFunc sync.Map

func (scope Scope) IsZero() bool {
	return len(scope.values) == 0
}

var (
	typeIDMap = func() atomic.Value {
		var v atomic.Value
		v.Store(make(map[reflect.Type]_TypeID))
		return v
	}()
	typeIDLock sync.Mutex
	nextTypeID _TypeID
)

func getTypeID(t reflect.Type) (r _TypeID) {
	m := typeIDMap.Load().(map[reflect.Type]_TypeID)
	if i, ok := m[t]; ok {
		return i
	}
	typeIDLock.Lock()
	m = typeIDMap.Load().(map[reflect.Type]_TypeID)
	if i, ok := m[t]; ok { // NOCOVER
		typeIDLock.Unlock()
		return i
	}
	newM := make(map[reflect.Type]_TypeID, len(m)+1)
	for k, v := range m {
		newM[k] = v
	}
	nextTypeID++
	newM[t] = _TypeID(nextTypeID)
	typeIDMap.Store(newM)
	typeIDLock.Unlock()
	return newM[t]
}

func getName(name string, value any, isReducer bool) string {
	if name == "" {
		if t, ok := value.(reflect.Type); ok { // reducer type
			name = fmt.Sprintf("%v", t)
		} else {
			name = fmt.Sprintf("%T", value)
		}
	}
	if isReducer {
		name = "reducer(" + name + ")"
	}
	return name
}
