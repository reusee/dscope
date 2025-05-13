package dscope

import (
	"fmt"
	"strings"
	"testing"
)

func TestInjectStruct(t *testing.T) {
	// dependable
	New(func(
		inject InjectStruct,
	) int {
		return 42
	})

	scope := New(
		func() int {
			return 42
		},
	)
	var s struct {
		I int `dscope:"."`
	}
	injectStruct(scope, &s)
	if s.I != 42 {
		t.Fatal()
	}

	scope.Call(func(
		inject InjectStruct,
	) {
		var s struct {
			I int `dscope:"."`
		}
		inject(&s)
		if s.I != 42 {
			t.Fatal()
		}
	})
}

func BenchmarkInjectStruct(b *testing.B) {
	scope := New(
		func() int {
			return 42
		},
	)
	var s struct {
		I int `dscope:"."`
	}
	for b.Loop() {
		injectStruct(scope, &s)
	}
}

func TestInjectStructBadType(t *testing.T) {
	New().Call(func(
		inject InjectStruct,
	) {
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				msg := fmt.Sprintf("%v", p)
				if !strings.Contains(msg, "target type int is not a struct or pointer to struct") {
					t.Fatalf("got %v", msg)
				}
			}()
			inject(42)
		}()
	})
}

func TestInjectStructNotFound(t *testing.T) {
	t.Run("wrapped", func(t *testing.T) {
		New().Call(func(
			inject InjectStruct,
		) {
			var s struct {
				I Inject[int]
			}
			inject(&s)
			func() {
				defer func() {
					p := recover()
					if p == nil {
						t.Fatal("should panic")
					}
					msg := fmt.Sprintf("%v", p)
					if !strings.Contains(msg, "no definition for int") {
						t.Fatalf("got %v", msg)
					}
				}()
				s.I()
			}()
		})
	})

	t.Run("tagged", func(t *testing.T) {
		New().Call(func(
			inject InjectStruct,
		) {
			var s struct {
				I int `dscope:"."`
			}
			func() {
				defer func() {
					p := recover()
					if p == nil {
						t.Fatal("should panic")
					}
					msg := fmt.Sprintf("%v", p)
					if !strings.Contains(msg, "no definition for int") {
						t.Fatalf("got %v", msg)
					}
				}()
				inject(&s)
			}()
		})
	})
}
