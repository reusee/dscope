package dscope

import (
	"reflect"
	"sync/atomic"
)

type MutableScope struct {
	scope atomic.Value
}

type MutateCall func(fn any) Scope

type Mutate func(decls ...any) Scope

type GetScope func() Scope

func NewMutable(
	decls ...any,
) *MutableScope {

	m := new(MutableScope)

	get := GetScope(func() Scope {
		return *m.scope.Load().(*Scope)
	})

	mutateCall := MutateCall(func(fn any) Scope {
		numRedo := 0
	mutate:
		if numRedo > 1024 {
			panic("dependency loop or too much contention")
		}
		from := m.scope.Load().(*Scope)
		cur := *from
		res := cur.CallValue(reflect.ValueOf(fn))
		var decls []any
		for _, v := range res.Values {
			decls = append(decls, v.Interface())
		}
		mutated := cur.Fork(decls...)
		mutatedPtr := &mutated
		if !m.scope.CompareAndSwap(from, mutatedPtr) {
			numRedo++
			goto mutate
		}
		return mutated
	})

	mutate := Mutate(func(decls ...any) Scope {
		numRedo := 0
	mutate:
		if numRedo > 1024 { // NOCOVER
			panic("dependency loop or too much contention")
		}
		from := m.scope.Load().(*Scope)
		cur := *from
		mutated := cur.Fork(decls...)
		mutatedPtr := &mutated
		if !m.scope.CompareAndSwap(from, mutatedPtr) {
			numRedo++
			goto mutate
		}
		return mutated
	})

	decls = append(decls, &get, &mutateCall, &mutate)
	s := New(decls...)
	m.scope.Store(&s)

	return m
}

func (m *MutableScope) GetScope() Scope {
	return *m.scope.Load().(*Scope)
}

func (m *MutableScope) Fork(
	initializers ...any,
) Scope {
	return m.GetScope().Fork(initializers...)
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

func (m *MutableScope) FillStruct(ptr any) {
	m.GetScope().FillStruct(ptr)
}
