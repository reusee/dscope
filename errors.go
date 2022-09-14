package dscope

import (
	"errors"
	"fmt"
	"strings"
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
	buf.WriteString(fmt.Sprintf("%v", typeIDToType(p.TypeID)))
	if p.Prev != nil {
		buf.WriteString(" <- ")
		buf.WriteString(p.Prev.String())
	}
	return buf.String()
}

func (p Path) Error() string {
	return p.String()
}
