package dscope

import "testing"

func TestWithFork(t *testing.T) {
	New(
		PtrTo(42),
	).Call(func(
		i WithFork[int],
	) {
		n := i(PtrTo(1))
		if n != 1 {
			t.Fatalf("got %v", n)
		}
	})
}
