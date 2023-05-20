package dscope

import (
	"encoding/binary"
	"hash/maphash"
	"math"
	"reflect"
	"runtime"
	"sync"

	"github.com/reusee/e5"
	"github.com/reusee/pr2"
)

type _Value struct {
	typeInfo    *_TypeInfo
	initializer *_Initializer
}

type _TypeInfo struct {
	DefType    reflect.Type
	TypeID     _TypeID
	Position   int
	DefIsMulti bool
}

type _TypeID int

type Scope struct {
	reducers    map[_TypeID]reflect.Type
	values      *_StackedMap
	path        *Path
	signature   complex128
	forkFuncKey complex128
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
	if _, err := h.Write(buf); err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(buf, math.Float64bits(imag(scope.signature)))
	if _, err := h.Write(buf); err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(buf2, math.Float64bits(real(scope.signature)))
	if _, err := h.Write(buf2); err != nil {
		panic(err)
	}
	binary.LittleEndian.PutUint64(buf2, math.Float64bits(imag(scope.signature)))
	if _, err := h.Write(buf2); err != nil {
		panic(err)
	}
	for _, def := range defs {
		id := getTypeID(reflect.TypeOf(def))
		binary.LittleEndian.PutUint64(buf, uint64(id))
		if _, err := h.Write(buf); err != nil {
			panic(err)
		}
		binary.LittleEndian.PutUint64(buf2, uint64(id))
		if _, err := h2.Write(buf2); err != nil {
			panic(err)
		}
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
			_ = throw(we.With(
				e5.Info("%T is not a pointer", o),
			)(
				ErrBadArgument,
			))
		}
		t := v.Type().Elem()
		value, err := scope.Get(t)
		if err != nil {
			_ = throw(err)
		}
		v.Elem().Set(value)
	}
}

func Assign[T any](scope Scope, ptr *T) {
	*ptr = Get[T](scope)
}

func (scope Scope) get(id _TypeID, t reflect.Type) (
	ret reflect.Value,
	err error,
) {

	if _, ok := scope.reducers[id]; !ok {
		// non-reducer

		value, ok := scope.values.LoadOne(id)
		if !ok {

			return ret, we.With(
				e5.Info("no definition for %v", t),
				e5.Info("path: %+v", scope.path),
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
		_ = throw(err)
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

var reflectValuesPool = pr2.NewPool(
	uint32(runtime.NumCPU()),
	func() *[]reflect.Value {
		slice := make([]reflect.Value, reflectValuesPoolMaxLen)
		return &slice
	},
).WithReset(
	pr2.ResetSlice[reflect.Value](reflectValuesPoolMaxLen, -1),
)

func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	var args []reflect.Value
	if nArgs := fnType.NumIn(); nArgs <= reflectValuesPoolMaxLen {
		var ptr *[]reflect.Value
		put := reflectValuesPool.Get(&ptr)
		defer put()
		args = *ptr
	} else {
		args = make([]reflect.Value, nArgs)
	}
	n, err := scope.getArgs(fnType, args)
	if err != nil {
		_ = throw(err)
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
