package dscope

import (
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"
)

type _TypeDecl struct {
	Kind       reflect.Kind
	Init       any
	Get        func(scope Scope) []reflect.Value
	ValueIndex int
	TypeID     _TypeID
	IsUnset    bool
}

type _TypeID int

var typeIDSize = int(unsafe.Sizeof(_TypeID(0)))

type Unset struct{}

var unsetTypeID = getTypeID(reflect.TypeOf((*Unset)(nil)).Elem())

type Scope struct {
	declarations UnionMap
	initTypeSig  []byte
	ID           int64
}

var nextID int64 = 42

func New(
	inits ...any,
) Scope {
	return Scope{
		ID: atomic.AddInt64(&nextID, 1),
	}.Sub(inits...)
}

func cachedInit(init any) func(Scope) []reflect.Value {
	var once sync.Once
	var values []reflect.Value
	return func(scope Scope) []reflect.Value {
		once.Do(func() {
			values = scope.Call(init)
		})
		return values
	}
}

var (
	scopeType = reflect.TypeOf((*Scope)(nil)).Elem()
)

var subFuncs _SubFuncsMap

var subFuncsLock sync.RWMutex

type _SubFunc = func(Scope, []any) (
	Scope,
	map[reflect.Type]struct{},
)

func dumbScopeProvider() (_ Scope) { // NOCOVER
	return
}

func (s Scope) Sub(
	inits ...any,
) Scope {
	scope, _ := s.DetailedSub(inits...)
	return scope
}

