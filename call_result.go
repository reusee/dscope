package dscope

import (
	"fmt"
	"reflect"

	"github.com/reusee/e4"
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
			panic(we(
				e4.With(ArgInfo{
					Value: target,
				}),
				e4.With(Reason("must be a pointer")),
			)(
				ErrBadArgument,
			))
		}
		if targetValue.Type().Elem() != c.Values[i].Type() {
			panic(we(
				e4.With(ArgInfo{
					Value: target,
				}),
				e4.With(Reason(
					fmt.Sprintf("must be pointer to %v", targetValue.Type().Elem().String()),
				)),
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
			panic(we(
				e4.With(ArgInfo{
					Value: target,
				}),
				e4.With(Reason("must be a pointer")),
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
