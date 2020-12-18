package dscope

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var ErrDependencyLoop = errors.New("dependency loop")

var ErrDependencyNotFound = errors.New("dependency not found")

var ErrBadArgument = errors.New("bad argument")

var ErrBadDeclaration = errors.New("bad declaration")

type InitInfo struct {
	Value any
	Name  string
}

func (i InitInfo) Error() string {
	return fmt.Sprintf("init: name %s, type %T", i.Name, i.Value)
}

type TypeInfo struct {
	Type reflect.Type
}

func (t TypeInfo) Error() string {
	return fmt.Sprintf("type: %v", t.Type)
}

type ArgInfo struct {
	Value any
}

func (a ArgInfo) Error() string {
	return fmt.Sprintf("arg: %#v", a.Value)
}

type Reason string

func (r Reason) Error() string {
	return "reason: " + string(r)
}

type Path []reflect.Type

func (p Path) Error() string {
	buf := new(strings.Builder)
	buf.WriteString("path: ")
	for i, t := range p {
		if i > 0 {
			buf.WriteString(" > ")
		}
		buf.WriteString(fmt.Sprintf("%v", t))
	}
	return buf.String()
}
