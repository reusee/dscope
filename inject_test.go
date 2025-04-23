package dscope

import "testing"

func TestInject(t *testing.T) {
	New(
		Provide(int(42)),
	).Call(func(
		inject InjectStruct,
	) {
		var s struct {
			I Inject[int]
		}
		if err := inject(&s); err != nil {
			panic(err)
		}
		if s.I() != 42 {
			t.Fatal()
		}
	})
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
