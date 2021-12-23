package dscope

import "testing"

func TestDefs(t *testing.T) {
	defs := Defs{
		func() int {
			return 42
		},
	}
	defs.Add(func(i int) int8 {
		return int8(i % 256)
	})
	var i8 int8
	defs.NewScope().Assign(&i8)
	if i8 != 42 {
		t.Fatal()
	}
}
