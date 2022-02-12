package dscope

import (
	"encoding/binary"
	"hash/maphash"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/reusee/e4"
	"github.com/reusee/pr"
)

type _Value struct {
	*_ValueInfo
	Def         any
	Initializer *_Initializer
}

type _ValueInfo struct {
	Type       reflect.Type
	TypeID     _TypeID
	DefKind    reflect.Kind
	Position   int
	DefIsMulti bool
}

type _TypeID int

type Scope struct {
	reducers    map[_TypeID]reflect.Type
	signature   [2]uint64
	forkFuncKey [2]uint64
	values      *_StackedMap
	path        *Path
}

var Universe = Scope{}

func New(
	defs ...any,
) Scope {
	return Universe.Fork(defs...)
}

func (scope Scope) appendPath(t reflect.Type) Scope {
	path := &Path{
		Prev: scope.path,
		Type: t,
	}
	scope.path = path
	return scope
}

var forkers sync.Map

var (
	hashSeed  = maphash.MakeSeed()
	hashSeed2 = maphash.MakeSeed()
)

func (scope Scope) Fork(
	defs ...any,
) Scope {

	// get transition signature
	h := new(maphash.Hash)
	h2 := new(maphash.Hash)
	h.SetSeed(hashSeed)
	h2.SetSeed(hashSeed2)
	buf := make([]byte, 8)
	buf2 := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, scope.signature[0])
	binary.LittleEndian.PutUint64(buf, scope.signature[1])
	binary.LittleEndian.PutUint64(buf2, scope.signature[0])
	binary.LittleEndian.PutUint64(buf2, scope.signature[1])
	h.Write(buf)
	h2.Write(buf2)
	for _, def := range defs {
		id := getTypeID(reflect.TypeOf(def))
		binary.LittleEndian.PutUint64(buf, uint64(id))
		binary.LittleEndian.PutUint64(buf2, uint64(id))
		h.Write(buf)
		h2.Write(buf2)
	}
	key := [2]uint64{
		h.Sum64(),
		h2.Sum64(),
	}

	value, ok := forkers.Load(key)
	if ok {
		return value.(*_Forker).Fork(scope, defs)
	}

	forker := newForker(scope, defs, key)
	forkers.Store(key, forker)

	return forker.Fork(scope, defs)
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
		values, err = value.Initializer.Get(scope.appendPath(t))
		if err != nil { // NOCOVER
			return ret, err
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
	func() *[]reflect.Value {
		slice := make([]reflect.Value, reflectValuesPoolMaxLen)
		return &slice
	},
)

func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	var args []reflect.Value
	if nArgs := fnType.NumIn(); nArgs <= reflectValuesPoolMaxLen {
		ptr, put := reflectValuesPool.Get()
		defer put()
		args = *ptr
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
