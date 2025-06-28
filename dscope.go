package dscope

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
)

type _Value struct {
	typeInfo    *_TypeInfo
	initializer *_Initializer
}

type _TypeInfo struct {
	DefType      reflect.Type
	TypeID       _TypeID
	Position     int
	Dependencies []_TypeID
}

// _TypeID is a unique identifier for a reflect.Type.
type _TypeID int

// _Hash is used for scope signatures and cache keys.
type _Hash [sha256.Size]byte

// Scope represents an immutable dependency injection container.
// Operations like Fork create new Scope values.
type Scope struct {
	// values points to the top layer of the immutable value stack (_StackedMap).
	values *_StackedMap
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

	// handle modules
	var moduleObjects []any
	for i := 0; i < len(defs); {
		if def, ok := defs[i].(isModule); ok {
			defs = slices.Replace(defs, i, i+1)
			moduleObjects = append(moduleObjects, def)
		} else {
			i++
		}
	}
	if len(moduleObjects) > 0 {
		defs = append(defs, Methods(moduleObjects...)...)
	}

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
	// h.Write (from sha256.New()) is not expected to return an error,
	// but check is included for robustness against potential future changes
	// or different hash.Hash implementations.
	if _, err := h.Write(buf); err != nil {
		panic(fmt.Errorf("unexpected error during hash calculation in Scope.Fork: %w", err))
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
			panic(errors.Join(
				fmt.Errorf("%T is not a pointer", o),
				ErrBadArgument,
			))
		}
		t := v.Type().Elem()
		value, ok := scope.Get(t)
		if !ok {
			throwErrDependencyNotFound(t)
		}
		if !v.IsNil() {
			v.Elem().Set(value)
		}
	}
}

// Assign is a type-safe generic wrapper for Scope.Assign for a single pointer.
func Assign[T any](scope Scope, ptr *T) {
	*ptr = Get[T](scope)
}

// get retrieves a value of a specific type ID and type from the scope.
// Internal function called by Get and Assign.
func (scope Scope) get(id _TypeID) (
	ret reflect.Value,
	ok bool,
) {

	// special types
	switch id {
	case injectStructTypeID:
		return reflect.ValueOf(scope.InjectStruct), true
	}

	value, ok := scope.values.Load(id)
	if !ok {
		return ret, false
	}

	return value.initializer.get(scope, value.typeInfo.Position), true
}

// Get retrieves a single value of the specified type `t` from the scope.
// It panics if the type is not found or if an error occurs during resolution.
// Use Assign or the generic Get[T] for safer retrieval.
func (scope Scope) Get(t reflect.Type) (
	ret reflect.Value,
	ok bool,
) {
	return scope.get(getTypeID(t))
}

// Get is a type-safe generic function to retrieve a single value of type T.
// It panics if the type is not found or if an error occurs during resolution.
func Get[T any](scope Scope) (o T) {
	typ := reflect.TypeFor[T]()
	value, ok := scope.Get(typ)
	if !ok {
		throwErrDependencyNotFound(typ)
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
func (scope Scope) getArgs(fnType reflect.Type, args []reflect.Value) int {
	if v, ok := getArgsFunc.Load(fnType); ok {
		return v.(func(Scope, []reflect.Value) int)(scope, args)
	}
	return scope.getArgsSlow(fnType, args)
}

// getArgsSlow generates and caches the argument-fetching logic for a function type.
func (scope Scope) getArgsSlow(fnType reflect.Type, args []reflect.Value) int {
	numIn := fnType.NumIn()
	ids := make([]_TypeID, numIn)
	for i := range numIn {
		t := fnType.In(i)
		ids[i] = getTypeID(t)
	}
	getArgs := func(scope Scope, args []reflect.Value) int {
		for i := range ids {
			var ok bool
			args[i], ok = scope.get(ids[i])
			if !ok {
				throwErrDependencyNotFound(typeIDToType(ids[i]))
			}
		}
		return numIn
	}
	v, _ := getArgsFunc.LoadOrStore(fnType, getArgs)
	return v.(func(Scope, []reflect.Value) int)(scope, args)
}

// Cache for mapping return types to their position index for a given function type.
// reflect.Type -> map[reflect.Type]int
var fnRetTypes sync.Map

const reflectValuesPoolMaxLen = 64

var reflectValuesPool = sync.Pool{
	New: func() any {
		return new([reflectValuesPoolMaxLen]reflect.Value)
	},
}

// CallValue executes the given function `fnValue` after resolving its arguments from the scope.
// It returns a CallResult containing the function's return values.
func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := fnValue.Type()
	var args []reflect.Value
	// Use pool for small number of arguments
	if nArgs := fnType.NumIn(); nArgs <= reflectValuesPoolMaxLen {
		ptr := reflectValuesPool.Get().(*[reflectValuesPoolMaxLen]reflect.Value)
		args = (*ptr)[:]
		defer func() {
			clear(args)
			reflectValuesPool.Put(ptr)
		}()
	} else {
		args = make([]reflect.Value, nArgs)
	}
	n := scope.getArgs(fnType, args)
	res.Values = fnValue.Call(args[:n])

	// Cache return type positions
	if v, ok := fnRetTypes.Load(fnType); ok {
		res.positionsByType = v.(map[reflect.Type][]int)
	} else {
		m := make(map[reflect.Type][]int)
		for i := range fnType.NumOut() {
			t := fnType.Out(i)
			m[t] = append(m[t], i)
		}
		actual, _ := fnRetTypes.LoadOrStore(fnType, m)
		res.positionsByType = actual.(map[reflect.Type][]int)
	}

	return
}
