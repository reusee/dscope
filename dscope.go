package dscope

//TODO reducer也只计算一次

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/reusee/e4"
)

type _Decl struct {
	Init       any
	InitName   string
	Type       reflect.Type
	Get        _Get
	Kind       reflect.Kind
	ValueIndex int
	TypeID     _TypeID
}

type _Get struct {
	Func func(scope Scope) ([]reflect.Value, error)
	ID   int64
}

type _TypeID int

type Scope struct {
	reducers     map[_TypeID]struct{}
	signature    string
	subFuncKey   string
	declarations _UnionMap
	path         Path
}

var Universe = Scope{}

func New(
	inits ...any,
) Scope {
	return Universe.Sub(inits...)
}

var nextGetID int64 = 42

var logInit = os.Getenv("DSCOPE_LOG_INIT") != ""

func cachedInit(init any, name string) _Get {
	var once sync.Once
	var values []reflect.Value
	var err error
	id := atomic.AddInt64(&nextGetID, 1)
	fnName := name
	if fnName == "" {
		fnName = fmt.Sprintf("%T", init)
	}
	return _Get{
		ID: id,
		Func: func(scope Scope) ([]reflect.Value, error) {
			if len(scope.path) > 1 {
				last := scope.path[len(scope.path)-1]
				for _, elem := range scope.path[:len(scope.path)-1] {
					if elem == last {
						return nil, we(
							ErrDependencyLoop,
							e4.With(InitInfo{
								Value: init,
								Name:  name,
							}),
							e4.With(scope.path),
						)
					}
				}
			}
			once.Do(func() {
				if logInit { // NOCOVER
					t0 := time.Now()
					defer func() {
						debugLog("[DSCOPE] run %s in %v\n", fnName, time.Since(t0))
					}()
				}
				var result CallResult
				func() {
					defer he(&err, e4.WithInfo("dscope: call %s", fnName))
					result = scope.Call(init)
				}()
				values = result.Values
			})
			return values, err
		},
	}
}

func (s Scope) appendPath(t reflect.Type) Scope {
	s.path = append(
		append(s.path[:0:0], s.path...),
		t,
	)
	return s
}

var (
	scopeType = reflect.TypeOf((*Scope)(nil)).Elem()
)

func dumbScopeProvider() (_ Scope) { // NOCOVER
	return
}

var subFns sync.Map

var badScope = Scope{}

