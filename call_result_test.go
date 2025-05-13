package dscope

import (
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

}
