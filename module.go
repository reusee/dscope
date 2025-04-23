package dscope

import "reflect"

type Module struct{}

type isModule interface {
	isDscopeModule()
}

var isModuleType = reflect.TypeFor[isModule]()

func (Module) isDscopeModule() {}
