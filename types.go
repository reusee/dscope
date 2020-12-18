package dscope

import (
	"fmt"
	"reflect"
	"strings"
)

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
	Value  any
	Reason string
}

func (a ArgInfo) Error() string {
	return fmt.Sprintf("arg: %s: %#v", a.Reason, a.Value)
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
