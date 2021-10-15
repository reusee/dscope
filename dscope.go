package dscope

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/reusee/e4"
	"github.com/reusee/pr"
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
	reducers     map[_TypeID]reflect.Type
	signature    string
	forkFuncKey  string
	declarations _StackedMap
	path         Path
	proxy        []proxyEntry
	proxyPath    Path
}

type proxyEntry struct {
	TypeID _TypeID
	Func   reflect.Value
}

var Universe = Scope{}

func New(
	inits ...any,
) Scope {
	return Universe.Fork(inits...)
}

var nextGetID int64 = 42

var logInit = os.Getenv("DSCOPE_LOG_INIT") != ""

func cachedGet(
	init any,
	name string,
	isReducer bool,
) _Get {

	var once sync.Once
	var values []reflect.Value
	var err error
	id := atomic.AddInt64(&nextGetID, 1)

	var nameOnce sync.Once
	getName := func() string {
		nameOnce.Do(func() {
			if name == "" {
				if t, ok := init.(reflect.Type); ok { // reducer type
					name = fmt.Sprintf("%v", t)
				} else {
					name = fmt.Sprintf("%T", init)
				}
			}
			if isReducer {
				name = "reducer(" + name + ")"
			}
		})
		return name
	}

	return _Get{
		ID: id,

		Func: func(scope Scope) ([]reflect.Value, error) {
			if len(scope.path) > 1 {
				last := scope.path[len(scope.path)-1]
				for _, elem := range scope.path[:len(scope.path)-1] {
					if elem == last {
						return nil, we.With(
							e4.With(InitInfo{
								Value: init,
								Name:  name,
							}),
							e4.With(scope.path),
						)(
							ErrDependencyLoop,
						)
					}
				}
			}

			once.Do(func() {
				if logInit { // NOCOVER
					t0 := time.Now()
					defer func() {
						debugLog("[DSCOPE] run %s in %v\n", getName(), time.Since(t0))
					}()
				}

				// reducer
				if isReducer {
					typ := init.(reflect.Type)
					typeID := getTypeID(typ)
					decls, ok := scope.declarations.Load(typeID)
					if !ok { // NOCOVER
						panic("impossible")
					}
					pathScope := scope.appendPath(typ)
					vs := make([]reflect.Value, len(decls))
					names := make(InitNames, len(decls))
					for i, decl := range decls {
						var values []reflect.Value
						values, err = decl.Get.Func(pathScope)
						if err != nil { // NOCOVER
							return
						}
						if decl.ValueIndex >= len(values) { // NOCOVER
							err = we.With(
								e4.With(TypeInfo{
									Type: typ,
								}),
								e4.With(Reason(fmt.Sprintf(
									"get %v from %T at %d",
									typ,
									decl.Init,
									decl.ValueIndex,
								))),
							)(
								ErrBadDeclaration,
							)
							return
						}
						vs[i] = values[decl.ValueIndex]
						names[i] = decl.InitName
					}
					scope = scope.Fork(&names)
					values = []reflect.Value{
						vs[0].Interface().(Reducer).Reduce(scope, vs),
					}
					return
				}

				// non-reducer
				initValue := reflect.ValueOf(init)
				initKind := initValue.Kind()

				if initKind == reflect.Func {
					var result CallResult
					func() {
						defer he(&err, func(err error) error {
							return e4.NewInfo("dscope: call %s", getName())(err)
						})
						defer func() {
							if p := recover(); p != nil {
								pt("func: %s\n", getName())
								panic(p)
							}
						}()
						result = scope.Call(init)
					}()
					values = result.Values

				} else if initKind == reflect.Ptr {
					values = []reflect.Value{initValue.Elem()}
				}

			})

			return values, err
		},
	}
}

func (s Scope) appendPath(t reflect.Type) Scope {
	path := make(Path, len(s.path), len(s.path)+1)
	copy(path, s.path)
	path = append(path, t)
	s.path = path
	return s
}

func (s Scope) appendProxyPath(t reflect.Type) Scope {
	path := make(Path, len(s.proxyPath), len(s.proxyPath)+1)
	copy(path, s.proxyPath)
	path = append(path, t)
	s.proxyPath = path
	return s
}

