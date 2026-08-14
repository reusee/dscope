package dscope

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
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

// TheoryOfScopeCore documents the fundamental model of dscope: an immutable,
// type-keyed container of lazily evaluated definitions.
const TheoryOfScopeCore = `
dscope core theory:
- A Scope is an immutable, type-keyed container. Every provided value is
  identified by its declared Go type — concrete or interface — and a type has
  at most one effective definition per scope.
- A definition is a provider function (parameters are dependencies, results
  are the provided values) or a pointer (the pointed-at value is copied into
  the scope at construction time).
- Fork layers new definitions onto a scope and returns a new scope: a branch
  of the same definition lineage. Scopes have no child-parent relationship:
  the original scope is never mutated, and the innermost definition of a
  type is the effective one in each branch.
- Resolution is by exact declared type: a provider returning an interface
  satisfies requests for that interface, not requests for the concrete value
  it holds; no implicit conversions are performed.
- Providers are lazy: a provider evaluates at most once per cached value, on
  first access, and the cached result is shared by all consumers. Overriding
  a type or resetting a scope installs fresh caches for the affected types.
- Three built-in dependencies — InjectStruct, Fork, Reset — are always
  available, bound to the current scope, and cannot be overridden: they are
  the escape hatches through which providers interact with the scope
  dynamically.
- Every public operation — Get, TryGet, Assign, Call, InjectStruct, AllTypes,
  ToDOT — reflects the effective definitions of the scope it is invoked on.
`

// TheoryOfScopeDefinitions documents the accepted definition forms and the
// validation applied to them during scope construction.
const TheoryOfScopeDefinitions = `
dscope definition theory:
- A definition is a provider function (its parameters are dependencies resolved
  from the scope, its results are the provided values) or a pointer to a value
  (the pointed-at value is copied into the scope at construction time).
- A provider may return multiple values; each result type becomes a provided
  type of the scope, and a single evaluation feeds all of them.
- Modules are not definitions themselves: a module passed to New or Fork is
  expanded into its exported methods, which then act as provider functions.
- Public scope construction validates every definition before deriving type
  identity: nil definitions, nil function or pointer definitions, functions
  that return nothing, and non-function non-pointer values are rejected.
- Two definitions in the same Fork call must not produce the same type;
  redefining an inherited type is the override mechanism of a later Fork layer.
- Invalid definitions produce structured dscope errors rather than leaking
  reflection, hashing, or storage implementation panics.
`

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
// The key is a _Hash derived from the base scope signature and new def types.
// _Hash -> *_Forker
var forkers sync.Map

