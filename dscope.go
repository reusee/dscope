package dscope

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

type _TypeDecl struct {
	Kind       reflect.Kind
	Init       any
	Get        func(scope Scope) ([]reflect.Value, error)
	ValueIndex int
	Type       reflect.Type
	TypeID     _TypeID
	IsUnset    bool
}

type _TypeID int

var typeIDSize = int(unsafe.Sizeof(_TypeID(0)))

type Unset struct{}

var unsetTypeID = getTypeID(reflect.TypeOf((*Unset)(nil)).Elem())

type Scope struct {
	declarations UnionMap
	signature    string
	ID           int64
	ParentID     int64
	ChangedTypes map[reflect.Type]struct{}
	SubFuncKey   string
}

var nextID int64 = 42

var Root = Scope{
	ID:           0,
	ChangedTypes: make(map[reflect.Type]struct{}),
}

func New(
	inits ...any,
) Scope {
	return Root.Sub(inits...)
}

func cachedInit(init any) func(Scope) ([]reflect.Value, error) {
	var once sync.Once
	var values []reflect.Value
	var err error
	return func(scope Scope) ([]reflect.Value, error) {
		once.Do(func() {
			values, err = scope.Pcall(init)
		})
		return values, err
	}
}

var (
	scopeType = reflect.TypeOf((*Scope)(nil)).Elem()
)

func dumbScopeProvider() (_ Scope) { // NOCOVER
	return
}

var subFns sync.Map

func (s Scope) Sub(
	inits ...any,
) Scope {

	inits = append(inits, dumbScopeProvider)

	var buf strings.Builder
	buf.WriteString(s.signature)
	buf.WriteByte('-')
	for _, init := range inits {
		id := getTypeID(reflect.TypeOf(init))
		buf.WriteString(strconv.FormatInt(int64(id), 10))
		buf.WriteByte('.')
	}
	key := buf.String()

	if value, ok := subFns.Load(key); ok {
		return value.(func(Scope, []any) Scope)(s, inits)
	}

	// collect new decls
	var newDeclsTemplate []_TypeDecl
	shadowedIDs := make(map[_TypeID]struct{})
	initNumDecls := make([]int, 0, len(inits))
	initKinds := make([]reflect.Kind, 0, len(inits))
	changedTypes := make(map[reflect.Type]struct{})
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
							Type:   in,
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
						changedTypes[in] = struct{}{}
					}

				} else {
					// non-unset
					newDeclsTemplate = append(newDeclsTemplate, _TypeDecl{
						Kind:   reflect.Func,
						Type:   t,
						TypeID: id,
						Init:   init,
					})
					numDecls++
					if t != scopeType {
						if _, ok := s.declarations.Load(id); ok {
							shadowedIDs[id] = struct{}{}
						}
					}
					changedTypes[t] = struct{}{}
				}

			}
			initNumDecls = append(initNumDecls, numDecls)

		case reflect.Ptr:
			t := initType.Elem()
			id := getTypeID(t)
			newDeclsTemplate = append(newDeclsTemplate, _TypeDecl{
				Kind:   reflect.Ptr,
				Type:   t,
				TypeID: id,
				Init:   init,
			})
			if t != scopeType {
				if _, ok := s.declarations.Load(id); ok {
					shadowedIDs[id] = struct{}{}
				}
			}
			initNumDecls = append(initNumDecls, 1)
			changedTypes[t] = struct{}{}

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
	downstreams := make(map[_TypeID][]_TypeDecl)
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
					decl,
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

	initTypeIDs := make([]_TypeID, 0, declarationsTemplate.Len())
	declarationsTemplate.Range(func(decl _TypeDecl) {
		traverse(decl)
		t := reflect.TypeOf(decl.Init)
		id := getTypeID(t)
		i := sort.Search(len(initTypeIDs), func(i int) bool {
			return id >= initTypeIDs[i]
		})
		if i < len(initTypeIDs) {
			if initTypeIDs[i] == id {
				// existed
			} else {
				initTypeIDs = append(initTypeIDs[:i], append([]_TypeID{id}, initTypeIDs[i:]...)...)
			}
		} else {
			initTypeIDs = append(initTypeIDs, id)
		}
	})
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
			if _, ok := s.declarations.Load(downstream.TypeID); !ok {
				continue
			}
			if _, ok := set[downstream.TypeID]; !ok {
				resetIDs = append(resetIDs, downstream.TypeID)
				changedTypes[downstream.Type] = struct{}{}
			}
			set[downstream.TypeID] = struct{}{}
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
	fn := func(s Scope, inits []any) Scope {
		scope := Scope{
			signature:    signature,
			ID:           atomic.AddInt64(&nextID, 1),
			ParentID:     s.ID,
			ChangedTypes: changedTypes,
			SubFuncKey:   key,
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
						// use made func for unset decls
						initFunc = info.Init
					}
					newDecls[info.ValueIndex] = _TypeDecl{
						Kind:       info.Kind,
						Init:       initFunc,
						Get:        get,
						ValueIndex: i,
						Type:       info.Type,
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
					Get: func(scope Scope) ([]reflect.Value, error) {
						return []reflect.Value{v}, nil
					},
					ValueIndex: 0,
					Type:       info.Type,
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

		return scope
	}

	subFns.Store(key, fn)

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
		value, err := scope.Get(t)
		if err != nil {
			panic(err)
		}
		v.Elem().Set(value)
	}
}

func (scope Scope) getByID(id _TypeID, t reflect.Type) (
	ret reflect.Value,
	err error,
) {
	decl, ok := scope.declarations.Load(id)
	if !ok {
		return ret, ErrDependencyNotFound{
			Type: t,
		}
	}
	if decl.IsUnset {
		return ret, ErrDependencyNotFound{
			Type: t,
		}
	}
	values, err := decl.Get(scope)
	if err != nil {
		return ret, err
	}
	return values[decl.ValueIndex], nil
}

func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	err error,
) {
	return scope.getByID(getTypeID(t), t)
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

const maxPooledArgsSize = 500

const pooledArgsSizeStep = 5

var argsPools = func() []sync.Pool {
	numPools := maxPooledArgsSize / pooledArgsSizeStep
	var pools []sync.Pool
	for i := 0; i < numPools; i++ {
		size := (i + 1) * pooledArgsSizeStep
		pools = append(pools, sync.Pool{
			New: func() any {
				slice := make([]reflect.Value, size)
				return &slice
			},
		})
	}
	return pools
}()

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
			for i, id := range ids {
				var err error
				args[i], err = scope.getByID(id, types[i])
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

func (scope Scope) PcallValue(fnValue reflect.Value, retArgs ...any) ([]reflect.Value, error) {
	fnType := fnValue.Type()

	var args []reflect.Value
	numIn := fnType.NumIn()
	i := numIn / pooledArgsSizeStep
	if i < len(argsPools) {
		args = *argsPools[i].Get().(*[]reflect.Value)
		defer argsPools[i].Put(&args)
	} else {
		args = make([]reflect.Value, numIn)
	}

	n, err := scope.GetArgs(fnType, args)
	if err != nil {
		return nil, err
	}
	retValues := fnValue.Call(args[:n])

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
