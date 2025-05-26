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

		// Ensure we have enough values for this target
		if i >= len(c.Values) {
			panic(errors.Join(
				fmt.Errorf("not enough values for targets: have %d, want at least %d",
					len(c.Values), i+1),
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
		if !c.Values[i].Type().AssignableTo(targetValue.Type().Elem()) {
			panic(errors.Join(
				fmt.Errorf("cannot assign value of type %v to target of type %v",
					c.Values[i].Type(),
					targetValue.Type().Elem()),
				ErrBadArgument,
			))
		}
		targetValue.Elem().Set(c.Values[i])
	}
}

// Assign assigns targets by types
func (c CallResult) Assign(targets ...any) {
	offsets := make(map[reflect.Type]int)
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
		typ := targetValue.Type().Elem()
		poses, ok := c.positionsByType[typ]
		if !ok {
			continue
		}
		offset := offsets[typ]
		offsets[typ]++
		if offset >= len(poses) {
			panic(errors.Join(
				fmt.Errorf("not enough return values of type %v to assign to target (wanted at least %d, have %d)",
					typ, offset+1, len(poses)),
				ErrBadArgument,
			))
		}
		targetValue.Elem().Set(c.Values[poses[offset]])
	}
}