var (
	scopeType = reflect.TypeOf((*Scope)(nil)).Elem()
)

type DependentScope struct {
	Scope
}

var dependentScopeType = reflect.TypeOf((*DependentScope)(nil)).Elem()

var dependentScopeTypeID = getTypeID(dependentScopeType)

type predefinedProvider func() (
	_ Scope,
	_ DependentScope,
	_ Fork,
	_ Assign,
	_ Get,
	_ Call,
	_ CallValue,
)

var predefinedTypeIDs = func() map[_TypeID]struct{} {
	m := make(map[_TypeID]struct{})
	t := reflect.TypeOf(predefinedProvider(nil))
	for i := 0; i < t.NumOut(); i++ {
		out := t.Out(i)
		m[getTypeID(out)] = struct{}{}
	}
	return m
}()

var forkFns sync.Map

var badScope = Scope{}

type Fork func(...any) Scope

var forkType = reflect.TypeOf((*Fork)(nil)).Elem()

func (s Scope) Fork(
	initializers ...any,
) Scope {

	// get transition signature
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
		buf.WriteString(strconv.Itoa(int(id)))
		buf.WriteByte('.')
	}
	key := buf.String()

	if value, ok := forkFns.Load(key); ok {
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
				throw(we.With(
					e4.With(ArgInfo{
						Value: initializer,
					}),
					e4.With(Reason("function returns nothing")),
				)(
					ErrBadArgument,
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
				if _, ok := predefinedTypeIDs[id]; !ok {
					if _, ok := s.declarations.LoadOne(id); ok {
						if t.Implements(noShadowType) {
							throw(we.With(
								e4.With(ArgInfo{
									Value: initializer,
								}),
								e4.With(TypeInfo{
									Type: t,
								}),
								e4.With(Reason(
									fmt.Sprintf("should not shadow %v", t),
								)),
							)(
								ErrBadShadow,
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
			if _, ok := predefinedTypeIDs[id]; !ok {
				if _, ok := s.declarations.LoadOne(id); ok {
					if t.Implements(noShadowType) {
						throw(we.With(
							e4.With(ArgInfo{
								Value: initializer,
							}),
							e4.With(TypeInfo{
								Type: t,
							}),
							e4.With(Reason(
								fmt.Sprintf("should not shadow %v", t),
							)),
						)(
							ErrBadShadow,
						))
					}
					shadowedIDs[id] = struct{}{}
				}
			}

			initNumDecls = append(initNumDecls, 1)

		default:
			throw(we.With(
				e4.With(ArgInfo{
					Value: initializer,
				}),
				e4.With(Reason("not a function or a pointer")),
			)(
				ErrBadArgument,
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
			return we.With(
				e4.With(InitInfo{
					Value: decls[0].Init,
					Name:  decls[0].InitName,
				}),
				e4.With(Path(append(path, decls[0].Type))),
			)(
				ErrDependencyLoop,
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
				downstreams[id2] = append(
					downstreams[id2],
					decl,
				)
				if _, ok := predefinedTypeIDs[id2]; !ok {
					// not pre-defined
					decls2, ok := declarationsTemplate.Load(id2)
					if !ok {
						return we.With(
							e4.With(TypeInfo{
								Type: requiredType,
							}),
							e4.With(InitInfo{
								Value: decl.Init,
								Name:  decl.InitName,
							}),
						)(
							ErrDependencyNotFound,
						)
					}
					if err := traverse(decls2, append(path, decl.Type)); err != nil {
						return err
					}
				}
			}
		}
		colors[id] = 2
		return nil
	}

	initTypeIDs := make([]_TypeID, 0, declarationsTemplate.Len())
	reducers := make(map[_TypeID]reflect.Type)
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
			reducers[decls[0].TypeID] = decls[0].Type
			if !decls[0].Type.Implements(reducerType) {
				return we.With(
					e4.With(TypeInfo{
						Type: decls[0].Type,
					}),
					e4.With(Reason("non-reducer type has multiple declarations")),
				)(
					ErrBadDeclaration,
				)
			}
		}

		return nil
	}); err != nil {
		throw(err)
	}
	buf.Reset()
	for _, id := range initTypeIDs {
		buf.WriteString(strconv.Itoa(int(id)))
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

	shadowedIDs[dependentScopeTypeID] = struct{}{}
	for id := range shadowedIDs {
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
	// new decl reducers
	for _, decl := range newDeclsTemplate {
		resetReducer(decl.TypeID)
	}
	// reset reducers
	for _, id := range resetIDs {
		resetReducer(id)
	}
	sort.Slice(resetReducers, func(i, j int) bool {
		return resetReducers[i].MarkTypeID < resetReducers[j].MarkTypeID
	})

	// fn
	fn := func(s Scope, initializers []any) Scope {

		// new scope
		scope := Scope{
			signature:   signature,
			forkFuncKey: key,
			reducers:    reducers,
		}
		if len(s.proxy) > 0 {
			proxy := make([]proxyEntry, len(s.proxy))
			copy(proxy, s.proxy)
			scope.proxy = proxy
			proxyPath := make(Path, len(s.proxyPath))
			copy(proxyPath, s.proxyPath)
			scope.proxyPath = proxyPath
		}

		// declarations
		var declarations _StackedMap
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
			declarations = _StackedMap{decls}
		} else {
			declarations = make(_StackedMap, len(s.declarations), len(s.declarations)+2)
			copy(declarations, s.declarations)
		}

		// new decls
		newDecls := make([]_Decl, len(newDeclsTemplate))
		n := 0
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
				get := cachedGet(initValue, initName, false)
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
				get := cachedGet(initValue, initName, false)
				newDecls[posesAtSorted[n]] = _Decl{
					Kind:       info.Kind,
					Init:       initValue,
					InitName:   initName,
					Get:        get,
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
			for _, id := range resetIDs {
				decls, ok := declarations.Load(id)
				if !ok { // NOCOVER
					panic("impossible")
				}
				for _, decl := range decls {
					found := false
					for _, d := range resetDecls {
						if d.Get.ID == decl.Get.ID {
							found = true
							decl.Get = d.Get
						}
					}
					if !found {
						decl.Get = cachedGet(decl.Init, decl.InitName,
							// no reducer type in resetIDs
							false)
					}
					resetDecls = append(resetDecls, decl)
				}
			}
			declarations = append(declarations, resetDecls)
		}

		// reducers
		if len(resetReducers) > 0 {
			reducerDecls := make([]_Decl, 0, len(resetReducers))
			for _, info := range resetReducers {
				info := info
				reducerDecls = append(reducerDecls, _Decl{
					Init: func() { // NOCOVER
						panic("should not be called")
					},
					Type:       info.MarkType,
					Get:        cachedGet(info.Type, "", true),
					Kind:       reflect.Func,
					ValueIndex: 0,
					TypeID:     info.MarkTypeID,
				})
			}
			declarations = append(declarations, reducerDecls)
		}

		scope.declarations = declarations

		return scope
	}

	forkFns.Store(key, fn)

	return fn(s, initializers)
}

type Assign func(...any)

var assignType = reflect.TypeOf((*Assign)(nil)).Elem()

func (scope Scope) Assign(objs ...any) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Ptr {
			throw(we.With(
				e4.With(ArgInfo{
					Value: o,
				}),
				e4.With(Reason("must be a pointer")),
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

func (scope Scope) get(id _TypeID, t reflect.Type) (
	ret reflect.Value,
	err error,
) {

	// proxy
	if len(scope.proxy) > 0 {
		for _, elem := range scope.proxyPath {
			if elem == t {
				goto skip_proxy
			}
		}
		n := sort.Search(len(scope.proxy), func(i int) bool {
			return scope.proxy[i].TypeID >= id
		})
		if n < len(scope.proxy) {
			entry := scope.proxy[n]
			if entry.TypeID == id {
				// proxied
				scope = scope.appendProxyPath(t)
				result := scope.CallValue(entry.Func)
				pos, ok := result.positionsByType[t]
				if !ok {
					panic("impossible")
				}
				return result.Values[pos], nil
			}
		}
	}
skip_proxy:

	// pre-defined
	switch t {
	case scopeType:
		return reflect.ValueOf(scope), nil
	case forkType:
		return reflect.ValueOf(scope.Fork), nil
	case assignType:
		return reflect.ValueOf(scope.Assign), nil
	case getType:
		return reflect.ValueOf(scope.Get), nil
	case callType:
		return reflect.ValueOf(scope.Call), nil
	case callValueType:
		return reflect.ValueOf(scope.CallValue), nil
	case dependentScopeType:
		return reflect.ValueOf(DependentScope{
			Scope: scope,
		}), nil
	}

	if _, ok := scope.reducers[id]; !ok {
		// non-reducer
		decl, ok := scope.declarations.LoadOne(id)
		if !ok {
			return ret, we.With(
				e4.With(TypeInfo{
					Type: t,
				}),
			)(
				ErrDependencyNotFound,
			)
		}
		var values []reflect.Value
		values, err = decl.Get.Func(scope.appendPath(t))
		if err != nil { // NOCOVER
			return ret, err
		}
		if decl.ValueIndex >= len(values) { // NOCOVER
			err = we.With(
				e4.With(TypeInfo{
					Type: t,
				}),
				e4.With(Reason(fmt.Sprintf(
					"get %v from %T at %d",
					t,
					decl.Init,
					decl.ValueIndex,
				))),
			)(
				ErrBadDeclaration,
			)
			return
		}
		return values[decl.ValueIndex], nil

	} else {
		// reducer
		markType := getReducerMarkType(t)
		return scope.get(
			getTypeID(markType),
			markType,
		)
	}

}

type Get func(reflect.Type) (reflect.Value, error)

var getType = reflect.TypeOf((*Get)(nil)).Elem()

func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	err error,
) {
	return scope.get(getTypeID(t), t)
}

type Call func(any) CallResult

var callType = reflect.TypeOf((*Call)(nil)).Elem()

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

type CallValue func(reflect.Value) CallResult

var callValueType = reflect.TypeOf((*CallValue)(nil)).Elem()

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

func (s *Scope) SetProxy(provideFuncs ...any) {
	for _, provideFunc := range provideFuncs {
		fnValue := reflect.ValueOf(provideFunc)
		fnType := fnValue.Type()
		if fnType.NumOut() == 0 {
			panic(fmt.Errorf("bad proxy func: %v", fnType))
		}
		for i := 0; i < fnType.NumOut(); i++ {
			typ := fnType.Out(i)
			typeID := getTypeID(typ)
			n := sort.Search(len(s.proxy), func(i int) bool {
				return s.proxy[i].TypeID >= typeID
			})
			if n >= len(s.proxy) {
				s.proxy = append(s.proxy, proxyEntry{
					TypeID: typeID,
					Func:   fnValue,
				})
			} else {
				if s.proxy[n].TypeID == typeID {
					panic(fmt.Errorf("duplicated proxy type: %v, by %v", typ, fnType))
				} else {
					entry := proxyEntry{
						TypeID: typeID,
						Func:   fnValue,
					}
					s.proxy = append(s.proxy[:n],
						append([]proxyEntry{entry}, s.proxy[n:]...)...)
				}
			}
		}
	}
}

func (s Scope) FillStruct(ptr any) {
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Ptr ||
		v.Type().Elem().Kind() != reflect.Struct {
		throw(we.With(
			e4.NewInfo("expecting pointer to struct, got %T", ptr),
		)(ErrBadArgument))
	}
	v = v.Elem()
	for i, max := 0, v.NumField(); i < max; i++ {
		s.Assign(v.Field(i).Addr().Interface())
	}
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
	ids        = make(map[int64]struct{})
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
	for {
		id := rand.Int63()
		if _, ok := ids[id]; ok { // NOCOVER
			continue
		}
		newM[t] = _TypeID(id)
		break
	}
	typeIDMap.Store(newM)
	typeIDLock.Unlock()
	return newM[t]
}
