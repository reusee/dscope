package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"
)

// TheoryOfLazyInitialization documents the design rationale for dscope's
// lazy initialization mechanism. Provider functions are evaluated on first
// access; results are cached and shared across all consumers within the same
// scope. A panicking provider must NOT permanently cache the failure — subsequent
// accesses must re-invoke the provider to reproduce the original error, ensuring
// that transient provider failures are always diagnosable and never leave the
// system in an unrecoverable or misleading state.
const TheoryOfLazyInitialization = `
dscope lazy initialization theory:
- Providers evaluate at most once per initializer instance; results are cached.
- A provider panic must NOT be cached as a permanent failure state.
  Subsequent accesses re-invoke the provider to reproduce the original error.
- Reset initializers (created on Fork when dependencies change) inherit
  this contract: a fresh initializer always re-evaluates on first access.
`

type _Initializer struct {
	Def          any
	DefIsPointer bool
	Values       []reflect.Value
	_values      [1]reflect.Value
	ID           int64
	done         atomic.Bool
	mu           sync.Mutex
}

func newInitializer(def any, isPointer bool) *_Initializer {
	ret := &_Initializer{
		ID:           atomic.AddInt64(&nextInitializerID, 1),
		Def:          def,
		DefIsPointer: isPointer,
	}
	if isPointer {
		ret._values[0] = reflect.ValueOf(
			// make a copy
			reflect.ValueOf(def).Elem().Interface(),
		)
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
		ID:           s.ID,
		Def:          s.Def,
		DefIsPointer: s.DefIsPointer,
	}
}

var nextInitializerID int64 = 42

func (i *_Initializer) get(scope Scope, position int) (ret reflect.Value) {
	if !i.DefIsPointer && !i.done.Load() {
		i.mu.Lock()
		defer i.mu.Unlock()
		if !i.done.Load() {
			i.Values = scope.CallValue(reflect.ValueOf(i.Def)).Values
			i.done.Store(true)
		}
	}
	return i.Values[position]
}