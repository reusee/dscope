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
	// state publishes the result, tracks evaluation locking/contention, and
	// records whether result holds multiple outputs.
	state        atomic.Uint32
	DefIsPointer bool
}

const (
	initializerStateDone uint32 = 1 << iota
	initializerStateMulti
	initializerStateLocked
	initializerStateWaiters
)

var (
	initWaitMu  sync.Mutex
	initWaiters = make(map[*_Initializer]chan struct{})
)

type _MultiResult struct {
	vals   []reflect.Value
	ifaces []any
}

func initInitializer(i *_Initializer, def any, isPointer bool) {
	i.Def = def
	i.DefIsPointer = isPointer
	if !isPointer {
		return
	}
	definitionValue := reflect.ValueOf(def).Elem()
	if definitionValue.Kind() == reflect.Interface {
		var copiedValue reflect.Value
		var iface any
		if definitionValue.IsNil() {
			copiedValue = reflect.Zero(definitionValue.Type())
		} else {
			copiedValue = definitionValue.Elem().Convert(definitionValue.Type())
			iface = copiedValue.Interface()
		}
		i.result = &_MultiResult{
			vals:   []reflect.Value{copiedValue},
			ifaces: []any{iface},
		}
		i.state.Store(initializerStateDone | initializerStateMulti)
		return
	}
	i.result = definitionValue.Interface()
	i.state.Store(initializerStateDone)
}

// reset makes the initializer re-evaluate Values.
func (s *_Initializer) reset() *_Initializer {
	if s.DefIsPointer {
		return s
	}
	return &_Initializer{
		Def:          s.Def,
		DefIsPointer: s.DefIsPointer,
	}
}

func (i *_Initializer) releaseWaiters() {
	initWaitMu.Lock()
	ch := initWaiters[i]
	delete(initWaiters, i)
	initWaitMu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func (i *_Initializer) waitEvaluation() {
	initWaitMu.Lock()
	s := i.state.Load()
	if s&initializerStateDone != 0 || s == 0 {
		initWaitMu.Unlock()
		return
	}
	ch, existed := initWaiters[i]
	if !existed {
		ch = make(chan struct{})
		initWaiters[i] = ch
	}
	if s&initializerStateWaiters == 0 {
		if !i.state.CompareAndSwap(s, s|initializerStateWaiters) {
			if !existed {
				delete(initWaiters, i)
			}
			initWaitMu.Unlock()
			return
		}
	}
	initWaitMu.Unlock()
	<-ch
}

func (i *_Initializer) runProvider(scope Scope) {
	completed := false
	defer func() {
		if !completed {
			if prev := i.state.Swap(0); prev&initializerStateWaiters != 0 {
				i.releaseWaiters()
			}
		}
	}()

	vals := scope.call(reflect.ValueOf(i.Def))
	finalState := initializerStateDone
	if len(vals) == 1 && vals[0].Kind() != reflect.Interface {
		i.result = vals[0].Interface()
	} else {
		ifaces := make([]any, len(vals))
		for j, vj := range vals {
			if vj.Kind() != reflect.Interface || !vj.IsNil() {
				ifaces[j] = vj.Interface()
			}
		}
		finalState |= initializerStateMulti
		i.result = &_MultiResult{
			vals:   vals,
			ifaces: ifaces,
		}
	}
	completed = true
	if prev := i.state.Swap(finalState); prev&initializerStateWaiters != 0 {
		i.releaseWaiters()
	}
}

func (i *_Initializer) evaluate(scope Scope) {
	for {
		s := i.state.Load()
		if s&initializerStateDone != 0 {
			return
		}
		if s == 0 && i.state.CompareAndSwap(0, initializerStateLocked) {
			i.runProvider(scope)
			return
		}
		i.waitEvaluation()
	}
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
