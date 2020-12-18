package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrDependencyLoop = errors.New("dependency loop")

var ErrDependencyNotFound = errors.New("dependency not found")

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
