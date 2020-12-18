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
	return fmt.Sprintf("init: %T", i.Value)
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
