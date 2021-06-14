package dscope

import (
	"reflect"
	"sync/atomic"
	"unsafe"
)

type MutateCall func(fn any) Scope

type Mutate func(decls ...any) Scope

type GetScope func() Scope

func NewMutable(
	decls ...any,
) Scope {

	var ptr unsafe.Pointer

	get := GetScope(func() Scope {
		return *(*Scope)(atomic.LoadPointer(&ptr))
	})

	mutateCall := MutateCall(func(fn any) Scope {
		numRedo := 0
	mutate:
		if numRedo > 1024 {
			panic("dependency loop or too much contention")
		}
		from := atomic.LoadPointer(&ptr)
		cur := *(*Scope)(from)
		res := cur.CallValue(reflect.ValueOf(fn))
		var decls []any
		for _, v := range res.Values {
			decls = append(decls, v.Interface())
		}
		mutated := cur.Sub(decls...)
		mutatedPtr := unsafe.Pointer(&mutated)
		if !atomic.CompareAndSwapPointer(&ptr, from, mutatedPtr) {
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
		from := atomic.LoadPointer(&ptr)
		cur := *(*Scope)(from)
		mutated := cur.Sub(decls...)
		mutatedPtr := unsafe.Pointer(&mutated)
		if !atomic.CompareAndSwapPointer(&ptr, from, mutatedPtr) {
			numRedo++
			goto mutate
		}
		return mutated
	})

	decls = append(decls, &get, &mutateCall, &mutate)
	scope := New(decls...)
	ptr = unsafe.Pointer(&scope)

	return scope
}
