package dscope

type Module struct{}

type isModule interface {
	isDscopeModule()
}

func (Module) isDscopeModule() {}