func (s Scope) DetailedSub(
	inits ...any,
) (
	Scope,
	map[reflect.Type]struct{},
) {

	inits = append(inits, dumbScopeProvider)

	initTypeIDs := make([]_TypeID, 0, len(inits))
	for _, init := range inits {
		initTypeIDs = append(initTypeIDs, getTypeID(reflect.TypeOf(init)))
	}
	var sig []byte
	h := (*reflect.SliceHeader)(unsafe.Pointer(&sig))
	h.Len = len(initTypeIDs) * typeIDSize
	h.Data = uintptr(unsafe.Pointer(&initTypeIDs[0]))
	h.Cap = h.Len

	subFuncsLock.RLock()
	if spec, ok := subFuncs.Find(s.initTypeSig, sig); ok {
		subFuncsLock.RUnlock()
		return spec.Func(s, inits)
	}
	subFuncsLock.RUnlock()

	// collect new decls
	var newDeclsTemplate []_TypeDecl
	shadowedIDs := make(map[_TypeID]struct{})
	initNumDecls := make([]int, 0, len(inits))
	initKinds := make([]reflect.Kind, 0, len(inits))
	newDeclTypes := make(map[reflect.Type]struct{})
	for _, init := range inits {
		initType := reflect.TypeOf(init)
		initKinds = append(initKinds, initType.Kind())
		switch initType.Kind() {
		case reflect.Func:
			numOut := initType.NumOut()
			if numOut == 0 {
				panic(ErrBadArgument{
					Value:  init,
					Reason: "function returns nothing",
				})
			}
			var numDecls int
			for i := 0; i < numOut; i++ {
				t := initType.Out(i)
				id := getTypeID(t)

				if id == unsetTypeID {
					// unset decl
					numIn := initType.NumIn()
					for i := 0; i < numIn; i++ {
						in := initType.In(i)
						inID := getTypeID(in)
						newDeclsTemplate = append(newDeclsTemplate, _TypeDecl{
							Kind:   reflect.Func,
							TypeID: inID,
							Init: reflect.MakeFunc(
								reflect.FuncOf(
									[]reflect.Type{},
									[]reflect.Type{
										in,
									},
									false,
								),
								func([]reflect.Value) []reflect.Value {
									panic("impossible")
								},
							).Interface(),
							IsUnset: true,
						})
						numDecls++
						if in != scopeType {
							if _, ok := s.declarations.Load(inID); ok {
								shadowedIDs[inID] = struct{}{}
							}
						}
						newDeclTypes[in] = struct{}{}
					}

				} else {
					// non-unset
					newDeclsTemplate = append(newDeclsTemplate, _TypeDecl{
						Kind:   reflect.Func,
						TypeID: id,
						Init:   init,
					})
					numDecls++
					if t != scopeType {
						if _, ok := s.declarations.Load(id); ok {
							shadowedIDs[id] = struct{}{}
						}
					}
					newDeclTypes[t] = struct{}{}
				}

			}
			initNumDecls = append(initNumDecls, numDecls)

		case reflect.Ptr:
			t := initType.Elem()
			id := getTypeID(t)
			newDeclsTemplate = append(newDeclsTemplate, _TypeDecl{
				Kind:   reflect.Ptr,
				TypeID: id,
				Init:   init,
			})
			if t != scopeType {
				if _, ok := s.declarations.Load(id); ok {
					shadowedIDs[id] = struct{}{}
				}
			}
			initNumDecls = append(initNumDecls, 1)
			newDeclTypes[t] = struct{}{}

		default:
			panic(ErrBadArgument{
				Value:  init,
				Reason: "not a function or a pointer",
			})
		}
	}
	newDeclOrders := make([]int, 0, len(newDeclsTemplate))
	for i := range newDeclsTemplate {
		newDeclOrders = append(newDeclOrders, i)
	}
	sort.Slice(newDeclOrders, func(i, j int) bool {
		return newDeclsTemplate[newDeclOrders[i]].TypeID < newDeclsTemplate[newDeclOrders[j]].TypeID
	})
	for i, idx := range newDeclOrders {
		newDeclsTemplate[idx].ValueIndex = i
	}

	declarationsTemplate := append(s.declarations[:0:0], s.declarations...)
	sortedNewDeclsTemplate := append(newDeclsTemplate[:0:0], newDeclsTemplate...)
	sort.Slice(sortedNewDeclsTemplate, func(i, j int) bool {
		return sortedNewDeclsTemplate[i].TypeID < sortedNewDeclsTemplate[j].TypeID
	})
	declarationsTemplate = append(declarationsTemplate, sortedNewDeclsTemplate)

	colors := make(map[_TypeID]int)
	downstreams := make(map[_TypeID][]_TypeID)
	var traverse func(decl _TypeDecl)
	traverse = func(decl _TypeDecl) {
		color := colors[decl.TypeID]
		if color == 1 {
			panic(ErrDependencyLoop{
				Value: decl.Init,
			})
		} else if color == 2 {
			return
		}
		switch decl.Kind {
		case reflect.Func:
			initType := reflect.TypeOf(decl.Init)
			numIn := initType.NumIn()
			if numIn == 0 {
				colors[decl.TypeID] = 2
				return
			}
			colors[decl.TypeID] = 1
			for i := 0; i < numIn; i++ {
				requiredType := initType.In(i)
				id2 := getTypeID(requiredType)
				decl2, ok := declarationsTemplate.Load(id2)
				if !ok {
					panic(ErrDependencyNotFound{
						Type: requiredType,
						By:   decl.Init,
					})
				}
				downstreams[id2] = append(
					downstreams[id2],
					decl.TypeID,
				)
				traverse(decl2)
			}
		case reflect.Ptr:
			colors[decl.TypeID] = 2
			return
		default:
			panic("impossible")
		}
		colors[decl.TypeID] = 2
	}
	declarationsTemplate.Range(func(decl _TypeDecl) {
		traverse(decl)
	})

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
			if _, ok := s.declarations.Load(downstream); !ok {
				continue
			}
			if _, ok := set[downstream]; !ok {
				resetIDs = append(resetIDs, downstream)
			}
			set[downstream] = struct{}{}
			resetDownstream(downstream)
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
	fn := func(s Scope, inits []any) (Scope, map[reflect.Type]struct{}) {
		scope := Scope{
			ID:          atomic.AddInt64(&nextID, 1),
			initTypeSig: sig,
		}
		var declarations UnionMap
		if len(s.declarations) > 32 {
			// flatten
			var decls []_TypeDecl
			s.declarations.Range(func(decl _TypeDecl) {
				decls = append(decls, decl)
			})
			sort.Slice(decls, func(i, j int) bool {
				return decls[i].TypeID < decls[j].TypeID
			})
			declarations = UnionMap{decls}
		} else {
			declarations = make(UnionMap, len(s.declarations), len(s.declarations)+2)
			copy(declarations, s.declarations)
		}

		newDecls := make([]_TypeDecl, len(newDeclsTemplate))
		n := 0
		inits[len(inits)-1] = func() Scope {
			return scope
		}
		for idx, init := range inits {
			switch initKinds[idx] {
			case reflect.Func:
				get := cachedInit(init)
				numDecls := initNumDecls[idx]
				for i := 0; i < numDecls; i++ {
					info := newDeclsTemplate[n]
					initFunc := init
					if info.IsUnset {
						// use maked func for unset decls
						initFunc = info.Init
					}
					newDecls[info.ValueIndex] = _TypeDecl{
						Kind:       info.Kind,
						Init:       initFunc,
						Get:        get,
						ValueIndex: i,
						TypeID:     info.TypeID,
						IsUnset:    info.IsUnset,
					}
					n++
				}
			case reflect.Ptr:
				info := newDeclsTemplate[n]
				v := reflect.ValueOf(init).Elem()
				newDecls[info.ValueIndex] = _TypeDecl{
					Kind: info.Kind,
					Init: init,
					Get: func(scope Scope) []reflect.Value {
						return []reflect.Value{v}
					},
					ValueIndex: 0,
					TypeID:     info.TypeID,
					IsUnset:    info.IsUnset,
				}
				n++
			}
		}
		declarations = append(declarations, newDecls)

		if len(resetIDs) > 0 {
			resetDecls := make([]_TypeDecl, 0, len(resetIDs))
			for _, id := range resetIDs {
				decl, ok := declarations.Load(id)
				if !ok { // NOCOVER
					panic("impossible")
				}
				decl.Get = cachedInit(decl.Init)
				resetDecls = append(resetDecls, decl)
			}
			declarations = append(declarations, resetDecls)
		}

		scope.declarations = declarations

		return scope, newDeclTypes
	}

	subFuncsLock.Lock()
	subFuncs = subFuncs.Insert(_SubFuncsSpec{
		Sig1: s.initTypeSig,
		Sig2: sig,
		Func: fn,
	})
	subFuncsLock.Unlock()

	return fn(s, inits)
}

