package dscope

import (
	"reflect"
	"sync"
)

type WithFork[T any] func(defs ...any) T

func (w WithFork[T]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork[T](func(defs ...any) (t T) {
		Assign(scope.Fork(defs...), &t)
		return
	}))
}

var _ typeWrapper = WithFork[int](nil)

func (w WithFork[T]) unwrapType() []reflect.Type {
	var t T
	return []reflect.Type{reflect.TypeOf(t)}
}

type WithFork2[T any, T2 any] func(defs ...any) (T, T2)

func (w WithFork2[T, T2]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork2[T, T2](func(defs ...any) (t T, t2 T2) {
		scope.Fork(defs...).Assign(&t, &t2)
		return
	}))
}

var _ typeWrapper = WithFork2[int, int](nil)

func (w WithFork2[T, T2]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
	}
}

type withFork interface {
	newWithForkValue(scope Scope) reflect.Value
}

var withForkType = reflect.TypeOf((*withFork)(nil)).Elem()

var withForkForkers sync.Map

func getWithForkForker(t reflect.Type) func(Scope) reflect.Value {
	if v, ok := withForkForkers.Load(t); ok {
		if v != nil {
			return v.(func(Scope) reflect.Value)
		}
		return nil
	}
	if t.Implements(withForkType) {
		forker := reflect.New(t).Elem().Interface().(withFork).newWithForkValue
		withForkForkers.Store(t, forker)
		return forker
	}
	withForkForkers.Store(t, nil)
	return nil
}
