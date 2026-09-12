package dscope

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestMethods(t *testing.T) {
	s := New(Methods(new(TestMethodsFoo))...)
	if s.Get[int]() != 42 {
		t.Fatal()
	}
}

type TestMethodsFoo struct {
	Module
}

func (TestMethodsFoo) Foo() int {
	return 42
}

func TestMethodFromFields(t *testing.T) {
	type Foo struct {
		Foo TestMethodsFoo
	}
	defs := Methods(new(Foo))
	if len(defs) == 0 {
		t.Fatal()
	}
}

func TestMethodsNil(t *testing.T) {

	t.Run("nil typed", func(t *testing.T) {
		type M struct {
			Module
		}
		Methods((*M)(nil))
	})

	t.Run("nil pointer to interface", func(t *testing.T) {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			msg := fmt.Sprintf("%v", p)
			if !strings.Contains(msg, "nil pointer to interface *io.Reader") {
				t.Fatalf("got %s", msg)
			}
		}()
		Methods((*io.Reader)(nil))
	})

	t.Run("nil interface", func(t *testing.T) {
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				msg := fmt.Sprintf("%v", p)
				if !strings.Contains(msg, "invalid value") {
					t.Fatalf("got %s", msg)
				}
			}()
			Methods((io.Reader)(nil))
		}()
	})
}

type testMethodsValueMod struct {
	Module
}

func (testMethodsValueMod) Value() int64    { return 1 }
func (*testMethodsValueMod) Pointer() int32 { return 2 }

type testMethodsContainer struct {
	Module
	V testMethodsValueMod
}

func TestMethodsEmbeddedValue(t *testing.T) {
	// This test ensures that we can discover methods on the pointer receiver
	// of a module embedded by value.
	scope := New(Methods(&testMethodsContainer{})...)

	// Should find Value() (int64)
	if v := scope.Get[int64](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}

	// Should find Pointer() (int32)
	// Before fix, this fails because we only visit testMethodsValueMod as a value
	if v := scope.Get[int32](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
}

func TestMethodsDoublePointer(t *testing.T) {
	// This test verifies that we can extract methods from a pointer to a pointer
	// e.g. passing **T should find methods defined on *T
	m := &testMethodsValueMod{}
	scope := New(Methods(&m)...)

	// Should find Value() (int64) from T
	if v := scope.Get[int64](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	// Should find Pointer() (int32) from *T
	if v := scope.Get[int32](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
}

func TestMethodsMultiPointerNil(t *testing.T) {
	// This test verifies that Methods handles multi-level nil pointers without panicking.
	// e.g., passing **T(nil) should work.
	defs := Methods((**TestMethodsFoo)(nil))
	if len(defs) == 0 {
		t.Fatal("no methods found for multi-level nil pointer")
	}
}

func TestMethodsEmbeddedValuePassedByValue(t *testing.T) {
	// This test verifies that we can capture methods on pointer receivers
	// of embedded modules even when the parent struct is passed by value (non-addressable).
	m := testMethodsContainer{
		V: testMethodsValueMod{},
	}
	// Pass by value
	scope := New(Methods(m)...)

	// Should find Pointer() (int32) from *testMethodsValueMod
	// This would fail if we didn't create an addressable copy of the embedded field
	if v := scope.Get[int32](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}

	// Should find Value() (int64) from testMethodsValueMod
	if v := scope.Get[int64](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
}

type testMethodsRootValue struct {
	Module
}

func (testMethodsRootValue) Value() int64    { return 1 }
func (*testMethodsRootValue) Pointer() int32 { return 2 }

func TestMethodsRootValueAddressability(t *testing.T) {
	m := testMethodsRootValue{}
	// Passing by value
	scope := New(Methods(m)...)

	// Should find Value() -> int64
	if v := scope.Get[int64](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}

	// Should find Pointer() -> int32
	// Without fix, this fails to find the provider for int32
	if v := scope.Get[int32](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
}

func TestMethodsMultiLevelNilInterface(t *testing.T) {
	t.Run("nil double pointer to interface", func(t *testing.T) {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			msg := err.Error()
			if !strings.Contains(msg, "nil pointer to interface **io.Reader") {
				t.Fatalf("got %s", msg)
			}
		}()
		Methods((**io.Reader)(nil))
	})

	t.Run("nil triple pointer to interface", func(t *testing.T) {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			msg := err.Error()
			if !strings.Contains(msg, "nil pointer to interface ***io.Reader") {
				t.Fatalf("got %s", msg)
			}
		}()
		Methods((***io.Reader)(nil))
	})
}

func TestMethodsRecursivePointer(t *testing.T) {
	type RecursivePointer *RecursivePointer
	var pointer RecursivePointer

	defer func() {
		panicValue := recover()
		if panicValue == nil {
			t.Fatal("should panic")
		}
		err, ok := panicValue.(error)
		if !ok {
			t.Fatalf("panic value not an error: %v", panicValue)
		}
		if !strings.Contains(err.Error(), "recursive pointer type") {
			t.Fatalf("unexpected error message: %v", err)
		}
	}()

	Methods(pointer)
}

type testMethodsPromotedLeaf struct {
	Module
}

func (testMethodsPromotedLeaf) LeafValue() int32 { return 2 }

// TestMethodsPromotedInner defines a method of its own and carries a named
// module field, so it exercises both promotion and descent. The type name is
// exported because an embedded field whose type name is unexported is itself
// an unexported field, which method discovery skips.
type TestMethodsPromotedInner struct {
	Module
	Leaf testMethodsPromotedLeaf
}

func (TestMethodsPromotedInner) InnerValue() int64 { return 1 }

// testMethodsPromotedOuter embeds an exported module by value, so InnerValue is
// promoted into its method set.
type testMethodsPromotedOuter struct {
	TestMethodsPromotedInner
}

func (testMethodsPromotedOuter) OuterValue() string { return "outer" }

func TestMethodsEmbeddedModulePromotion(t *testing.T) {
	// InnerValue is promoted into Outer, so discovery must collect it once:
	// before the fix this returned four definitions and New panicked with
	// "int64 has multiple definitions in the same Fork call".
	defs := Methods(new(testMethodsPromotedOuter))
	if n := len(defs); n != 3 {
		t.Fatalf("expected 3 definitions, got %d", n)
	}

	scope := New(defs...)
	if v := scope.Get[int64](); v != 1 {
		t.Fatalf("promoted module method not provided: got %d", v)
	}
	if v := scope.Get[string](); v != "outer" {
		t.Fatalf("module's own method not provided: got %s", v)
	}
	if v := scope.Get[int32](); v != 2 {
		t.Fatalf("method of a named module field inside an embedded module not provided: got %d", v)
	}
}
