package dscope

import (
	"fmt"
	"io"
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

func TestInjectStructNested(t *testing.T) {
	type Inner struct {
		I int `dscope:"."`
	}
	type Outer struct {
		S     string `dscope:"."`
		Inner        // Nested struct
	}

	scope := New(
		Provide(42),    // Provides int
		Provide("foo"), // Provides string
	)

	var outer Outer
	injectStruct(scope, &outer)

	if outer.S != "foo" {
		t.Fatalf("Outer string field not injected: got %q, want %q", outer.S, "foo")
	}
	if outer.Inner.I != 42 {
		t.Fatalf("Nested struct int field not injected: got %d, want %d", outer.Inner.I, 42)
	}
}

func TestInjectStructNilPointerTarget(t *testing.T) {
	scope := New(Provide(42)) // Provide something to make injection possible

	type MyStruct struct {
		I int `dscope:"."`
	}

	t.Run("nil pointer to struct", func(t *testing.T) {
		var ptr *MyStruct = nil // Nil pointer target
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				msg := fmt.Sprintf("%v", p)
				// Check for the expected panic message indicating a nil pointer target
				if !strings.Contains(msg, "cannot inject into a nil pointer target of type *dscope.MyStruct") {
					t.Fatalf("got %v", msg)
				}
			}()
			injectStruct(scope, ptr) // Call with nil pointer
		}()
	})

	t.Run("non-nil pointer to struct", func(t *testing.T) {
		ptr := &MyStruct{} // Non-nil pointer target
		injectStruct(scope, ptr)
		if ptr.I != 42 {
			t.Fatalf("injection failed for non-nil pointer: got %d, want %d", ptr.I, 42)
		}
	})

	t.Run("nil interface holding nil pointer to struct", func(t *testing.T) {
		var target any = (*MyStruct)(nil) // nil interface holding nil pointer
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				msg := fmt.Sprintf("%v", p)
				// Check for the expected panic message
				if !strings.Contains(msg, "cannot inject into a nil pointer target of type *dscope.MyStruct") {
					t.Fatalf("got %v", msg)
				}
			}()
			injectStruct(scope, target)
		}()
	})

	t.Run("non-nil interface holding non-nil pointer to struct", func(t *testing.T) {
		var target any = &MyStruct{} // non-nil interface holding non-nil pointer
		injectStruct(scope, target)
		ptr := target.(*MyStruct)
		if ptr.I != 42 {
			t.Fatalf("injection failed for non-nil interface holding pointer: got %d, want %d", ptr.I, 42)
		}
	})

	t.Run("embedded interface", func(t *testing.T) {
		type Foo struct {
			io.Reader // ignore
		}
		var foo Foo
		New().InjectStruct(&foo)
	})

	t.Run("embedded pointers", func(t *testing.T) {
		type Foo struct {
			*int
		}
		var foo Foo
		New().InjectStruct(&foo)
	})

	t.Run("private field", func(t *testing.T) {
		type Foo struct {
			i int `dscope:"."`
		}
		var foo Foo
		New(func() int {
			return 42
		}).InjectStruct(&foo)
	})

}
