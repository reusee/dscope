package dscope

import "reflect"

type Inject[T any] func() T

type injectMark struct{}

func (Inject[T]) isInject(injectMark) {}

var isInjectType = reflect.TypeFor[interface {
	isInject(injectMark)
}]()
