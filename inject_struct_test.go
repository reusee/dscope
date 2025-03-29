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
	err := injectStruct(scope, &s)
	if err != nil {
		t.Fatal(err)
	}
	if s.I != 42 {
		t.Fatal()
	}

	scope.Call(func(
		inject InjectStruct,
	) {
		var s struct {
			I int `dscope:"."`
		}
		if err := inject(&s); err != nil {
			t.Fatal(err)
		}
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
		err := injectStruct(scope, &s)
		if err != nil {
			b.Fatal(err)
		}
	}
}
