package dscope

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCallResult(t *testing.T) {
	scope := New(func() int {
		return 42
	})

	t.Run("Extract", func(t *testing.T) {
		res := scope.Call(func(i int) int {
			return i * 2
		})
		var i int
		res.Extract(&i)
		if i != 84 {
			t.Fatal()
		}
	})

	t.Run("Extract to nil", func(t *testing.T) {
		res := scope.Call(func(i int) int {
			return i * 2
		})
		res.Extract(nil, nil)
	})

	t.Run("Extract non pointer", func(t *testing.T) {
		res := scope.Call(func(i int) int {
			return i * 2
		})
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				msg := fmt.Sprintf("%v", p)
				if !strings.Contains(msg, "int is not a pointer") {
					t.Fatalf("got %v", msg)
				}
			}()
			res.Extract(42)
		}()
	})

	t.Run("Assign target type not found", func(t *testing.T) {
		res := scope.Call(func(i int) int {
			return i * 2
		})
		var s string
		res.Assign(&s)
	})

	t.Run("Extract too many", func(t *testing.T) {
		res := scope.Call(func(i int) int {
			return i * 2
		})
		var i, j int
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				msg := fmt.Sprintf("%v", p)
				if !strings.Contains(msg, "not enough values for targets") {
					t.Fatalf("got %s", msg)
				}
			}()
			res.Extract(&i, &j)
		}()
	})

}

func TestCallResultAssign(t *testing.T) {

	t.Run("basic", func(t *testing.T) {
		result := New(func() int {
			return 42
		}).Call(func(i int) (int, int) {
			return 42, 1
		})

		var i int
		result.Assign(nil, &i)
		if i != 42 {
			t.Fatal()
		}

		var i2 int
		result.Assign(&i2, &i)
		if i != 1 {
			t.Fatal()
		}
		if i2 != 42 {
			t.Fatal()
		}
	})

	t.Run("too many target", func(t *testing.T) {
		scope := New()
		res := scope.Call(func() (int, string, int) {
			return 1, "foo", 2
		})
		var i1, i2, i3 int

		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			if !errors.Is(err, ErrBadArgument) {
				t.Errorf("expected ErrBadArgument, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "not enough return values of type int to assign to target (wanted at least 3, have 2)") {
				t.Errorf("unexpected error message: %s", err.Error())
			}
		}()
		res.Assign(&i1, &i2, &i3)
	})
}
