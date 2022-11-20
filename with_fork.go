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

type withFork interface {
	newWithForkValue(scope Scope) reflect.Value
}

var withForkType = reflect.TypeOf((*withFork)(nil)).Elem()

var withForkForkers sync.Map

func getWithForkForker(t reflect.Type) func(Scope) reflect.Value {
	if v, ok := withForkForkers.Load(t); ok {
		return v.(func(Scope) reflect.Value)
	}
	forker := reflect.New(t).Elem().Interface().(withFork).newWithForkValue
	withForkForkers.Store(t, forker)
	return forker
}
