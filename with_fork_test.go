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

	New(
		PtrTo(1),
		func(i int) uint {
			return uint(i * 2)
		},
	).Call(func(
		u WithFork[uint],
	) {
		if u(PtrTo(2)) != 4 {
			t.Fatal()
		}
	})

}

func TestWithForkAsDeps(t *testing.T) {
	var i int64
	New(
		PtrTo(42),
		func(
			i WithFork[int],
		) int64 {
			return int64(
				i(PtrTo(1)) * 2,
			)
		},
	).Assign(&i)
	if i != 2 {
		t.Fatal()
	}
}

func TestWithForkN(t *testing.T) {
	s := New(
		PtrTo(42),
		func(i int) int8 {
			return int8(i)
		},
		func(i int) int16 {
			return int16(i * 2)
		},
	)
	var with WithFork2[int8, int16]
	s.Assign(&with)
	i1, i2 := with(PtrTo(1))
	if i1 != 1 {
		t.Fatal()
	}
	if i2 != 2 {
		t.Fatal()
	}
}
