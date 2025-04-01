package dscope

import (
	"reflect"
	"testing"
)

func TestReduce(t *testing.T) {
	vs := []reflect.Value{
		reflect.ValueOf([]int{1, 2, 3}),
		reflect.ValueOf([]int{4, 5, 6}),
	}
	v := Reduce(vs)
	slice, ok := v.Interface().([]int)
	if !ok {
		t.Fatal()
	}
	if len(slice) != 6 {
		t.Fatal()
	}
	for i, n := range slice {
		if n != i+1 {
			t.Fatal()
		}
	}

	type S string
	vs = []reflect.Value{
		reflect.ValueOf(S("foo")),
		reflect.ValueOf(S("bar")),
	}
	v = Reduce(vs)
	str, ok := v.Interface().(S)
	if !ok {
		t.Fatal()
	}
	if str != "foobar" {
		t.Fatal()
	}

}
