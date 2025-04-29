package dscope

import (
	"reflect"

	"github.com/reusee/e5"
)

type CallResult struct {
	positionsByType map[reflect.Type][]int
	Values          []reflect.Value
}

func (c CallResult) Extract(targets ...any) {
	for i, target := range targets {
		if target == nil {
			continue
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Pointer {
			_ = throw(we.With(
				e5.Info("%T is not a pointer", target),
			)(
				ErrBadArgument,
			))
		}
		if !targetValue.Type().Elem().AssignableTo(c.Values[i].Type()) {
			_ = throw(we.With(
				e5.Info("%T is not assignable to %v",
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
	offsets := make(map[reflect.Type]int)
	for _, target := range targets {
		if target == nil {
			continue
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Pointer {
			panic(we.With(
				e5.Info("%v is not a pointer", target),
			)(
				ErrBadArgument,
			))
		}
		typ := targetValue.Type().Elem()
		poses, ok := c.positionsByType[typ]
		if !ok {
			continue
		}
		offset := offsets[typ]
		offsets[typ]++
		targetValue.Elem().Set(c.Values[poses[offset]])
	}
}
