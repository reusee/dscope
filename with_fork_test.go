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
		func(i int) int32 {
			return int32(i * 3)
		},
		func(i int) int64 {
			return int64(i * 4)
		},
		func(i int) uint8 {
			return uint8(i * 5)
		},
		func(i int) uint16 {
			return uint16(i * 6)
		},
		func(i int) uint32 {
			return uint32(i * 7)
		},
		func(i int) uint64 {
			return uint64(i * 8)
		},
	)

	{
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

	{
		var with WithFork3[int8, int16, int32]
		s.Assign(&with)
		i1, i2, i3 := with(PtrTo(1))
		if i1 != 1 {
			t.Fatal()
		}
		if i2 != 2 {
			t.Fatal()
		}
		if i3 != 3 {
			t.Fatal()
		}
	}

	{
		var with WithFork4[int8, int16, int32, int64]
		s.Assign(&with)
		i1, i2, i3, i4 := with(PtrTo(1))
		if i1 != 1 {
			t.Fatal()
		}
		if i2 != 2 {
			t.Fatal()
		}
		if i3 != 3 {
			t.Fatal()
		}
		if i4 != 4 {
			t.Fatal()
		}
	}

	{
		var with WithFork5[int8, int16, int32, int64, uint8]
		s.Assign(&with)
		i1, i2, i3, i4, i5 := with(PtrTo(1))
		if i1 != 1 {
			t.Fatal()
		}
		if i2 != 2 {
			t.Fatal()
		}
		if i3 != 3 {
			t.Fatal()
		}
		if i4 != 4 {
			t.Fatal()
		}
		if i5 != 5 {
			t.Fatal()
		}
	}

	{
		var with WithFork6[int8, int16, int32, int64, uint8, uint16]
		s.Assign(&with)
		i1, i2, i3, i4, i5, i6 := with(PtrTo(1))
		if i1 != 1 {
			t.Fatal()
		}
		if i2 != 2 {
			t.Fatal()
		}
		if i3 != 3 {
			t.Fatal()
		}
		if i4 != 4 {
			t.Fatal()
		}
		if i5 != 5 {
			t.Fatal()
		}
		if i6 != 6 {
			t.Fatal()
		}
	}

	{
		var with WithFork7[int8, int16, int32, int64, uint8, uint16, uint32]
		s.Assign(&with)
		i1, i2, i3, i4, i5, i6, i7 := with(PtrTo(1))
		if i1 != 1 {
			t.Fatal()
		}
		if i2 != 2 {
			t.Fatal()
		}
		if i3 != 3 {
			t.Fatal()
		}
		if i4 != 4 {
			t.Fatal()
		}
		if i5 != 5 {
			t.Fatal()
		}
		if i6 != 6 {
			t.Fatal()
		}
		if i7 != 7 {
			t.Fatal()
		}
	}

	{
		var with WithFork8[int8, int16, int32, int64, uint8, uint16, uint32, uint64]
		s.Assign(&with)
		i1, i2, i3, i4, i5, i6, i7, i8 := with(PtrTo(1))
		if i1 != 1 {
			t.Fatal()
		}
		if i2 != 2 {
			t.Fatal()
		}
		if i3 != 3 {
			t.Fatal()
		}
		if i4 != 4 {
			t.Fatal()
		}
		if i5 != 5 {
			t.Fatal()
		}
		if i6 != 6 {
			t.Fatal()
		}
		if i7 != 7 {
			t.Fatal()
		}
		if i8 != 8 {
			t.Fatal()
		}
	}

	{
		s = s.Fork(
			func(
				with WithFork2[
					int8, int16,
				],
			) func() {
				return func() {
					i1, i2 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

	{
		s = s.Fork(
			func(
				with WithFork3[
					int8, int16, int32,
				],
			) func() {
				return func() {
					i1, i2, i3 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
					if i3 != 3 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

	{
		s = s.Fork(
			func(
				with WithFork4[
					int8, int16, int32, int64,
				],
			) func() {
				return func() {
					i1, i2, i3, i4 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
					if i3 != 3 {
						t.Fatal()
					}
					if i4 != 4 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

	{
		s = s.Fork(
			func(
				with WithFork5[
					int8, int16, int32, int64,
					uint8,
				],
			) func() {
				return func() {
					i1, i2, i3, i4, i5 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
					if i3 != 3 {
						t.Fatal()
					}
					if i4 != 4 {
						t.Fatal()
					}
					if i5 != 5 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

	{
		s = s.Fork(
			func(
				with WithFork6[
					int8, int16, int32, int64,
					uint8, uint16,
				],
			) func() {
				return func() {
					i1, i2, i3, i4, i5, i6 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
					if i3 != 3 {
						t.Fatal()
					}
					if i4 != 4 {
						t.Fatal()
					}
					if i5 != 5 {
						t.Fatal()
					}
					if i6 != 6 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

	{
		s = s.Fork(
			func(
				with WithFork7[
					int8, int16, int32, int64,
					uint8, uint16, uint32,
				],
			) func() {
				return func() {
					i1, i2, i3, i4, i5, i6, i7 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
					if i3 != 3 {
						t.Fatal()
					}
					if i4 != 4 {
						t.Fatal()
					}
					if i5 != 5 {
						t.Fatal()
					}
					if i6 != 6 {
						t.Fatal()
					}
					if i7 != 7 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

	{
		s = s.Fork(
			func(
				with WithFork8[
					int8, int16, int32, int64,
					uint8, uint16, uint32, uint64,
				],
			) func() {
				return func() {
					i1, i2, i3, i4, i5, i6, i7, i8 := with(PtrTo(1))
					if i1 != 1 {
						t.Fatal()
					}
					if i2 != 2 {
						t.Fatal()
					}
					if i3 != 3 {
						t.Fatal()
					}
					if i4 != 4 {
						t.Fatal()
					}
					if i5 != 5 {
						t.Fatal()
					}
					if i6 != 6 {
						t.Fatal()
					}
					if i7 != 7 {
						t.Fatal()
					}
					if i8 != 8 {
						t.Fatal()
					}
				}
			},
		)
		var fn func()
		s.Assign(&fn)
		fn()
	}

}
