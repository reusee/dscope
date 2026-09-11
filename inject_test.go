package dscope

import (
	"testing"
)

func TestInject(t *testing.T) {
	inject := New(
		Provide(int(42)),
	).Get[InjectStruct]()
	var s struct {
		I Inject[int]
	}
	inject(&s)
	if s.I() != 42 {
		t.Fatal()
	}
}

func TestGetInjectStruct(t *testing.T) {
	// Regression test: Get[InjectStruct] must return a value of the named
	// InjectStruct type, not the unnamed func(any) type produced by the
	// method value. Otherwise the type assertion in Scope.Get[T] panics.
	scope := New(Provide(int(42)))

	inject := scope.Get[InjectStruct]()
	if inject == nil {
		t.Fatal("got nil InjectStruct")
	}
	var s struct {
		I int `dscope:"."`
	}
	inject(&s)
	if s.I != 42 {
		t.Fatalf("injected %d, want 42", s.I)
	}

	var inject2 InjectStruct
	scope.Assign(&inject2)
	if inject2 == nil {
		t.Fatal("Assign got nil InjectStruct")
	}
}

func BenchmarkInject(b *testing.B) {
	scope := New(
		Provide(int(42)),
	)
	var s struct {
		I Inject[int]
	}
	for b.Loop() {
		scope.InjectStruct(&s)
	}
}
