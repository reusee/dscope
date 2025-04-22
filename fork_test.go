package dscope

import "testing"

func TestProvideFork(t *testing.T) {
	New(
		func() int {
			return 42
		},
	).Call(func(
		fork Fork,
	) {
		fork(Provide(int(1))).Call(func(
			i int,
		) {
			if i != 1 {
				t.Fatal()
			}
		})
	})
}
