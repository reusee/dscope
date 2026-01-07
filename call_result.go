package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

type CallResult struct {
	positionsByType map[reflect.Type][]int
	Values          []reflect.Value
}

// Extract extracts results by positions
func (c CallResult) Extract(targets ...any) {
	for i, target := range targets {
		if target == nil {
			continue
		}
		if i >= len(c.Values) {
			panic(errors.Join(
				fmt.Errorf("not enough values for targets: have %d, want at least %d", len(c.Values), i+1),
				ErrBadArgument,
			))
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Pointer {
			panic(errors.Join(
				fmt.Errorf("%T is not a pointer", target),
				ErrBadArgument,
			))
		}
		if targetValue.IsNil() { // prevent reflect panics on nil pointers
			panic(errors.Join(
				fmt.Errorf("cannot assign to a nil pointer target of type %v", targetValue.Type()),
				ErrBadArgument,
			))
		}
		if !c.Values[i].Type().AssignableTo(targetValue.Type().Elem()) {
			panic(errors.Join(
				fmt.Errorf("cannot assign value of type %v to target of type %v", c.Values[i].Type(), targetValue.Type().Elem()),
				ErrBadArgument,
			))
		}
		targetValue.Elem().Set(c.Values[i])
	}
}

func (c CallResult) Assign(targets ...any) {
	assigned := make([]bool, len(c.Values))
	for _, target := range targets {
		if target == nil {
			continue
		}
		targetValue := reflect.ValueOf(target)
		if targetValue.Kind() != reflect.Pointer {
			panic(errors.Join(
				fmt.Errorf("%v is not a pointer", target),
				ErrBadArgument,
			))
		}
		if targetValue.IsNil() {
			panic(errors.Join(
				fmt.Errorf("cannot assign to a nil pointer target of type %v", targetValue.Type()),
				ErrBadArgument,
			))
		}
		targetType := targetValue.Type().Elem()
		found := false
		for i, v := range c.Values {
			if !assigned[i] && v.Type().AssignableTo(targetType) {
				targetValue.Elem().Set(v)
				assigned[i] = true
				found = true
				break
			}
		}
		if !found {
			panic(errors.Join(
				fmt.Errorf("no return values of type %v", targetType),
				ErrBadArgument,
			))
		}
	}
}