func (s Scope) Sub(
	initializers ...any,
) Scope {

	initializers = append(initializers, dumbScopeProvider)

	var buf strings.Builder
	buf.WriteString(s.signature)
	buf.WriteByte('-')
	for _, initializer := range initializers {
		var id _TypeID
		if named, ok := initializer.(NamedInit); ok {
			id = getTypeID(reflect.TypeOf(named.Value))
		} else {
			id = getTypeID(reflect.TypeOf(initializer))
		}
		buf.WriteString(strconv.FormatInt(int64(id), 10))
		buf.WriteByte('.')
	}
	key := buf.String()

	if value, ok := subFns.Load(key); ok {
		return value.(func(Scope, []any) Scope)(s, initializers)
	}

	// collect new decls
	var newDeclsTemplate []_Decl
	shadowedIDs := make(map[_TypeID]struct{})
	initNumDecls := make([]int, 0, len(initializers))
	initKinds := make([]reflect.Kind, 0, len(initializers))
	for _, initializer := range initializers {
		var initValue any
		var initName string
		if named, ok := initializer.(NamedInit); ok {
			initValue = named.Value
			initName = named.Name
		} else {
			initValue = initializer
		}
		initType := reflect.TypeOf(initValue)
		initKinds = append(initKinds, initType.Kind())

		switch initType.Kind() {

		case reflect.Func:
			numOut := initType.NumOut()
			if numOut == 0 {
				throw(we(
					ErrBadArgument,
					e4.With(ArgInfo{
						Value: initializer,
					}),
					e4.With(Reason("function returns nothing")),
				))
			}
			var numDecls int
			for i := 0; i < numOut; i++ {
				t := initType.Out(i)
				id := getTypeID(t)

				newDeclsTemplate = append(newDeclsTemplate, _Decl{
					Kind:     reflect.Func,
					Type:     t,
					TypeID:   id,
					Init:     initValue,
					InitName: initName,
				})
				numDecls++
				if t != scopeType {
					if _, ok := s.declarations.LoadOne(id); ok {
						if t.Implements(noShadowType) {
							throw(we(
								ErrBadShadow,
								e4.With(ArgInfo{
									Value: initializer,
								}),
								e4.With(TypeInfo{
									Type: t,
								}),
								e4.With(Reason(
									fmt.Sprintf("should not shadow %v", t),
								)),
							))
						}
						shadowedIDs[id] = struct{}{}
					}
				}

			}
			initNumDecls = append(initNumDecls, numDecls)

		case reflect.Ptr:
			t := initType.Elem()
			id := getTypeID(t)
			newDeclsTemplate = append(newDeclsTemplate, _Decl{
				Kind:     reflect.Ptr,
				Type:     t,
				TypeID:   id,
				Init:     initValue,
				InitName: initName,
			})
			if t != scopeType {
				if _, ok := s.declarations.LoadOne(id); ok {
					if t.Implements(noShadowType) {
						throw(we(
							ErrBadShadow,
							e4.With(ArgInfo{
								Value: initializer,
							}),
							e4.With(TypeInfo{
								Type: t,
							}),
							e4.With(Reason(
								fmt.Sprintf("should not shadow %v", t),
							)),
						))
					}
					shadowedIDs[id] = struct{}{}
				}
			}
			initNumDecls = append(initNumDecls, 1)

		default:
			throw(we(
				ErrBadArgument,
				e4.With(ArgInfo{
					Value: initializer,
				}),
				e4.With(Reason("not a function or a pointer")),
			))
		}
	}
	type posAtTemplate int
	posesAtTemplate := make([]posAtTemplate, 0, len(newDeclsTemplate))
	for i := range newDeclsTemplate {
		posesAtTemplate = append(posesAtTemplate, posAtTemplate(i))
	}
	sort.Slice(posesAtTemplate, func(i, j int) bool {
		return newDeclsTemplate[posesAtTemplate[i]].TypeID < newDeclsTemplate[posesAtTemplate[j]].TypeID
	})
	type posAtSorted int
	posesAtSorted := make([]posAtSorted, len(posesAtTemplate))
	for i, j := range posesAtTemplate {
		posesAtSorted[j] = posAtSorted(i)
	}

	declarationsTemplate := append(s.declarations[:0:0], s.declarations...)
	sortedNewDeclsTemplate := append(newDeclsTemplate[:0:0], newDeclsTemplate...)
	sort.Slice(sortedNewDeclsTemplate, func(i, j int) bool {
		return sortedNewDeclsTemplate[i].TypeID < sortedNewDeclsTemplate[j].TypeID
	})
	declarationsTemplate = append(declarationsTemplate, sortedNewDeclsTemplate)

	colors := make(map[_TypeID]int)
	downstreams := make(map[_TypeID][]_Decl)
	var traverse func(decls []_Decl, path []reflect.Type) error
	traverse = func(decls []_Decl, path []reflect.Type) error {
		id := decls[0].TypeID
		color := colors[id]
		if color == 1 {
			return we(
				ErrDependencyLoop,
				e4.With(InitInfo{
					Value: decls[0].Init,
					Name:  decls[0].InitName,
				}),
				e4.With(Path(append(path, decls[0].Type))),
			)
		} else if color == 2 {
			return nil
		}
		colors[id] = 1
		for _, decl := range decls {
			if decl.Kind != reflect.Func {
				continue
			}
			initType := reflect.TypeOf(decl.Init)
			numIn := initType.NumIn()
			for i := 0; i < numIn; i++ {
				requiredType := initType.In(i)
				id2 := getTypeID(requiredType)
				decls2, ok := declarationsTemplate.Load(id2)
				if !ok {
					return we(
						ErrDependencyNotFound,
						e4.With(TypeInfo{
							Type: requiredType,
						}),
						e4.With(InitInfo{
							Value: decl.Init,
							Name:  decl.InitName,
						}),
					)
				}
				downstreams[id2] = append(
					downstreams[id2],
					decl,
				)
				if err := traverse(decls2, append(path, decl.Type)); err != nil {
					return err
				}
			}
		}
		colors[id] = 2
		return nil
	}

	initTypeIDs := make([]_TypeID, 0, declarationsTemplate.Len())
	reducers := make(map[_TypeID]struct{})
	if err := declarationsTemplate.Range(func(decls []_Decl) error {
		if err := traverse(decls, nil); err != nil {
			return err
		}

		for _, decl := range decls {
			t := reflect.TypeOf(decl.Init)
			id := getTypeID(t)
			i := sort.Search(len(initTypeIDs), func(i int) bool {
				return id >= initTypeIDs[i]
			})
			if i < len(initTypeIDs) {
				if initTypeIDs[i] == id {
					// existed
				} else {
					initTypeIDs = append(
						initTypeIDs[:i],
						append([]_TypeID{id}, initTypeIDs[i:]...)...,
					)
				}
			} else {
				initTypeIDs = append(initTypeIDs, id)
			}
		}

		if len(decls) > 1 {
			reducers[decls[0].TypeID] = struct{}{}
			if !decls[0].Type.Implements(reducerType) {
				return we(
					ErrBadDeclaration,
					e4.With(TypeInfo{
						Type: decls[0].Type,
					}),
					e4.With(Reason("non-reducer type has multiple declarations")),
				)
			}
		}

		return nil
	}); err != nil {
		throw(err)
	}
	buf.Reset()
	for _, id := range initTypeIDs {
		buf.WriteString(strconv.FormatInt(int64(id), 10))
		buf.WriteByte('.')
	}
	signature := buf.String()

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
			if _, ok := s.declarations.LoadOne(downstream.TypeID); !ok {
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

	for id := range shadowedIDs {
		resetDownstream(id)
	}

	sort.Slice(resetIDs, func(i, j int) bool {
		return resetIDs[i] < resetIDs[j]
	})

	// fn
	fn := func(s Scope, initializers []any) Scope {

		// new scope
		scope := Scope{
			signature:  signature,
			subFuncKey: key,
			reducers:   reducers,
		}

		// declarations
		var declarations _UnionMap
		if len(s.declarations) > 32 {
			// flatten
			var decls []_Decl
			if err := s.declarations.Range(func(ds []_Decl) error {
				decls = append(decls, ds...)
				return nil
			}); err != nil { // NOCOVER
				throw(err)
			}
			sort.Slice(decls, func(i, j int) bool {
				return decls[i].TypeID < decls[j].TypeID
			})
			declarations = _UnionMap{decls}
		} else {
			declarations = make(_UnionMap, len(s.declarations), len(s.declarations)+2)
			copy(declarations, s.declarations)
		}

		// new decls
		newDecls := make([]_Decl, len(newDeclsTemplate))
		n := 0
		initializers[len(initializers)-1] = func() Scope { // NOCOVER
			panic("impposible")
		}
		for idx, initializer := range initializers {
			var initValue any
			var initName string
			if named, ok := initializer.(NamedInit); ok {
				initValue = named.Value
				initName = named.Name
			} else {
				initValue = initializer
			}
			switch initKinds[idx] {
			case reflect.Func:
				get := cachedInit(initValue, initName)
				numDecls := initNumDecls[idx]
				for i := 0; i < numDecls; i++ {
					info := newDeclsTemplate[n]
					initFunc := initValue
					newDecls[posesAtSorted[n]] = _Decl{
						Kind:       info.Kind,
						Init:       initFunc,
						InitName:   initName,
						Get:        get,
						ValueIndex: i,
						Type:       info.Type,
						TypeID:     info.TypeID,
					}
					n++
				}
			case reflect.Ptr:
				info := newDeclsTemplate[n]
				v := reflect.ValueOf(initValue).Elem()
				newDecls[posesAtSorted[n]] = _Decl{
					Kind:     info.Kind,
					Init:     initValue,
					InitName: initName,
					Get: _Get{
						ID: atomic.AddInt64(&nextGetID, 1),
						Func: func(scope Scope) ([]reflect.Value, error) {
							return []reflect.Value{v}, nil
						},
					},
					ValueIndex: 0,
					Type:       info.Type,
					TypeID:     info.TypeID,
				}
				n++
			}
		}
		declarations = append(declarations, newDecls)

		// reset decls
		if len(resetIDs) > 0 {
			resetDecls := make([]_Decl, 0, len(resetIDs))
			gets := make(map[int64]_Get)
			for _, id := range resetIDs {
				decls, ok := declarations.Load(id)
				if !ok { // NOCOVER
					panic("impossible")
				}
				for _, decl := range decls {
					get, ok := gets[decl.Get.ID]
					if !ok {
						get = cachedInit(decl.Init, decl.InitName)
						gets[decl.Get.ID] = get
					}
					decl.Get = get
					resetDecls = append(resetDecls, decl)
				}
			}
			declarations = append(declarations, resetDecls)
		}

		scope.declarations = declarations

		return scope
	}

	subFns.Store(key, fn)

	return fn(s, initializers)
}

func (scope Scope) Assign(objs ...any) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Ptr {
			throw(we(
				ErrBadArgument,
				e4.With(ArgInfo{
					Value: o,
				}),
				e4.With(Reason("must be a pointer")),
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

func (scope Scope) get(id _TypeID, t reflect.Type) (
	ret reflect.Value,
	err error,
) {

	if t == scopeType {
		return reflect.ValueOf(scope), nil
	}

	if _, ok := scope.reducers[id]; !ok {
		// non-reducer
		decl, ok := scope.declarations.LoadOne(id)
		if !ok {
			return ret, we(
				ErrDependencyNotFound,
				e4.With(TypeInfo{
					Type: t,
				}),
			)
		}
		var values []reflect.Value
		values, err = decl.Get.Func(scope.appendPath(t))
		if err != nil { // NOCOVER
			return ret, err
		}
		if decl.ValueIndex >= len(values) { // NOCOVER
			err = we(
				ErrBadDeclaration,
				e4.With(TypeInfo{
					Type: t,
				}),
				e4.With(Reason(fmt.Sprintf(
					"get %v from %T at %d",
					t,
					decl.Init,
					decl.ValueIndex,
				))),
			)
			return
		}
		return values[decl.ValueIndex], nil

	} else {
		// reducer
		decls, ok := scope.declarations.Load(id)
		if !ok { // NOCOVER
			panic("impossible")
		}
		pathScope := scope.appendPath(t)
		vs := make([]reflect.Value, len(decls))
		names := make(InitNames, len(decls))
		for i, decl := range decls {
			var values []reflect.Value
			values, err = decl.Get.Func(pathScope)
			if err != nil { // NOCOVER
				return
			}
			if decl.ValueIndex >= len(values) { // NOCOVER
				err = we(
					ErrBadDeclaration,
					e4.With(TypeInfo{
						Type: t,
					}),
					e4.With(Reason(fmt.Sprintf(
						"get %v from %T at %d",
						t,
						decl.Init,
						decl.ValueIndex,
					))),
				)
				return
			}
			vs[i] = values[decl.ValueIndex]
			names[i] = decl.InitName
		}
		scope = scope.Sub(&names)
		ret = vs[0].Interface().(Reducer).Reduce(scope, vs)
		return
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

func (scope Scope) GetArgs(fnType reflect.Type, args []reflect.Value) (int, error) {
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

func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	args := make([]reflect.Value, fnType.NumIn())
	n, err := scope.GetArgs(fnType, args)
	if err != nil {
		throw(err)
	}
	//TODO optimize: generate hint for static call
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

func (s Scope) Extend(t reflect.Type, inits ...any) Scope {
	if !t.Implements(reducerType) { // NOCOVER
		throw(we(
			ErrBadDeclaration,
			e4.With(TypeInfo{
				Type: t,
			}),
			e4.With(Reason("not a reducer type")),
		))
	}
	decls, _ := s.declarations.Load(getTypeID(t))
	for _, decl := range decls {
		inits = append(inits, decl.Init)
	}
	return s.Sub(inits...)
}

func (s Scope) RangePtrValues(
	fn func(reflect.Type, []any) error,
) error {
	return s.declarations.Range(func(decls []_Decl) error {
		t := decls[0].Type
		var vs []any
		for _, decl := range decls {
			if decl.Kind != reflect.Ptr {
				continue
			}
			vs = append(vs, decl.Init)
		}
		if len(vs) > 0 {
			fn(t, vs)
		}
		return nil
	})
}

var returnTypeMap sync.Map

var getArgsFunc sync.Map

func (scope Scope) IsZero() bool {
	return len(scope.declarations) == 0
}

var (
	typeIDMap = func() atomic.Value {
		var v atomic.Value
		v.Store(make(map[reflect.Type]_TypeID))
		return v
	}()
	typeIDLock sync.Mutex
	nextTypeID _TypeID // guarded by typeIDLock
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
	newM := make(map[reflect.Type]_TypeID, len(m))
	for k, v := range m {
		newM[k] = v
	}
	nextTypeID++
	id := nextTypeID
	newM[t] = id
	typeIDMap.Store(newM)
	typeIDLock.Unlock()
	return id
}
