package dscope

import (
	"reflect"
	"testing"
)

func TestReduceNil(t *testing.T) {
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !is(err, ErrNoValues) {
				t.Fatal()
			}
		}()
		Reduce(nil)
	}()
}

func TestReduceMap(t *testing.T) {
	m := Reduce([]reflect.Value{
		reflect.ValueOf(map[int]int{
			1: 2,
		}),
		reflect.ValueOf(map[int]int{
			3: 4,
		}),
	}).Interface().(map[int]int)
	if len(m) != 2 {
		t.Fatal()
	}
	if m[1] != 2 {
		t.Fatal()
	}
	if m[3] != 4 {
		t.Fatal()
	}
}
