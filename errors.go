package dscope

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/reusee/e5"
)

var ErrDependencyLoop = errors.New("dependency loop")

var ErrDependencyNotFound = errors.New("dependency not found")

var ErrBadArgument = errors.New("bad argument")

var ErrBadDefinition = errors.New("bad definition")

type Path struct {
	Prev   *Path
	TypeID _TypeID
}

func (p *Path) String() string {
	buf := new(strings.Builder)
	fmt.Fprintf(buf, "%v", typeIDToType(p.TypeID))
	if p.Prev != nil {
		buf.WriteString(" <- ")
		buf.WriteString(p.Prev.String())
	}
	return buf.String()
}

func (p Path) Error() string {
	return p.String()
}

func throwErrDependencyNotFound(typ reflect.Type, path *Path) {
	_ = throw(we.With(
		e5.Info("no definition for %v", typ),
		e5.Info("path: %+v", path),
	)(
		ErrDependencyNotFound,
	))
}
