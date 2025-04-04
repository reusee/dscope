package dscope

import "testing"

type mod1 struct {
	Module
	Mod2 mod2 `dscope:"."`
}

type mod2 struct {
}

func (mod2) Int() int {
	return 42
}

func (mod2) Int8() int8 {
	return 42
}

func TestModule(t *testing.T) {
	scope := New(
		Provide(float32(42)),
		new(mod1),
		Provide(float64(42)),
	)
	Get[float32](scope)
	Get[int](scope)
	Get[int8](scope)
	Get[float64](scope)
}
