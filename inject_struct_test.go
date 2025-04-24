package dscope

import "testing"

func TestInjectStruct(t *testing.T) {
	// dependable
	New(func(
		inject InjectStruct,
	) int {
		return 42
	})

	scope := New(
		func() int {
			return 42
		},
	)
	var s struct {
		I int `dscope:"."`
	}
	injectStruct(scope, &s)
	if s.I != 42 {
		t.Fatal()
	}

	scope.Call(func(
		inject InjectStruct,
	) {
		var s struct {
			I int `dscope:"."`
		}
		inject(&s)
		if s.I != 42 {
			t.Fatal()
		}
	})
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
		injectStruct(scope, &s)
	}
}
