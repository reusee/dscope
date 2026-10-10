package dscope

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func TestAllTypes(t *testing.T) {
	scope := New(
		func() (int32, int64) {
			return 42, 42
		},
		func() (string, float64) {
			return "foo", 42
		},
	)

	var names []string
	for t := range scope.AllTypes() {
		names = append(names, fmt.Sprintf("%v", t))
	}
	slices.Sort(names)
	if str := fmt.Sprintf("%v", names); str != "[dscope.Fork dscope.InjectStruct dscope.Reset float64 int32 int64 string]" {
		t.Fatalf("got %v", str)
	}

	scope = scope.Fork(
		func() int32 {
			return 42
		},
		func() int8 {
			return 42
		},
	)
	names = nil
	for t := range scope.AllTypes() {
		names = append(names, fmt.Sprintf("%v", t))
	}
	slices.Sort(names)
	if str := fmt.Sprintf("%v", names); str != "[dscope.Fork dscope.InjectStruct dscope.Reset float64 int32 int64 int8 string]" {
		t.Fatalf("got %v", str)
	}

	// early break
	for range scope.AllTypes() {
		break
	}

}

// TestAllTypesBuiltins verifies that each built-in dependency appears exactly
// once in AllTypes, also when a user definition for the same type is present.
func TestAllTypesBuiltins(t *testing.T) {
	scope := New(
		func() InjectStruct {
			return func(target any) {}
		},
		func() Fork {
			return func(defs ...any) Scope { return New() }
		},
		func() Reset {
			return func() Scope { return New() }
		},
	)
	for _, typ := range []reflect.Type{
		reflect.TypeFor[InjectStruct](),
		reflect.TypeFor[Fork](),
		reflect.TypeFor[Reset](),
	} {
		var count int
		for got := range scope.AllTypes() {
			if got == typ {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("%v should appear exactly once in AllTypes, got %d", typ, count)
		}
	}
}
