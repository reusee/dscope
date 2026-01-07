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
	satisfied := make([]bool, len(targets))
	targetValues := make([]reflect.Value, len(targets))
	for i, target := range targets {
		if target == nil {
			satisfied[i] = true
			continue
		}
		v := reflect.ValueOf(target)
		if v.Kind() != reflect.Pointer {
			panic(errors.Join(fmt.Errorf("%v is not a pointer", target), ErrBadArgument))
		}
		if v.IsNil() {
			panic(errors.Join(fmt.Errorf("cannot assign to a nil pointer target of type %v", v.Type()), ErrBadArgument))
		}
		targetValues[i] = v
	}
	// Two-pass matching to prevent interface targets from "stealing" concrete return values
	for pass := 0; pass < 2; pass++ { // pass 0: exact matches, pass 1: assignable matches
		for i, v := range targetValues {
			if satisfied[i] {
				continue
			}
			typ := v.Type().Elem()
			for j, val := range c.Values {
				if assigned[j] {
					continue
				}
				if (pass == 0 && val.Type() == typ) || (pass == 1 && val.Type().AssignableTo(typ)) {
					v.Elem().Set(val)
					assigned[j] = true
					satisfied[i] = true
					break
				}
			}
		}
	}
	for i, ok := range satisfied {
		if !ok {
			panic(errors.Join(fmt.Errorf("no return values of type %v", targetValues[i].Type().Elem()), ErrBadArgument))
		}
	}
}