package dscope

import (
	"hash/maphash"
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

// _Hash is used for scope signatures and cache keys. A scope's identity is its
// sorted set of definition type IDs; a collision would bind a scope to another
// scope's forker and silently hand out wrong values, so the digest stays 128
// bits wide, keyed with two independent random seeds.
type _Hash [2]uint64

// hashSeeds keys the two halves of every _Hash. The keys come from the runtime's
// random hash seed and never leave the process: signatures and fork keys are
// process-local.
var hashSeeds = [2]uint64{
	maphash.Bytes(maphash.MakeSeed(), []byte("dscope hash key 0")),
	maphash.Bytes(maphash.MakeSeed(), []byte("dscope hash key 1")),
}

// hashMul0 and hashMul1 are odd multipliers that spread the bits of a folded
// word over the whole 64 bits of a hash half.
const (
	hashMul0 = 0x9e3779b97f4a7c15
	hashMul1 = 0xbf58476d1ce4e5b9
)

// hashAbsorb folds one 64-bit word into a hash state. Each half of the state
// mixes the word with a multiplier of its own, so a collision must satisfy both
// halves at once.
func hashAbsorb(h0, h1, word uint64) (uint64, uint64) {
	return (h0 ^ word) * hashMul0, (h1 ^ word) * hashMul1
}

// hashFinal spreads the bits of one half of a hash state, so that every bit of
// the input reaches every bit of the digest.
func hashFinal(h uint64) uint64 {
	h ^= h >> 33
	h *= 0xff51afd7ed558ccd
	h ^= h >> 33
	h *= 0xc4ceb9fe1a85ec53
	h ^= h >> 33
	return h
}

// hashDigest spreads a hash state into the final digest.
func hashDigest(h0, h1 uint64) (ret _Hash) {
	ret[0] = hashFinal(h0)
	ret[1] = hashFinal(h1)
	return
}

// hashTypeIDs hashes a seed followed by the given type IDs, each absorbed as one
// 64-bit word. A scope's identity is its sorted set of definition type IDs, so
// every signature and cache key must use this encoding. The IDs stream into the
// state one at a time, so the digest needs no intermediate buffer.
func hashTypeIDs(seed []byte, ids []_TypeID) (ret _Hash) {
	h0, h1 := hashSeeds[0], hashSeeds[1]
	for _, b := range seed {
		h0, h1 = hashAbsorb(h0, h1, uint64(b))
	}
	for _, id := range ids {
		h0, h1 = hashAbsorb(h0, h1, uint64(id))
	}
	return hashDigest(h0, h1)
}

// forkKey derives the forker cache key of a Fork call: the base scope's
// signature followed by the type IDs of the new definitions, in the encoding
// hashTypeIDs uses. It streams the signature and each ID into the state, so the
// key needs no intermediate buffer and a cache lookup of a small Fork call
// allocates nothing.
func forkKey(signature _Hash, defs []any) (ret _Hash) {
	h0, h1 := hashSeeds[0], hashSeeds[1]
	for _, word := range signature {
		h0, h1 = hashAbsorb(h0, h1, word)
	}
	for _, def := range defs {
		h0, h1 = hashAbsorb(h0, h1, uint64(getTypeID(reflect.TypeOf(def))))
	}
	return hashDigest(h0, h1)
}

// TheoryOfScopeCore documents the fundamental model of dscope: an immutable,
// type-keyed container of lazily evaluated definitions.
const TheoryOfScopeCore = `
dscope core theory:
- A Scope is an immutable, type-keyed container. Every provided value is
  identified by its declared Go type — concrete or interface — and a type has
  at most one effective definition per scope.
- Resolution is by exact declared type: a provider returning an interface
  satisfies requests for that interface, not requests for the concrete value
  it holds; no implicit conversions are performed.
- A computation is a provider: fork the computing function, then get its
  result type. The parameters are the dependencies and the results are the
  computed values, so one declaration carries both, and no separate
  invocation entry point is needed.
- Three built-in dependencies — InjectStruct, Fork, Reset — are always
  available, bound to the current scope, and cannot be overridden: they are
  the escape hatches through which providers interact with the scope
  dynamically.
- Every public operation — Get, TryGet, Assign, InjectStruct, AllTypes —
  reflects the effective definitions of the scope it is invoked on.
`

const TheoryOfScopeDefinitions = `
dscope definition theory:
- A definition is a provider function (its parameters are dependencies resolved
  from the scope, its results are the provided values) or a pointer to a value
  (the pointed-at value is copied into the scope at construction time).
- A provider may return multiple values; each result type becomes a provided
  type of the scope, and a single evaluation feeds all of them.
- Public scope construction validates every definition before deriving type
  identity: nil definitions, nil function or pointer definitions, functions
  that return nothing, variadic functions, and non-function non-pointer
  values are rejected.
- Two definitions in the same Fork call must not produce the same type; a
  duplicate is rejected with an error naming both conflicting definitions.
  Redefining an inherited type is the override mechanism of a later Fork layer.
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

func (scope Scope) Fork(
	defs ...any,
) Scope {

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

	// Validate every (possibly module-expanded) definition before reflection
	// and cache-key generation. The forker cache is keyed by definition types
	// only, so on a cache hit newForker never runs; validation must therefore
	// happen here on every call, or malformed definitions would reach value
	// construction and leak reflection panics at first access.
	for _, def := range defs {
		validateDefinition(def)
	}

	// sorting defs may reduce memory consumption if there're calls with same defs but different order
	// but sorting will increase heap allocations, causing performance drop

	// Calculate cache key for this Fork operation.
	// Key is based on the base scope signature and the types of new definitions.
	// Hashing types is sufficient as only one definition instance per type is effectively used.
	key := forkKey(scope.signature, defs)

	// Check cache
	if v, ok := forkers.Load(key); ok {
		return v.(*_Forker).Fork(scope, defs)
	}

	// Cache miss, create and cache forker
	forker := newForker(scope, defs)
	v, _ := forkers.LoadOrStore(key, forker)

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
- Typical use: keep the definitions but drop every cached result, either to
  observe fresh provider evaluation in tests, or to re-run the graph after
  external state (files, clocks, globals) has changed.
`

// Reset returns a new Scope in which every value will be recomputed the next
// time it is requested. The original scope is unaffected.
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
		signature: scope.signature,
	}
}

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
- Assign[T] accepts a single typed pointer; a nil pointer is a bad argument
  and must produce a structured dscope error instead of leaking a runtime or
  reflection panic.
