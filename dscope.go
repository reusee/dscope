package dscope

import (
	"encoding/binary"
	"hash/maphash"
	"math"
	"reflect"
	"runtime"
	"sync"

	"github.com/reusee/e5"
	"github.com/reusee/pr"
)

type _Value struct {
	typeInfo    *_TypeInfo
	initializer *_Initializer
}

type _TypeInfo struct {
	TypeID     _TypeID
	DefType    reflect.Type
	Position   int
	DefIsMulti bool
}

type _TypeID int

type Scope struct {
	reducers    map[_TypeID]reflect.Type
	signature   complex128
	forkFuncKey complex128
	values      *_StackedMap
	path        *Path
}

var Universe = Scope{}

func New(
	defs ...any,
) Scope {
	return Universe.Fork(defs...)
}

func (scope Scope) appendPath(typeID _TypeID) Scope {
	path := &Path{
		Prev:   scope.path,
		TypeID: typeID,
	}
	scope.path = path
	return scope
}

var forkers = NewCowMap[complex128, *_Forker]()

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
	binary.LittleEndian.PutUint64(buf, math.Float64bits(real(scope.signature)))
	h.Write(buf)
	binary.LittleEndian.PutUint64(buf, math.Float64bits(imag(scope.signature)))
	h.Write(buf)
	binary.LittleEndian.PutUint64(buf2, math.Float64bits(real(scope.signature)))
	h.Write(buf2)
	binary.LittleEndian.PutUint64(buf2, math.Float64bits(imag(scope.signature)))
	h.Write(buf2)
	for _, def := range defs {
		id := getTypeID(reflect.TypeOf(def))
		binary.LittleEndian.PutUint64(buf, uint64(id))
		h.Write(buf)
		binary.LittleEndian.PutUint64(buf2, uint64(id))
		h2.Write(buf2)
	}
	key := complex(
		math.Float64frombits(h.Sum64()),
		math.Float64frombits(h2.Sum64()),
	)

	value, ok := forkers.Get(key)
	if ok {
		return value.Fork(scope, defs)
	}

	forker := newForker(scope, defs, key)
	forkers.Set(key, forker)

	return forker.Fork(scope, defs)
}

func (scope Scope) Assign(objs ...any) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Ptr {
			throw(we.With(
				e5.Info("%T is not a pointer", o),
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

func Assign[T any](scope Scope, ptr *T) {
	*ptr = Get[T](scope)
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
				e5.Info("no definition for %v", t),
			)(
				ErrDependencyNotFound,
			)
		}
		var values []reflect.Value
		values, err = value.initializer.get(scope, id)
		if err != nil { // NOCOVER
			return ret, err
		}
		return values[value.typeInfo.Position], nil

	} else {
		// reducer
		markType := getReducerMarkType(t, id)
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

func Get[T any](scope Scope) (o T) {
	value, err := scope.Get(reflect.TypeOf(o))
	if err != nil {
		throw(err)
	}
	return value.Interface().(T)
}

func (scope Scope) Call(fn any) CallResult {
	return scope.CallValue(reflect.ValueOf(fn))
}

var getArgsFunc sync.Map

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
		ptr, put, _ := reflectValuesPool.GetRC()
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
