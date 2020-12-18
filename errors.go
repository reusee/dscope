package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrDependencyLoop = errors.New("dependency loop")

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

type ErrBadDeclaration struct {
	Type   reflect.Type
	Reason string
}

var _ error = ErrBadDeclaration{}

func (e ErrBadDeclaration) Error() string {
	return fmt.Sprintf("bad declaration, %s: %v", e.Reason, e.Type)
}
