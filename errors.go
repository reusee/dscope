package dscope

import (
	"fmt"
	"reflect"
)

type ErrDependencyLoop struct {
	Value any
}

var _ error = ErrDependencyLoop{}

func (e ErrDependencyLoop) Error() string {
	return fmt.Sprintf("dependency loop: %T", e.Value)
}

type ErrDependencyNotFound struct {
	Type reflect.Type
	By   any
}

var _ error = ErrDependencyNotFound{}

func (e ErrDependencyNotFound) Error() string {
	if e.By != nil {
		return fmt.Sprintf("dependency not found: %T requires %v", e.By, e.Type)
	}
	return fmt.Sprintf("dependency not found: %v", e.Type)
}

type ErrBadArgument struct {
	Value  any
	Reason string
}

var _ error = ErrBadArgument{}

func (e ErrBadArgument) Error() string {
	return fmt.Sprintf("bad argument, %s: %T", e.Reason, e.Value)
}
