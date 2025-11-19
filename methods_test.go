package dscope

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestMethods(t *testing.T) {
	s := New(Methods(new(TestMethodsFoo))...)
	s.Call(func(
		foo int,
	) {
		if foo != 42 {
			t.Fatal()
		}
	})
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
	if v := Get[int64](scope); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}

	// Should find Pointer() (int32)
	// Before fix, this fails because we only visit testMethodsValueMod as a value
	if v := Get[int32](scope); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
}

func TestMethodsDoublePointer(t *testing.T) {
	// This test verifies that we can extract methods from a pointer to a pointer
	// e.g. passing **T should find methods defined on *T
	m := &testMethodsValueMod{}
	scope := New(Methods(&m)...)

	// Should find Value() (int64) from T
	if v := Get[int64](scope); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	// Should find Pointer() (int32) from *T
	if v := Get[int32](scope); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
}
