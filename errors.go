package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

var ErrDependencyLoop = errors.New("dependency loop")

var ErrDependencyNotFound = errors.New("dependency not found")

var ErrBadArgument = errors.New("bad argument")

var ErrBadDefinition = errors.New("bad definition")

func throwErrDependencyNotFound(typ reflect.Type) {
	panic(errors.Join(
		fmt.Errorf("no definition for %v", typ),
		ErrDependencyNotFound,
	))
}
