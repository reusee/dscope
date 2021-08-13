package dscope

import (
	"reflect"
	"sync/atomic"
)

type MutateCall func(fn any) Scope

type Mutate func(decls ...any) Scope

type GetScope func() Scope

func NewMutable(
	decls ...any,
) Scope {

	var scope atomic.Value

	get := GetScope(func() Scope {
		return *scope.Load().(*Scope)
	})

	mutateCall := MutateCall(func(fn any) Scope {
		numRedo := 0
	mutate:
		if numRedo > 1024 {
			panic("dependency loop or too much contention")
		}
		from := scope.Load().(*Scope)
		cur := *from
		res := cur.CallValue(reflect.ValueOf(fn))
		var decls []any
		for _, v := range res.Values {
			decls = append(decls, v.Interface())
		}
		mutated := cur.Sub(decls...)
		mutatedPtr := &mutated
		if !scope.CompareAndSwap(from, mutatedPtr) {
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
		from := scope.Load().(*Scope)
		cur := *from
		mutated := cur.Sub(decls...)
		mutatedPtr := &mutated
		if !scope.CompareAndSwap(from, mutatedPtr) {
			numRedo++
			goto mutate
		}
		return mutated
	})

	decls = append(decls, &get, &mutateCall, &mutate)
	s := New(decls...)
	scope.Store(&s)

	return s
}
