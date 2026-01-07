package dscope

import "testing"

type mod1 struct {
	Module
	Mod2 mod2
	Mod3 mod3
}

type mod2 struct {
	Module
}

func (mod2) Int() int {
	return 42
}

func (mod2) Int8() int8 {
	return 42
}

type mod3 struct {
	Module
}

func (mod3) Int16() int16 {
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
	Get[int16](scope)
}

func TestModuleInjectable(t *testing.T) {
	type Mod struct {
		Module
		Val int
	}
	m := &Mod{Val: 42}
	scope := New(m)

	// Providing *Mod as a value provider should make type Mod available in the scope.
	// Before the fix, Mod is missing because the pointer to it (which is a module)
	// was stripped from the definitions in Scope.Fork.
	val := Get[Mod](scope)
	if val.Val != 42 {
		t.Fatalf("expected 42, got %d", val.Val)
	}
}