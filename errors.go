package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrDependencyLoop = errors.New("dependency loop")

var ErrDependencyNotFound = errors.New("dependency not found")

var ErrBadArgument = errors.New("bad argument")

type ErrBadDeclaration struct {
	Type   reflect.Type
	Reason string
}

var _ error = ErrBadDeclaration{}

func (e ErrBadDeclaration) Error() string {
	return fmt.Sprintf("bad declaration, %s: %v", e.Reason, e.Type)
}
