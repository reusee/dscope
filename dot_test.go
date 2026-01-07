package dscope

import (
	"strconv"
	"strings"
	"testing"
)

func TestScopeToDOT(t *testing.T) {
	type A int
	type B string
	type C float64
	type D bool

	scope := New(
		func() A { return 1 },
		func(a A) B { return B(strconv.Itoa(int(a))) },
		func(a A, b B) C { return C(float64(a) + float64(len(b))) },
	).Fork(
		func(c C) D { return c > 1.0 }, // Override nothing, add D
	).Fork(
		func(a A) B { return "overridden" }, // Override B
	)

	buf := new(strings.Builder)
	err := scope.ToDOT(buf)
	if err != nil {
		t.Fatal(err)
	}
	dot := buf.String()
	t.Log(dot) // Print DOT graph for manual inspection

	// Basic checks for presence of types and dependencies
	if !strings.Contains(dot, "Defined By: func() dscope.A") {
		t.Error("Missing definition info for A")
	}
	if !strings.Contains(dot, "Defined By: func(dscope.A) dscope.B") { // Check for the *overriding* definition
		t.Error("Missing or incorrect definition info for B")
	}
	if !strings.Contains(dot, "->") { // Check if any edges were generated
		t.Error("No dependency edges found in DOT output")
	}
	if !strings.Contains(dot, "digraph dscope") {
		t.Error("DOT output seems invalid")
	}

}

func TestToDOTInjectStruct(t *testing.T) {
	scope := New(func(inject InjectStruct) int {
		return 42
	})
	buf := new(strings.Builder)
	if err := scope.ToDOT(buf); err != nil {
		t.Fatal(err)
	}
	dot := buf.String()
	if !strings.Contains(dot, "InjectStruct") {
		t.Fatal("InjectStruct missing from DOT graph")
	}
}