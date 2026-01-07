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
	if str := fmt.Sprintf("%v", names); str != "[dscope.InjectStruct float64 int32 int64 string]" {
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
	if str := fmt.Sprintf("%v", names); str != "[dscope.InjectStruct float64 int32 int64 int8 string]" {
		t.Fatalf("got %v", str)
	}

	// early break
	for range scope.AllTypes() {
		break
	}

}

func TestAllTypesInjectStruct(t *testing.T) {
	scope := New()
	found := false
	for typ := range scope.AllTypes() {
		if typ == reflect.TypeFor[InjectStruct]() {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("InjectStruct not found in AllTypes")
	}
}