// Fork creates a new scope by layering the given definitions (`defs`) on top
// of the current scope's definitions. The result is a new branch of the same
// definition lineage: scopes have no child-parent relationship, and the
// original scope is never mutated. Fork handles overriding existing
// definitions and ensures values are lazily initialized.
//
// Definitions can be provider functions or pointers to values. When a pointer is
// provided, the value it points to is copied; subsequent changes to the original
// variable will not affect the value in the scope. To provide a shared singleton,
// use a provider function that returns a pointer.
func (scope Scope) Fork(
	defs ...any,
) Scope {

	// Validate before reflection and cache-key generation so malformed public
	// input cannot leak implementation-specific panics.
	for _, def := range defs {
		if def == nil {
			panic(errors.Join(
				fmt.Errorf("nil definition"),
				ErrBadArgument,
			))
		}
	}

	// handle modules
	var moduleObjects []any
	for _, def := range defs {
		if module, ok := def.(isModule); ok {
			moduleObjects = append(moduleObjects, module)
		}
	}

	if len(moduleObjects) > 0 {
		// If we have modules, we need to construct a new defs slice
		// to avoid modifying the caller's slice.
		newDefs := make([]any, 0, len(defs))
		for _, def := range defs {
			if _, ok := def.(isModule); !ok {
				newDefs = append(newDefs, def)
			}
		}
		defs = append(newDefs, Methods(moduleObjects...)...)
	}

	// sorting defs may reduce memory consumption if there're calls with same defs but different order
	// but sorting will increase heap allocations, causing performance drop

	// Calculate cache key for this Fork operation.
	// Key is based on the base scope signature and the types of new definitions.
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

// TheoryOfScopeReset documents the semantics and typical use of Reset.
const TheoryOfScopeReset = `
dscope reset theory:
- Reset returns a new scope in which every cached provider result is
  invalidated; the original scope is unaffected.
- Reset is O(1): it installs a lazy reset layer over the existing value stack.
  Fresh initializers are created on demand, only for types that are actually
  accessed; untouched types incur zero overhead.
- The reset layer caches fresh initializers so each provider still evaluates at
  most once within the reset scope.
- Appending to (Forking from) a reset scope materialises the layer into a flat
  stack, preserving correct dependency-analysis invariants.
- Typical use: keep the definitions but drop every cached result, either to
  observe fresh provider evaluation in tests, or to re-run the graph after
  external state (files, clocks, globals) has changed.
`

// Reset returns a new Scope in which every value will be recomputed the next
// time it is requested. The original scope is unaffected.
//
// Reset is O(1): it wraps the value stack in a lazy reset layer rather than
// eagerly iterating all definitions. Fresh initializers are created on demand
// only for types that are actually accessed, so untouched types incur zero
// overhead. Once a provider is re-evaluated in the reset scope the result is
// cached, preserving the at-most-once evaluation guarantee.
func (scope Scope) Reset() Scope {
	if scope.values == nil {
		return scope
	}
	return Scope{
		values: &_StackedMap{
			ResetBase:  scope.values,
			ResetCache: new(sync.Map),
			Height:     1,
		},
		signature:   scope.signature,
		forkFuncKey: scope.forkFuncKey,
	}
}

// TheoryOfScopeAssignment documents the retrieval semantics shared by the
// assignment entry points.
const TheoryOfScopeAssignment = `
dscope assignment theory:
- Values are retrieved from a scope through typed pointers: Scope.Assign[T]
  resolves T from the scope and writes the value; Scope.Get[T] returns the
  value directly.
- A missing type panics with a structured dependency-not-found error;
  Scope.TryGet[T] is the non-panicking variant: it returns (zero, false) for
  a missing type, while provider panics still propagate.
- Reflection-based retrieval mirrors the generic forms: Scope.GetType and
  Scope.TryGetType take a reflect.Type and return a reflect.Value; GetType
  panics on a missing type, TryGetType returns (zero Value, false), and a
  nil type is a bad argument.
- CallResult.Assign matches return values to targets by type, preferring exact
  matches over assignable (interface) matches; CallResult.Extract assigns by
  position.
- Assign[T] accepts a single typed pointer; a nil pointer is a bad argument
  and must produce a structured dscope error instead of leaking a runtime or
  reflection panic. CallResult.Assign and CallResult.Extract validate nil and
  non-pointer targets for the same reason.
`

// Assign retrieves the value of type T from the scope and writes it to the
// provided pointer. It panics if ptr is nil or if the type is not found.
// It's safe to call Assign concurrently.
func (scope Scope) Assign[T any](ptr *T) {
	if ptr == nil {
		panic(errors.Join(
			fmt.Errorf("cannot assign to a nil pointer target of type %T", ptr),
			ErrBadArgument,
		))
	}
	*ptr = scope.Get[T]()
}

func (scope Scope) get(id _TypeID) (
	ret reflect.Value,
	ok bool,
) {

	// special types
	switch id {
	case injectStructTypeID:
		// Convert to the named InjectStruct type so that type assertions and
		// generic Get[InjectStruct] succeed. reflect.ValueOf of the method value
		// yields the unnamed func(any) type, which is not identical to InjectStruct.
		return reflect.ValueOf(scope.InjectStruct).Convert(reflect.TypeFor[InjectStruct]()), true
	case forkTypeID:
		// Convert to the named Fork type so that type assertions and
		// generic Get[Fork] succeed. The method value yields an unnamed
		// func(...any) type, which is not identical to Fork.
		return reflect.ValueOf(scope.Fork).Convert(reflect.TypeFor[Fork]()), true
	case resetTypeID:
		// Convert to the named Reset type so that type assertions and
		// generic Get[Reset] succeed. The method value yields an unnamed
		// func() Scope type, which is not identical to Reset.
		return reflect.ValueOf(scope.Reset).Convert(reflect.TypeFor[Reset]()), true
	}

	value, ok := scope.values.Load(id)
	if !ok {
		return ret, false
	}

	return value.initializer.get(scope, value.typeInfo.Position), true
}

// TheoryOfScopeInvocation documents the semantics of scope.Call and the
// validation applied to call targets.
const TheoryOfScopeInvocation = `
dscope invocation theory:
- scope.Call executes a function whose parameters are resolved from the scope
  as dependencies; the parameter list doubles as the dependency declaration,
  so a consumer declares exactly what it needs and nothing more, and the
  return values are delivered through CallResult.
- Exported call entry points must reject malformed call targets with dscope
  errors instead of leaking raw reflect panics: nil, invalid, or non-function
  call targets are bad arguments because the container cannot establish
  dependency semantics for them.
- Once a target is verified as callable, dependency resolution and provider
  execution follow the normal scope semantics.
`

func validateCallableValue(fnValue reflect.Value) reflect.Type {
	if !fnValue.IsValid() {
		panic(errors.Join(
			fmt.Errorf("nil function provided"),
			ErrBadArgument,
		))
	}

	fnType := fnValue.Type()
	if fnType.Kind() != reflect.Func {
		panic(errors.Join(
			fmt.Errorf("%v is not a function", fnType),
			ErrBadArgument,
		))
	}

	if fnValue.IsNil() {
		panic(errors.Join(
			fmt.Errorf("%v nil function provided", fnType),
			ErrBadArgument,
		))
	}

	return fnType
}

// Get is a type-safe generic method to retrieve a single value of type T.
// It panics if the type is not found or if an error occurs during resolution.
func (scope Scope) Get[T any]() T {
	value, ok := scope.TryGet[T]()
	if !ok {
		throwErrDependencyNotFound(reflect.TypeFor[T]())
	}
	return value
}

// TryGet is a type-safe generic method to retrieve a single value of type T.
// Unlike Get, it does not panic when the type is missing from the scope:
// it returns (zero, false) instead. Panics raised by provider evaluation
// still propagate.
func (scope Scope) TryGet[T any]() (o T, ok bool) {
	typ := reflect.TypeFor[T]()
	value, found := scope.get(getTypeID(typ))
	if !found {
		return o, false
	}
	if typ.Kind() == reflect.Interface && value.IsNil() {
		return o, true
	}
	return value.Interface().(T), true
}

// GetType retrieves the value of the given type from the scope and returns
// it as a reflect.Value. It panics with a structured dependency-not-found
// error if the type is not defined in the scope, mirroring the generic
// Get[T].
func (scope Scope) GetType(typ reflect.Type) reflect.Value {
	value, ok := scope.TryGetType(typ)
	if !ok {
		throwErrDependencyNotFound(typ)
	}
	return value
}

// TryGetType retrieves the value of the given type from the scope as a
// reflect.Value. Unlike GetType, it does not panic when the type is missing
// from the scope: it returns the zero reflect.Value and false instead.
// Panics raised by provider evaluation still propagate.
func (scope Scope) TryGetType(typ reflect.Type) (reflect.Value, bool) {
	if typ == nil {
		// Reject nil types up front: getTypeID(nil) would otherwise register a
		// bogus nil -> id mapping in the global type tables.
		panic(errors.Join(
			fmt.Errorf("nil reflect.Type provided"),
			ErrBadArgument,
		))
	}
	value, found := scope.get(getTypeID(typ))
	if !found {
		return reflect.Value{}, false
	}
	return value, true
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

// CallValue executes the given function value after resolving its arguments from the scope.
// It returns a CallResult containing the function's return values.
// It panics with ErrBadArgument if fnValue is invalid, nil, or not a function.
func (scope Scope) CallValue(fnValue reflect.Value) (res CallResult) {
	fnType := validateCallableValue(fnValue)
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
