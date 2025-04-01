package dscope

import "testing"

func TestMethods(t *testing.T) {
	s := New(Methods(new(TestMethodsFoo))...)
	s.Call(func(
		foo int,
	) {
		if foo != 42 {
			t.Fatal()
		}
	})
}

type TestMethodsFoo struct{}

func (TestMethodsFoo) Foo() int {
	return 42
}

func TestMethodFromFields(t *testing.T) {
	type Foo struct {
		Scope          Scope `dscope:"."`
		DuplicateScope Scope `dscope:"methods"`
	}
	defs := Methods(Foo{})
	if len(defs) == 0 {
		t.Fatal()
	}
}
