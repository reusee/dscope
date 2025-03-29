package dscope

import "testing"

func TestInjectStruct(t *testing.T) {
	scope := New(
		func() int {
			return 42
		},
	)
	var s struct {
		I int `dscope:"."`
	}
	err := InjectStruct(scope, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.I != 42 {
		t.Fatal()
	}
}

func BenchmarkInjectStruct(b *testing.B) {
	scope := New(
		func() int {
			return 42
		},
	)
	var s struct {
		I int `dscope:"."`
	}
	for b.Loop() {
		err := InjectStruct(scope, &s)
		if err != nil {
			b.Fatal(err)
		}
	}
}
