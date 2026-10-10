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

// errWith joins a formatted message with a sentinel error, so callers can
// classify the result with errors.Is.
func errWith(sentinel error, format string, args ...any) error {
	return errors.Join(fmt.Errorf(format, args...), sentinel)
}

func throwErrDependencyNotFound(typ reflect.Type) {
	panic(errWith(ErrDependencyNotFound, "no definition for %v", typ))
}
