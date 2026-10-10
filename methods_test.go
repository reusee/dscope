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
		for _, tc := range []struct {
			name   string
			object any
			typ    string
		}{
			{"single", (*io.Reader)(nil), "*io.Reader"},
			{"double", (**io.Reader)(nil), "**io.Reader"},
			{"triple", (***io.Reader)(nil), "***io.Reader"},
		} {
			func() {
				defer func() {
					p := recover()
					if p == nil {
						t.Fatalf("%s: should panic", tc.name)
					}
					msg := fmt.Sprintf("%v", p)
					if !strings.Contains(msg, "nil pointer to interface "+tc.typ) {
						t.Fatalf("%s: got %s", tc.name, msg)
					}
				}()
				Methods(tc.object)
			}()
		}
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

func TestMethodsNilChainProviderCallable(t *testing.T) {
	// A typed nil pointer anywhere on the chain must be materialised, so a
	// provider collected from a mid-chain pointer binds to a non-nil receiver
	// instead of panicking when invoked.
	t.Run("nil double pointer", func(t *testing.T) {
		scope := New(Methods((**TestMethodsFoo)(nil))...)
		if v := scope.Get[int](); v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	})
	t.Run("nil triple pointer", func(t *testing.T) {
		scope := New(Methods((***TestMethodsFoo)(nil))...)
		if v := scope.Get[int](); v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	})
	t.Run("non-nil outer nil inner", func(t *testing.T) {
		scope := New(Methods(new(*TestMethodsFoo))...)
		if v := scope.Get[int](); v != 42 {
			t.Fatalf("expected 42, got %d", v)
		}
	})
	t.Run("value and pointer receivers", func(t *testing.T) {
		scope := New(Methods((**testMethodsValueMod)(nil))...)
		if v := scope.Get[int64](); v != 1 {
			t.Fatalf("expected 1, got %d", v)
		}
		if v := scope.Get[int32](); v != 2 {
			t.Fatalf("expected 2, got %d", v)
		}
	})
}

type testMethodsRootValue struct {
	Module
}

func (testMethodsRootValue) Value() int64    { return 1 }
func (*testMethodsRootValue) Pointer() int32 { return 2 }

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

// TestMethodsAddressability verifies that method discovery finds the methods of
// pointer receivers on values that are not addressable: a module held in a
// field, a module passed by value, and a root struct passed by value.
func TestMethodsAddressability(t *testing.T) {
	for _, tc := range []struct {
		name   string
		object any
	}{
		{"container pointer", &testMethodsContainer{}},
		{"container value", testMethodsContainer{}},
		{"root value", testMethodsRootValue{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope := New(Methods(tc.object)...)
			if v := scope.Get[int64](); v != 1 {
				t.Fatalf("expected 1, got %d", v)
			}
			if v := scope.Get[int32](); v != 2 {
				t.Fatalf("expected 2, got %d", v)
			}
		})
	}
}