`

// Assign retrieves the value of type T from the scope and writes it to the
// provided pointer. It panics if ptr is nil or if the type is not found.
// It's safe to call Assign concurrently.
func (scope Scope) Assign[T any](ptr *T) {
	if ptr == nil {
		panic(errWith(ErrBadArgument, "cannot assign to a nil pointer target of type %T", ptr))
	}
	*ptr = scope.Get[T]()
}

func (scope Scope) get(id _TypeID) (
	ret reflect.Value,
	ok bool,
) {

	// Built-in dependencies are bound method values. Convert each to its named
	// type so that type assertions and generic Get[T] succeed: a method value
	// has an unnamed function type, which is not identical to the named type.
	switch id {
	case injectStructTypeID:
		return reflect.ValueOf(scope.InjectStruct).Convert(reflect.TypeFor[InjectStruct]()), true
	case forkTypeID:
		return reflect.ValueOf(scope.Fork).Convert(reflect.TypeFor[Fork]()), true
	case resetTypeID:
		return reflect.ValueOf(scope.Reset).Convert(reflect.TypeFor[Reset]()), true
	}

	value, ok := scope.values.Load(id)
	if !ok {
		return ret, false
	}

	return value.initializer.get(scope, value.typeInfo.Position), true
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
	// A nil interface value carries no concrete type to assert, so it comes back
	// as the zero T. The kind of a stored value is the kind of the type it was
	// defined with, so reading it here keeps the check off the common path: only
	// a value of interface kind reaches IsNil.
	if value.Kind() == reflect.Interface && value.IsNil() {
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
		panic(errWith(ErrBadArgument, "nil reflect.Type provided"))
	}
	value, found := scope.get(getTypeID(typ))
	if !found {
		return reflect.Value{}, false
	}
	return value, true
}

// Cache for the argument-fetching logic for a given function type.
// reflect.Type -> func(Scope, []reflect.Value) int
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

const reflectValuesPoolMaxLen = 64

var reflectValuesPool = sync.Pool{
	New: func() any {
		return new([reflectValuesPoolMaxLen]reflect.Value)
	},
}

// call resolves the parameters of fnValue from the scope, invokes fnValue, and
// returns its results. fnValue is a definition that already passed
// validateDefinition — a valid, non-nil function returning at least one value
// — so the invocation itself cannot fail on malformed input.
func (scope Scope) call(fnValue reflect.Value) []reflect.Value {
	fnType := fnValue.Type()
	nArgs := fnType.NumIn()
	if nArgs == 0 {
		// A provider without parameters takes the direct path: reflect accepts
		// a nil argument slice, so no pooled buffer is involved.
		return fnValue.Call(nil)
	}
	// Use pool for small number of arguments
	if nArgs <= reflectValuesPoolMaxLen {
		ptr := reflectValuesPool.Get().(*[reflectValuesPoolMaxLen]reflect.Value)
		args := ptr[:nArgs]
		n := scope.getArgs(fnType, args)
		rets := fnValue.Call(args[:n])
		// Release the resolved arguments before the buffer returns to the pool.
		clear(args)
		reflectValuesPool.Put(ptr)
		return rets
	}
	args := make([]reflect.Value, nArgs)
	n := scope.getArgs(fnType, args)
	return fnValue.Call(args[:n])
}
