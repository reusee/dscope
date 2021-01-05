package dscope

import (
	"fmt"
	"reflect"

	"github.com/reusee/e4"
)

type CallResult struct {
	Scope  Scope
	Values []reflect.Value
}

func (c CallResult) Sub() Scope {
	var decls []any
	for _, value := range c.Values {
		v := reflect.New(value.Type())
		v.Elem().Set(value)
		decls = append(decls, v.Interface())
	}
	return c.Scope.Sub(decls...)
}

func (c CallResult) Assign(targets ...any) {
	for i, target := range targets {
		if target == nil {
			continue
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Ptr {
			panic(we(
				ErrBadArgument,
				e4.With(ArgInfo{
					Value: target,
				}),
				e4.With(Reason("must be a pointer")),
			))
		}
		if targetValue.Type().Elem() != c.Values[i].Type() {
			panic(we(
				ErrBadArgument,
				e4.With(ArgInfo{
					Value: target,
				}),
				e4.With(Reason(
					fmt.Sprintf("must be %v", targetValue.Type().Elem().String()),
				)),
			))
		}
		targetValue.Elem().Set(c.Values[i])
	}
}