func (scope Scope) Assign(objs ...any) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Ptr {
			panic(ErrBadArgument{
				Value:  o,
				Reason: "must be a pointer",
			})
		}
		t := v.Type().Elem()
		value, ok := scope.Get(t)
		if !ok {
			panic(ErrDependencyNotFound{
				Type: t,
			})
		}
		v.Elem().Set(value)
	}
}

func (scope Scope) GetByID(id _TypeID) (
	ret reflect.Value,
	ok bool,
) {
	decl, ok := scope.declarations.Load(id)
	if !ok {
		return
	}
	if decl.IsUnset {
		ok = false
		return
	}
	return decl.Get(scope)[decl.ValueIndex], true
}

func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	ok bool,
) {
	return scope.GetByID(getTypeID(t))
}

func (scope Scope) Call(fn any, rets ...any) []reflect.Value {
	return scope.CallValue(
		reflect.ValueOf(fn),
		rets...,
	)
}

func (scope Scope) Pcall(fn any, rets ...any) ([]reflect.Value, error) {
	return scope.PcallValue(
		reflect.ValueOf(fn),
		rets...,
	)
}

func (scope Scope) CallValue(fnValue reflect.Value, retArgs ...any) []reflect.Value {
	rets, err := scope.PcallValue(fnValue, retArgs...)
	if err != nil {
		panic(err)
	}
	return rets
}

func (scope Scope) PcallValue(fnValue reflect.Value, retArgs ...any) ([]reflect.Value, error) {
	fnType := fnValue.Type()

	var getArgs func(Scope) ([]reflect.Value, error)
	if v, ok := getArgsFunc.Load(fnType); !ok {
		var types []reflect.Type
		var ids []_TypeID
		numIn := fnType.NumIn()
		for i := 0; i < numIn; i++ {
			t := fnType.In(i)
			types = append(types, t)
			ids = append(ids, getTypeID(t))
		}
		getArgs = func(scope Scope) ([]reflect.Value, error) {
			ret := make([]reflect.Value, len(ids))
			var ok bool
			for i, id := range ids {
				ret[i], ok = scope.GetByID(id)
				if !ok {
					return nil, ErrDependencyNotFound{
						Type: types[i],
					}
				}
			}
			return ret, nil
		}
		getArgsFunc.Store(fnType, getArgs)
	} else {
		getArgs = v.(func(Scope) ([]reflect.Value, error))
	}
	args, err := getArgs(scope)
	if err != nil {
		return nil, err
	}
	retValues := fnValue.Call(args)

	if len(retValues) > 0 && len(retArgs) > 0 {
		var m map[reflect.Type]int
		v, ok := returnTypeMap.Load(fnType)
		if !ok {
			m = make(map[reflect.Type]int)
			for i := 0; i < fnType.NumOut(); i++ {
				m[fnType.Out(i)] = i
			}
			returnTypeMap.Store(fnType, m)
		} else {
			m = v.(map[reflect.Type]int)
		}
		for _, retArg := range retArgs {
			v := reflect.ValueOf(retArg)
			t := v.Type()
			if t.Kind() != reflect.Ptr {
				panic(ErrBadArgument{
					Value:  retArg,
					Reason: "must be a pointer",
				})
			}
			if i, ok := m[t.Elem()]; ok {
				v.Elem().Set(retValues[i])
			}
		}
	}

	return retValues, nil
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
	nextTypeID _TypeID = 42 // guarded by typeIDLock
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
