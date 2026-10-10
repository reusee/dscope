package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"
)

const TheoryOfLazyInitialization = `
dscope lazy initialization theory:
- Providers evaluate at most once per initializer instance; results are cached.
- Construction is cheap: Fork validates and analyzes the dependency graph but
  never executes a provider; values come into existence on first access, so
  startup and tests pay only for the values actually touched.
- Scopes are safe for concurrent access: when several goroutines resolve the
  same value simultaneously, the provider still runs only once.
- Pointer definitions are copied while preserving their declared reflected type,
  including zero and nil interface values.
- A provider panic must NOT be cached as a permanent failure state.
  Subsequent accesses re-invoke the provider to reproduce the original error.
- Reset initializers (created on Fork when dependencies change) inherit
  this contract: a fresh initializer always re-evaluates on first access.
`

// _Initializer holds the values of one definition in one scope and evaluates
// them at most once. Its identity is its address: a reset layer keys the fresh
// initializer it hands out by the inherited initializer itself, so two
// initializers of the same definition in different scopes stay apart.
type _Initializer struct {
	Def    any
	Values []reflect.Value
	mu     sync.Mutex
	done   atomic.Bool
	// DefIsPointer marks a pointer definition. Its value is copied when the
	// initializer is built and never re-evaluated.
	DefIsPointer bool
	// _values backs Values for a pointer definition, so copying such a
	// definition needs no separate slice.
	_values [1]reflect.Value
}

func newInitializer(def any, isPointer bool) *_Initializer {
	ret := new(_Initializer)
	initInitializer(ret, def, isPointer)
	return ret
}

// initInitializer fills an initializer from def. The caller owns the storage of
// the initializer: a Fork keeps the initializers of its definitions in the
// allocation of its layer.
func initInitializer(i *_Initializer, def any, isPointer bool) {
	i.Def = def
	i.DefIsPointer = isPointer
	if isPointer {
		definitionValue := reflect.ValueOf(def).Elem()
		copiedValue := reflect.New(definitionValue.Type()).Elem()
		copiedValue.Set(definitionValue)
		i._values[0] = copiedValue
		i.Values = i._values[:1]
	}
}

// reset make the initializer re-evaluate Values
func (s *_Initializer) reset() *_Initializer {
	if s.DefIsPointer {
		// no need to re-evaluate
		return s
	}
	return &_Initializer{
		// these fields recognize the provided type and def to get the values, so not changing
		Def:          s.Def,
		DefIsPointer: s.DefIsPointer,
	}
}

// evaluate runs the definition once and caches its values. A panic of the
// definition leaves done unset, so the next access runs the definition again and
// reproduces the panic.
func (i *_Initializer) evaluate(scope Scope) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.done.Load() {
		return
	}
	i.Values = scope.call(reflect.ValueOf(i.Def))
	i.done.Store(true)
}

// get returns the value of one output and evaluates the definition on first
// access. The check of the cached state stays small, so the compiler inlines the
// function into its callers.
func (i *_Initializer) get(scope Scope, position int) (ret reflect.Value) {
	if !i.DefIsPointer && !i.done.Load() {
		i.evaluate(scope)
	}
	return i.Values[position]
}
