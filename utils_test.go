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

func (_ TestMethodsFoo) Foo() int {
	return 42
}
