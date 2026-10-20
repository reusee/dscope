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
`

type _Initializer struct {
	Def    any
	result any
	mu     sync.Mutex
	// state publishes the result and records whether it contains multiple
	// outputs. A release store publishes result before a reader observes the
	// completed state.
	state        atomic.Uint32
	DefIsPointer bool
}

const (
	initializerStateDone uint32 = 1 << iota
	initializerStateMulti
)

type _MultiResult struct {
	vals   []reflect.Value
	ifaces []any
}

func initInitializer(i *_Initializer, def any, isPointer bool) {
	i.Def = def
	i.DefIsPointer = isPointer
	if isPointer {
		definitionValue := reflect.ValueOf(def).Elem()
		if definitionValue.Kind() == reflect.Interface {
			copiedValue := reflect.New(definitionValue.Type()).Elem()
			copiedValue.Set(definitionValue)
			var iface any
			if !copiedValue.IsNil() {
				iface = copiedValue.Interface()
			}
			i.result = &_MultiResult{
				vals:   []reflect.Value{copiedValue},
				ifaces: []any{iface},
			}
			i.state.Store(initializerStateDone | initializerStateMulti)
		} else {
			i.result = definitionValue.Interface()
			i.state.Store(initializerStateDone)
		}
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

func (i *_Initializer) evaluate(scope Scope) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state.Load()&initializerStateDone != 0 {
		return
	}
	vals := scope.call(reflect.ValueOf(i.Def))
	state := initializerStateDone
	if len(vals) == 1 && vals[0].Kind() != reflect.Interface {
		i.result = vals[0].Interface()
	} else {
		ifaces := make([]any, len(vals))
		for j, vj := range vals {
			if vj.Kind() != reflect.Interface || !vj.IsNil() {
				ifaces[j] = vj.Interface()
			}
		}
		state |= initializerStateMulti
		i.result = &_MultiResult{
			vals:   vals,
			ifaces: ifaces,
		}
	}
	i.state.Store(state)
}

func (i *_Initializer) get(scope Scope, v _Value) reflect.Value {
	state := i.state.Load()
	if state&initializerStateDone == 0 {
		i.evaluate(scope)
		state = i.state.Load()
	}
	if state&initializerStateMulti == 0 {
		return reflect.ValueOf(i.result)
	}
	return i.result.(*_MultiResult).vals[v.typeInfo.Position]
}

func (i *_Initializer) getIface(scope Scope, v _Value) any {
	state := i.state.Load()
	if state&initializerStateDone == 0 {
		i.evaluate(scope)
		state = i.state.Load()
	}
	if state&initializerStateMulti == 0 {
		return i.result
	}
	return i.result.(*_MultiResult).ifaces[v.typeInfo.Position]
}
