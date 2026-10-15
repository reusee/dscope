package dscope

import "reflect"

type Inject[T any] func() T

type injectMark struct{}

func (Inject[T]) isInject(injectMark) {}

var isInjectType = reflect.TypeFor[interface {
	isInject(injectMark)
}]()

func (Inject[T]) makeInject(scope Scope) reflect.Value {
	return reflect.ValueOf(Inject[T](func() T {
		return scope.Get[T]()
	}))
}

type isInjectMaker interface {
	isInject(injectMark)
	makeInject(scope Scope) reflect.Value
}
