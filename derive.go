package dscope

import (
	"reflect"
	"sync/atomic"
	"unsafe"
)

type DeriveCall func(fn any) Scope

type Derive func(decls ...any) Scope

type GetLatest func() Scope

func NewDeriving(
	decls ...any,
) Scope {
	scope := New(decls...)

	var ptr unsafe.Pointer
	ptr = unsafe.Pointer(&scope)

	getLatest := GetLatest(func() Scope {
		return *(*Scope)(atomic.LoadPointer(&ptr))
	})

	deriveCall := DeriveCall(func(fn any) Scope {
		numRedo := 0
	derive:
		if numRedo > 1024 {
			panic("derive loop or too much contention")
		}
		from := atomic.LoadPointer(&ptr)
		cur := *(*Scope)(from)
		res := cur.CallValue(reflect.ValueOf(fn))
		var decls []any
		for _, v := range res.Values {
			decls = append(decls, v.Interface())
		}
		derived := cur.Sub(decls...)
		derivedPtr := unsafe.Pointer(&derived)
		if !atomic.CompareAndSwapPointer(&ptr, from, derivedPtr) {
			numRedo++
			goto derive
		}
		return derived
	})

	derive := Derive(func(decls ...any) Scope {
		numRedo := 0
	derive:
		if numRedo > 1024 {
			panic("derive loop or too much contention")
		}
		from := atomic.LoadPointer(&ptr)
		cur := *(*Scope)(from)
		derived := cur.Sub(decls...)
		derivedPtr := unsafe.Pointer(&derived)
		if !atomic.CompareAndSwapPointer(&ptr, from, derivedPtr) {
			numRedo++
			goto derive
		}
		return derived
	})

	scope = scope.Sub(
		&getLatest,
		&deriveCall,
		&derive,
	)

	return scope
}
