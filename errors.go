package dscope

import (
	"errors"
	"reflect"

	"github.com/reusee/e5"
)

var ErrDependencyLoop = errors.New("dependency loop")

var ErrDependencyNotFound = errors.New("dependency not found")

var ErrBadArgument = errors.New("bad argument")

var ErrBadDefinition = errors.New("bad definition")

func throwErrDependencyNotFound(typ reflect.Type) {
	_ = throw(we.With(
		e5.Info("no definition for %v", typ),
	)(
		ErrDependencyNotFound,
	))
}
