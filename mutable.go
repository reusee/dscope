package dscope

import (
	"reflect"
	"sync/atomic"
)

type MutableScope struct {
	scope *atomic.Pointer[Scope]
}

type MutateCall func(fn any) Scope

type Mutate func(defs ...any) Scope

type GetScope func() Scope

func NewMutable(
	defs ...any,
) *MutableScope {

	m := &MutableScope{
		scope: new(atomic.Pointer[Scope]),
	}

	get := GetScope(func() Scope {
		return *m.scope.Load()
	})

	mutateCall := MutateCall(m.MutateCall)

	mutate := Mutate(m.Mutate)

	defs = append(defs, &get, &mutateCall, &mutate)
	s := New(defs...)
	m.scope.Store(&s)

	return m
}

func (m *MutableScope) GetScope() Scope {
	return *m.scope.Load()
}

func (m *MutableScope) Fork(
	defs ...any,
) Scope {
	return m.GetScope().Fork(defs...)
}

func (m *MutableScope) Assign(objs ...any) {
	m.GetScope().Assign(objs...)
}

func (m *MutableScope) Get(t reflect.Type) (reflect.Value, error) {
	return m.GetScope().Get(t)
}

func (m *MutableScope) Call(fn any) CallResult {
	return m.GetScope().Call(fn)
}

func (m *MutableScope) CallValue(fn reflect.Value) CallResult {
	return m.GetScope().CallValue(fn)
}

func (m *MutableScope) Mutate(defs ...any) Scope {
	numRedo := 0
mutate:
	if numRedo > 1024 { // NOCOVER
		panic("dependency loop or too much contention")
	}
	from := m.scope.Load()
	cur := *from
	mutated := cur.Fork(defs...)
	mutatedPtr := &mutated
	if !m.scope.CompareAndSwap(from, mutatedPtr) {
		numRedo++
		goto mutate
	}
	return mutated
}

func (m *MutableScope) MutateCall(fn any) Scope {
	numRedo := 0
mutate:
	if numRedo > 1024 {
		panic("dependency loop or too much contention")
	}
	from := m.scope.Load()
	cur := *from
	res := cur.CallValue(reflect.ValueOf(fn))
	var defs []any
	for _, v := range res.Values {
		def := v.Interface()
		if def == nil {
			continue
		}
		defs = append(defs, def)
	}
	if len(defs) == 0 {
		return *from
	}
	mutated := cur.Fork(defs...)
	mutatedPtr := &mutated
	if !m.scope.CompareAndSwap(from, mutatedPtr) {
		numRedo++
		goto mutate
	}
	return mutated
}
