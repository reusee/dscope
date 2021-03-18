package dscope

import (
	"errors"
	"reflect"
)

type NoShadow interface {
	IsNoShadow()
}

var noShadowType = reflect.TypeOf((*NoShadow)(nil)).Elem()

var ErrBadShadow = errors.New("bad shadowing declaration")
