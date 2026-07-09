package dscope

import (
	"reflect"
	"testing"
)

func TestInject(t *testing.T) {
	New(
		Provide(int(42)),
	).Call(func(
		inject InjectStruct,
	) {
		var s struct {
			I Inject[int]
		}
		inject(&s)
		if s.I() != 42 {
			t.Fatal()
		}
	})
}

func TestGetInjectStruct(t *testing.T) {
	// Regression test: Get[InjectStruct] must return a value of the named
	// InjectStruct type, not the unnamed func(any) type produced by the
	// method value. Otherwise the type assertion in Get[T] panics.
	scope := New(Provide(int(42)))

	inject := Get[InjectStruct](scope)
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

	v, ok := scope.Get(reflect.TypeFor[InjectStruct]())
	if !ok {
		t.Fatal("Get(reflect.Type) returned ok=false")
	}
	if v.Interface().(InjectStruct) == nil {
		t.Fatal("type assertion to InjectStruct failed or nil")
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
