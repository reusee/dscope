package dscope

import (
	"reflect"
)

type WithFork[T any] func(defs ...any) T

func (w WithFork[T]) isWithFork() {}

func (w WithFork[T]) forker(scope Scope) reflect.Value {
	return reflect.ValueOf(WithFork[T](func(defs ...any) (t T) {
		Assign(scope.Fork(defs...), &t)
		return
	}))
}

type withFork interface {
	isWithFork()
	forker(scope Scope) reflect.Value
}

var isWithForkType = reflect.TypeOf((*withFork)(nil)).Elem()
