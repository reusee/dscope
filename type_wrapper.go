package dscope

import (
	"reflect"
	"sync"
)

type typeWrapper interface {
	unwrapType() []reflect.Type
}

var typeWrapperType = reflect.TypeFor[typeWrapper]()

var wrappedTypes sync.Map

func unwrapType(t reflect.Type) []reflect.Type {
	if v, ok := wrappedTypes.Load(t); ok {
		return v.([]reflect.Type)
	}
	wrapped := reflect.New(t).Elem().Interface().(typeWrapper).unwrapType()
	wrappedTypes.Store(t, wrapped)
	return wrapped
}
