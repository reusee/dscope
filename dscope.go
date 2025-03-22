package dscope

import (
	"crypto/sha256"
	"encoding/binary"
	"reflect"
	"runtime"
	"sync"

	"github.com/reusee/e5"
	"github.com/reusee/pr3"
)

type _Value struct {
	typeInfo    *_TypeInfo
	initializer *_Initializer
}

type _TypeInfo struct {
	DefType      reflect.Type
	TypeID       _TypeID
	Position     int
	DefIsMulti   bool
	Dependencies []_TypeID
}

type _TypeID int

type _Hash [sha256.Size]byte

type Scope struct {
	reducers    map[_TypeID]reflect.Type
	values      *_StackedMap
	path        *Path
	signature   _Hash
	forkFuncKey _Hash
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

// _Hash -> *_Forker
var forkers sync.Map

func (scope Scope) Fork(
	defs ...any,
) Scope {

	// sorting defs may reduce memory consumption if there're calls with same defs but different order
	// but sorting will increase heap allocations, causing performance drop

	// get transition signature
	h := sha256.New() // must be cryptographic hash to avoid collision
	h.Write(scope.signature[:])
	buf := make([]byte, 0, len(defs)*8)
	for _, def := range defs {
		id := getTypeID(reflect.TypeOf(def))
		buf = binary.NativeEndian.AppendUint64(buf, uint64(id))
	}
	if _, err := h.Write(buf); err != nil {
		panic(err)
	}
	key := *(*_Hash)(h.Sum(nil))

	v, ok := forkers.Load(key)
	if ok {
		return v.(*_Forker).Fork(scope, defs)
	}

	forker := newForker(scope, defs, key)
	v, _ = forkers.LoadOrStore(key, forker)

	return v.(*_Forker).Fork(scope, defs)
}

func (scope Scope) Assign(objs ...any) {
	for _, o := range objs {
		v := reflect.ValueOf(o)
		if v.Kind() != reflect.Pointer {
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
	if v, ok := getArgsFunc.Load(fnType); ok {
		return v.(func(Scope, []reflect.Value) (int, error))(scope, args)
	}
	return scope.getArgsSlow(fnType, args)
}

func (scope Scope) getArgsSlow(fnType reflect.Type, args []reflect.Value) (int, error) {
	numIn := fnType.NumIn()
	types := make([]reflect.Type, numIn)
	ids := make([]_TypeID, numIn)
	for i := range numIn {
		t := fnType.In(i)
		types[i] = t
		ids[i] = getTypeID(t)
	}
	getArgs := func(scope Scope, args []reflect.Value) (int, error) {
		for i := range ids {
			var err error
			args[i], err = scope.get(ids[i], types[i])
			if err != nil {
				return 0, err
			}
		}
		return numIn, nil
	}
	getArgsFunc.Store(fnType, getArgs)
	return getArgs(scope, args)
}

var fnRetTypes sync.Map

const reflectValuesPoolMaxLen = 64

var reflectValuesPool = pr3.NewPool(
	uint32(runtime.NumCPU()),
	func() []reflect.Value {
		return make([]reflect.Value, reflectValuesPoolMaxLen)
	},
)

func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	var args []reflect.Value
	if nArgs := fnType.NumIn(); nArgs <= reflectValuesPoolMaxLen {
		elem := reflectValuesPool.Get(&args)
		defer elem.Put()
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
