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
