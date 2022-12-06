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

type WithFork3[T any, T2 any, T3 any] func(defs ...any) (T, T2, T3)

func (w WithFork3[T, T2, T3]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork3[T, T2, T3](func(defs ...any) (t T, t2 T2, t3 T3) {
		scope.Fork(defs...).Assign(&t, &t2, &t3)
		return
	}))
}

var _ typeWrapper = WithFork3[int, int, int](nil)

func (w WithFork3[T, T2, T3]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	var t3 T3
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
		reflect.TypeOf(t3),
	}
}

type WithFork4[T any, T2 any, T3 any, T4 any] func(defs ...any) (T, T2, T3, T4)

func (w WithFork4[T, T2, T3, T4]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork4[T, T2, T3, T4](func(defs ...any) (t T, t2 T2, t3 T3, t4 T4) {
		scope.Fork(defs...).Assign(&t, &t2, &t3, &t4)
		return
	}))
}

var _ typeWrapper = WithFork4[int, int, int, int](nil)

func (w WithFork4[T, T2, T3, T4]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	var t3 T3
	var t4 T4
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
		reflect.TypeOf(t3),
		reflect.TypeOf(t4),
	}
}

type WithFork5[T any, T2 any, T3 any, T4 any, T5 any] func(defs ...any) (T, T2, T3, T4, T5)

func (w WithFork5[T, T2, T3, T4, T5]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork5[T, T2, T3, T4, T5](func(defs ...any) (t T, t2 T2, t3 T3, t4 T4, t5 T5) {
		scope.Fork(defs...).Assign(&t, &t2, &t3, &t4, &t5)
		return
	}))
}

var _ typeWrapper = WithFork5[int, int, int, int, int](nil)

func (w WithFork5[T, T2, T3, T4, T5]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	var t3 T3
	var t4 T4
	var t5 T5
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
		reflect.TypeOf(t3),
		reflect.TypeOf(t4),
		reflect.TypeOf(t5),
	}
}

type WithFork6[T any, T2 any, T3 any, T4 any, T5 any, T6 any] func(defs ...any) (T, T2, T3, T4, T5, T6)

func (w WithFork6[T, T2, T3, T4, T5, T6]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork6[T, T2, T3, T4, T5, T6](func(defs ...any) (t T, t2 T2, t3 T3, t4 T4, t5 T5, t6 T6) {
		scope.Fork(defs...).Assign(&t, &t2, &t3, &t4, &t5, &t6)
		return
	}))
}

var _ typeWrapper = WithFork6[int, int, int, int, int, int](nil)

func (w WithFork6[T, T2, T3, T4, T5, T6]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	var t3 T3
	var t4 T4
	var t5 T5
	var t6 T6
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
		reflect.TypeOf(t3),
		reflect.TypeOf(t4),
		reflect.TypeOf(t5),
		reflect.TypeOf(t6),
	}
}

type WithFork7[T any, T2 any, T3 any, T4 any, T5 any, T6 any, T7 any] func(defs ...any) (T, T2, T3, T4, T5, T6, T7)

func (w WithFork7[T, T2, T3, T4, T5, T6, T7]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork7[T, T2, T3, T4, T5, T6, T7](func(defs ...any) (t T, t2 T2, t3 T3, t4 T4, t5 T5, t6 T6, t7 T7) {
		scope.Fork(defs...).Assign(&t, &t2, &t3, &t4, &t5, &t6, &t7)
		return
	}))
}

var _ typeWrapper = WithFork7[int, int, int, int, int, int, int](nil)

func (w WithFork7[T, T2, T3, T4, T5, T6, T7]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	var t3 T3
	var t4 T4
	var t5 T5
	var t6 T6
	var t7 T7
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
		reflect.TypeOf(t3),
		reflect.TypeOf(t4),
		reflect.TypeOf(t5),
		reflect.TypeOf(t6),
		reflect.TypeOf(t7),
	}
}

type WithFork8[T any, T2 any, T3 any, T4 any, T5 any, T6 any, T7 any, T8 any] func(defs ...any) (T, T2, T3, T4, T5, T6, T7, T8)

func (w WithFork8[T, T2, T3, T4, T5, T6, T7, T8]) newWithForkValue(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork8[T, T2, T3, T4, T5, T6, T7, T8](func(defs ...any) (t T, t2 T2, t3 T3, t4 T4, t5 T5, t6 T6, t7 T7, t8 T8) {
		scope.Fork(defs...).Assign(&t, &t2, &t3, &t4, &t5, &t6, &t7, &t8)
		return
	}))
}

var _ typeWrapper = WithFork8[int, int, int, int, int, int, int, int](nil)

func (w WithFork8[T, T2, T3, T4, T5, T6, T7, T8]) unwrapType() []reflect.Type {
	var t T
	var t2 T2
	var t3 T3
	var t4 T4
	var t5 T5
	var t6 T6
	var t7 T7
	var t8 T8
	return []reflect.Type{
		reflect.TypeOf(t),
		reflect.TypeOf(t2),
		reflect.TypeOf(t3),
		reflect.TypeOf(t4),
		reflect.TypeOf(t5),
		reflect.TypeOf(t6),
		reflect.TypeOf(t7),
		reflect.TypeOf(t8),
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
