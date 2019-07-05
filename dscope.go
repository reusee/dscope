package dscope

import (
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/exp/rand"
)

func init() {
	var seed uint64
	binary.Read(crand.Reader, binary.LittleEndian, &seed)
	rand.Seed(seed)
}

type _TypeDecl struct {
	InitFunc   interface{}
	Get        func(scope Scope) []reflect.Value
	ValueIndex int
	TypeID     _TypeID
}

type _TypeID int64

var typeIDSize = int(unsafe.Sizeof(_TypeID(0)))

type Scope struct {
	declarations UnionMap
	initTypeSig  []byte
}

func New(
	inits ...interface{},
) Scope {
	return Scope{}.Sub(inits...)
}

func cachedInit(init interface{}) func(Scope) []reflect.Value {
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
	scopeType     = reflect.TypeOf((*Scope)(nil)).Elem()
	scopeTypeID   = getTypeID(scopeType)
	scopeInitType = reflect.TypeOf((*func() Scope)(nil)).Elem()
)

var subFuncs _SubFuncsMap

var subFuncsLock sync.RWMutex

type _SubFunc = func(Scope, []interface{}) Scope

func dumbScopeProvider() (_ Scope) { // NOCOVER
	return
}

func (s Scope) Sub(
	inits ...interface{},
) Scope {

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
	var shadowedIDs []_TypeID
	initNumOuts := make([]int, 0, len(inits))
	for _, init := range inits {
		initType := reflect.TypeOf(init)
		if initType.Kind() != reflect.Func {
			panic(fmt.Errorf("not func: %T", init))
		}
		numOut := initType.NumOut()
		if numOut == 0 {
			panic(fmt.Errorf("bad initializer: %T", init))
		}
		initNumOuts = append(initNumOuts, numOut)
		ids := make([]_TypeID, 0, numOut)
		for i := 0; i < numOut; i++ {
			t := initType.Out(i)
			id := getTypeID(t)
			ids = append(ids, id)
			newDeclsTemplate = append(newDeclsTemplate, _TypeDecl{
				TypeID:   id,
				InitFunc: init,
			})
			if t != scopeType {
				if _, ok := s.declarations.Load(id); ok {
					shadowedIDs = append(shadowedIDs, id)
				}
			}
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

	declarationsTemplate := make(UnionMap, len(s.declarations))
	copy(declarationsTemplate, s.declarations)
	sortedNewDeclsTemplate := make([]_TypeDecl, len(newDeclsTemplate))
	copy(sortedNewDeclsTemplate, newDeclsTemplate)
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
			panic(fmt.Errorf("dependency loop: %T", decl.InitFunc))
		} else if color == 2 {
			return
		}
		initType := reflect.TypeOf(decl.InitFunc)
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
				panic(fmt.Errorf("no declaration for %v required by %T", requiredType, decl.InitFunc))
			}
			downstreams[id2] = append(
				downstreams[id2],
				decl.TypeID,
			)
			traverse(decl2)
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
	for _, id := range shadowedIDs {
		resetDownstream(id)
	}
	sort.Slice(resetIDs, func(i, j int) bool {
		return resetIDs[i] < resetIDs[j]
	})

	// fn
	fn := func(s Scope, inits []interface{}) Scope {
		scope := Scope{
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
			get := cachedInit(init)
			numOut := initNumOuts[idx]
			for i := 0; i < numOut; i++ {
				info := newDeclsTemplate[n]
				newDecls[info.ValueIndex] = _TypeDecl{
					InitFunc:   init,
					Get:        get,
					ValueIndex: i,
					TypeID:     info.TypeID,
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
				decl.Get = cachedInit(decl.InitFunc)
				resetDecls = append(resetDecls, decl)
			}
			declarations = append(declarations, resetDecls)
		}

		scope.declarations = declarations

		return scope
	}

	subFuncsLock.Lock()
	subFuncs = subFuncs.Insert(_SubFuncsSpec{
		Sig1: s.initTypeSig,
		Sig2: sig,
		Func: fn,
	})
	subFuncsLock.Unlock()

	scope := fn(s, inits)

	return scope
}

func (scope Scope) Assign(objs ...interface{}) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Ptr {
			panic("must be a pointer")
		}
		t := v.Type().Elem()
		value, ok := scope.Get(t)
		if !ok {
			panic(fmt.Errorf("no declaration for %s", t.String()))
		}
		v.Elem().Set(value)
	}
}

func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	ok bool,
) {
	id := getTypeID(t)
	decl, ok := scope.declarations.Load(id)
	if !ok {
		return
	}
	return decl.Get(scope)[decl.ValueIndex], true
}

func (scope Scope) Call(fn interface{}, rets ...interface{}) []reflect.Value {
	return scope.CallValue(
		reflect.ValueOf(fn),
		rets...,
	)
}

func (scope Scope) CallValue(fnValue reflect.Value, retArgs ...interface{}) []reflect.Value {
	fnType := fnValue.Type()
	if h, ok := FastCalls[fnType]; ok {
		return h(scope, fnValue, retArgs)
	}
	numIn := fnType.NumIn()
	args := make([]reflect.Value, numIn)
	for i := 0; i < numIn; i++ {
		argType := fnType.In(i)
		argValue, ok := scope.Get(argType)
		if !ok {
			panic(fmt.Errorf("no declaration for %s", argType.String()))
		}
		args[i] = argValue
	}
	retValues := fnValue.Call(args)
	if len(retValues) > 0 && len(retArgs) > 0 {
		for _, retArg := range retArgs {
			v := reflect.ValueOf(retArg)
			t := v.Type()
			if t.Kind() != reflect.Ptr {
				panic(fmt.Errorf("return param is not pointer: %s", t.String()))
			}
			t = t.Elem()
			for _, retValue := range retValues {
				if retValue.Type() == t {
					v.Elem().Set(retValue)
					break
				}
			}
		}
	}
	return retValues
}

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
	i := _TypeID(rand.Int63())
	newM[t] = i
	typeIDMap.Store(newM)
	typeIDLock.Unlock()
	return i
}
