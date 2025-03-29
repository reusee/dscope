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

// _TypeID is a unique identifier for a reflect.Type.
type _TypeID int

// _Hash is used for scope signatures and cache keys.
type _Hash [sha256.Size]byte

// Scope represents an immutable dependency injection container.
// Operations like Fork create new Scope values.
type Scope struct {
	// reducers maps TypeID to reflect.Type for types that have reducer semantics.
	reducers map[_TypeID]reflect.Type
	// values points to the top layer of the immutable value stack (_StackedMap).
	values *_StackedMap
	// path tracks the dependency resolution path to detect loops. Nil when not resolving.
	path *Path
	// signature is a hash representing the structural identity of this scope,
	// based on all definition types involved in its creation.
	signature _Hash
	// forkFuncKey is a cache key representing the specific Fork operation that created this scope.
	forkFuncKey _Hash
}

// Universe is the empty root scope.
var Universe = Scope{}

// New creates a new root Scope with the given definitions.
// Equivalent to Universe.Fork(defs...).
func New(
	defs ...any,
) Scope {
	return Universe.Fork(defs...)
}

// appendPath creates a new Scope with an extended dependency resolution path.
// Used internally during value initialization.
func (scope Scope) appendPath(typeID _TypeID) Scope {
	path := &Path{
		Prev:   scope.path,
		TypeID: typeID,
	}
	scope.path = path
	return scope
}

// forkers caches _Forker instances to speed up repeated Fork calls.
// The key is a _Hash derived from the parent scope signature and new def types.
// _Hash -> *_Forker
var forkers sync.Map

// Fork creates a new child scope by layering the given definitions (`defs`)
// on top of the current scope. It handles overriding existing definitions
// and ensures values are lazily initialized.
func (scope Scope) Fork(
	defs ...any,
) Scope {

	// sorting defs may reduce memory consumption if there're calls with same defs but different order
	// but sorting will increase heap allocations, causing performance drop

	// Calculate cache key for this Fork operation.
	// Key is based on parent signature and the types of new definitions.
	// Hashing types is sufficient as only one definition instance per type is effectively used.
	h := sha256.New() // use cryptographic hash to avoid collision
	h.Write(scope.signature[:])
	buf := make([]byte, 0, len(defs)*8)
	for _, def := range defs {
		id := getTypeID(reflect.TypeOf(def))
		buf = binary.NativeEndian.AppendUint64(buf, uint64(id))
	}
	if _, err := h.Write(buf); err != nil {
		panic(err)
	}
	var key _Hash
	h.Sum(key[:0])

	// Check cache
	v, ok := forkers.Load(key)
	if ok {
		return v.(*_Forker).Fork(scope, defs)
	}

	// Cache miss, create and cache forker
	forker := newForker(scope, defs, key)
	v, _ = forkers.LoadOrStore(key, forker)

	return v.(*_Forker).Fork(scope, defs)
}

// Assign retrieves values from the scope matching the types of the provided pointers
// and assigns the values to the pointers.
// It panics if any argument is not a pointer or if a required type is not found.
// It's safe to call Assign concurrently.
func (scope Scope) Assign(objects ...any) {
	for _, o := range objects {
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

// Assign is a type-safe generic wrapper for Scope.Assign for a single pointer.
func Assign[T any](scope Scope, ptr *T) {
	*ptr = Get[T](scope)
}

// get retrieves a value of a specific type ID and type from the scope.
// It handles the distinction between regular values and reducers.
// Internal function called by Get and Assign.
func (scope Scope) get(id _TypeID) (
	ret reflect.Value,
	err error,
) {

	// special methods
	switch id {
	case injectStructTypeID:
		return reflect.ValueOf(scope.InjectStruct), nil
	}

	if _, ok := scope.reducers[id]; !ok {
		// non-reducer
		value, ok := scope.values.LoadOne(id)
		if !ok {
			return ret, we.With(
				e5.Info("no definition for %v", typeIDToType(id)),
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
		// reducer: get the reduced value via its internal marker type
		markType := getReducerMarkType(id)
		return scope.get(
			getTypeID(markType),
		)
	}

}

// Get retrieves a single value of the specified type `t` from the scope.
// It panics if the type is not found or if an error occurs during resolution.
// Use Assign or the generic Get[T] for safer retrieval.
func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	err error,
) {
	return scope.get(getTypeID(t))
}

// Get is a type-safe generic function to retrieve a single value of type T.
// It panics if the type is not found or if an error occurs during resolution.
func Get[T any](scope Scope) (o T) {
	value, err := scope.Get(reflect.TypeFor[T]())
	if err != nil {
		_ = throw(err)
	}
	return value.Interface().(T)
}

// Call executes the given function `fn`, resolving its arguments from the scope.
// It returns a CallResult containing the return values of the function.
// Panics if argument resolution fails or if `fn` is not a function.
func (scope Scope) Call(fn any) CallResult {
	return scope.CallValue(reflect.ValueOf(fn))
}

// Cache for the argument-fetching logic for a given function type.
// reflect.Type -> func(Scope, []reflect.Value) (int, error)
var getArgsFunc sync.Map

// getArgs resolves the arguments for a function of type `fnType` from the scope
// and places them into the `args` slice. It returns the number of arguments resolved.
func (scope Scope) getArgs(fnType reflect.Type, args []reflect.Value) (int, error) {
	if v, ok := getArgsFunc.Load(fnType); ok {
		return v.(func(Scope, []reflect.Value) (int, error))(scope, args)
	}
	return scope.getArgsSlow(fnType, args)
}

// getArgsSlow generates and caches the argument-fetching logic for a function type.
func (scope Scope) getArgsSlow(fnType reflect.Type, args []reflect.Value) (int, error) {
	numIn := fnType.NumIn()
	ids := make([]_TypeID, numIn)
	for i := range numIn {
		t := fnType.In(i)
		ids[i] = getTypeID(t)
	}
	getArgs := func(scope Scope, args []reflect.Value) (int, error) {
		for i := range ids {
			var err error
			args[i], err = scope.get(ids[i])
			if err != nil {
				return 0, err
			}
		}
		return numIn, nil
	}
	getArgsFunc.Store(fnType, getArgs)
	return getArgs(scope, args)
}

// Cache for mapping return types to their position index for a given function type.
// reflect.Type -> map[reflect.Type]int
var fnRetTypes sync.Map

const reflectValuesPoolMaxLen = 64

var reflectValuesPool = pr3.NewPool(
	uint32(runtime.NumCPU()),
	func() []reflect.Value {
		return make([]reflect.Value, reflectValuesPoolMaxLen)
	},
)

// CallValue executes the given function `fnValue` after resolving its arguments from the scope.
// It returns a CallResult containing the function's return values.
func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	var args []reflect.Value
	// Use pool for small number of arguments
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

	// Cache return type positions
	v, ok := fnRetTypes.Load(fnType)
	if !ok {
		m := make(map[reflect.Type]int)
		for i := range fnType.NumOut() {
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
