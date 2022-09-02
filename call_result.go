package dscope

import (
	"reflect"

	"github.com/reusee/e5"
)

type CallResult struct {
	positionsByType map[reflect.Type]int
	Values          []reflect.Value
}

func (c CallResult) Extract(targets ...any) {
	for i, target := range targets {
		if target == nil {
			continue
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Ptr {
			panic(we.With(
				e5.Info("%T is not a pointer", target),
			)(
				ErrBadArgument,
			))
		}
		if targetValue.Type().Elem() != c.Values[i].Type() {
			panic(we.With(
				e5.Info("%T is not a pointer to %v",
					target,
					targetValue.Type().Elem()),
			)(
				ErrBadArgument,
			))
		}
		targetValue.Elem().Set(c.Values[i])
	}
}

func (c CallResult) Assign(targets ...any) {
	for _, target := range targets {
		if target == nil {
			continue
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Ptr {
			panic(we.With(
				e5.Info("%v is not a pointer", target),
			)(
				ErrBadArgument,
			))
		}
		pos, ok := c.positionsByType[targetValue.Type().Elem()]
		if !ok {
			continue
		}
		targetValue.Elem().Set(c.Values[pos])
	}
}
