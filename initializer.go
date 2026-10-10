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
	Def          any
	DefIsPointer bool
	Values       []reflect.Value
	_values      [1]reflect.Value
	done         atomic.Bool
	mu           sync.Mutex
}

func newInitializer(def any, isPointer bool) *_Initializer {
	ret := &_Initializer{
		Def:          def,
		DefIsPointer: isPointer,
	}
	if isPointer {
		definitionValue := reflect.ValueOf(def).Elem()
		copiedValue := reflect.New(definitionValue.Type()).Elem()
		copiedValue.Set(definitionValue)
		ret._values[0] = copiedValue
		ret.Values = ret._values[:1]
	}
	return ret
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

func (i *_Initializer) get(scope Scope, position int) (ret reflect.Value) {
	if !i.DefIsPointer && !i.done.Load() {
		i.mu.Lock()
		defer i.mu.Unlock()
		if !i.done.Load() {
			i.Values = scope.call(reflect.ValueOf(i.Def))
			i.done.Store(true)
		}
	}
	return i.Values[position]
}
