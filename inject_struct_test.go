package dscope

import (
	"errors"
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
	injectStruct(scope, &s, 0)
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
		injectStruct(scope, &s, 0)
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
			inject(ptrTo(ptrTo(ptrTo(42))))
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
	injectStruct(scope, &outer, 0)

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
			injectStruct(scope, ptr, 0) // Call with nil pointer
		}()
	})

	t.Run("non-nil pointer to struct", func(t *testing.T) {
		ptr := &MyStruct{} // Non-nil pointer target
		injectStruct(scope, ptr, 0)
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
			injectStruct(scope, target, 0)
		}()
	})

	t.Run("non-nil interface holding non-nil pointer to struct", func(t *testing.T) {
		var target any = &MyStruct{} // non-nil interface holding non-nil pointer
		injectStruct(scope, target, 0)
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

func TestInjectStructEmbeddedNilPointer(t *testing.T) {
	type Inner struct {
		I int `dscope:"."`
	}
	type Outer struct {
		*Inner // Embedded nil pointer to struct
	}

	scope := New(Provide(42))
	var outer Outer
	// outer.Inner is nil here

	// This should not panic. It should allocate and inject into Inner.
	scope.InjectStruct(&outer)

	if outer.Inner == nil {
		t.Fatal("Embedded pointer field was not initialized")
	}
	if outer.Inner.I != 42 {
		t.Fatalf("Nested struct int field not injected: got %d, want %d", outer.Inner.I, 42)
	}
}

func TestInjectStructWithInjectTag(t *testing.T) {
	scope := New(
		Provide(int(42)),
	)
	var s struct {
		I int `dscope:"inject"`
	}
	scope.InjectStruct(&s)
	if s.I != 42 {
		t.Fatal()
	}
}

func TestInjectStructWithTaggedInjectField(t *testing.T) {
	scope := New(
		Provide(int(42)),
	)
	var s struct {
		I Inject[int] `dscope:"."`
	}
	scope.InjectStruct(&s)
	if s.I() != 42 {
		t.Fatal()
	}
}

func TestInjectStructTaggedEmbedded(t *testing.T) {
	type Inner struct {
		I int `dscope:"."`
	}
	type Outer struct {
		Inner // Embedded, no tag -> should recurse
	}

	scope := New(
		Provide(42), // Provides int, but not Inner
	)

	var outer Outer
	// Before the fix, this panics because it tries to find a provider for `Inner`.
	// After the fix, this should recursively inject into the embedded struct.
	scope.InjectStruct(&outer)

	if outer.I != 42 {
		t.Fatalf("Embedded tagged struct field not injected: got %d, want %d", outer.I, 42)
	}
}

func TestInjectStructNonPointerTarget(t *testing.T) {
	scope := New(Provide(42))
	// Pass a non‑pointer struct value; should panic with ErrBadArgument.
	var s struct {
		I int `dspace:"."`
	}
	defer func() {
		if p := recover(); p == nil {
			t.Fatal("should panic")
		} else {
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			if !errors.Is(err, ErrBadArgument) {
				t.Fatalf("expected ErrBadArgument, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "target must be a pointer") {
				t.Fatalf("unexpected error message: %s", err.Error())
			}
		}
	}()
	injectStruct(scope, s, 0) // non‑pointer target
}

func TestInjectStructEmbeddedValue(t *testing.T) {
	type Inner struct {
		I int `dscope:"."`
	}
	type Outer struct {
		Inner `dscope:"."` // Tagged embedded field -> should inject value from scope
	}

	// Case 1: Inner is provided in scope. Should be injected.
	scope := New(
		Provide(42),
		func() Inner {
			return Inner{I: 99}
		},
	)
	var outer Outer
	scope.InjectStruct(&outer)
	if outer.Inner.I != 99 {
		t.Fatalf("Tagged embedded struct should receive value from scope, got %d want 99", outer.Inner.I)
	}

	// Case 2: Inner is NOT provided. Should panic (not recurse).
	scope2 := New(Provide(42))
	var outer2 Outer
	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("should panic when tagged embedded struct is missing in scope")
			}
		}()
		scope2.InjectStruct(&outer2)
	}()
}

func TestInjectStructNilIntermediatePointer(t *testing.T) {
	type S struct {
		I int `dscope:"."`
	}
	scope := New(Provide(42))

	t.Run("pointer to nil pointer", func(t *testing.T) {
		var s *S // nil
		// InjectStruct takes &s which is **S.
		scope.InjectStruct(&s)
		if s == nil {
			t.Fatal("s is still nil")
		}
		if s.I != 42 {
			t.Fatalf("s.I = %d, want 42", s.I)
		}
	})

	t.Run("pointer to pointer to nil pointer", func(t *testing.T) {
		var s **S // nil
		scope.InjectStruct(&s)
		if s == nil {
			t.Fatal("s is nil")
		}
		if *s == nil {
			t.Fatal("*s is nil")
		}
		if (*s).I != 42 {
			t.Fatal("not injected")
		}
	})
}

func TestInjectStructRecursivePointer(t *testing.T) {
	type T *T
	var ptr T
	func() {
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
				t.Fatalf("expected ErrBadArgument, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "recursive pointer type") {
				t.Fatalf("unexpected error message: %s", err.Error())
			}
		}()
		injectStruct(New(), &ptr, 0)
	}()
}

func TestInjectStructNil(t *testing.T) {
	scope := New()
	defer func() {
		p := recover()
		if p == nil {
			t.Fatal("InjectStruct(nil) should panic")
		}
		err, ok := p.(error)
		if !ok {
			t.Fatalf("panic value not an error: %v", p)
		}
		// We expect a structured ErrBadArgument, not a raw reflect panic.
		if !errors.Is(err, ErrBadArgument) {
			t.Errorf("expected ErrBadArgument, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "target must be a pointer") {
			t.Errorf("unexpected error message: %s", err.Error())
		}
	}()
	// This currently causes a reflect panic because Type() is called on invalid value
	scope.InjectStruct(nil)
}

func TestInjectStructCircularEmbedding(t *testing.T) {
	// This test verifies that InjectStruct detects circular embedding and panics
	// instead of causing a stack overflow.
	type Rec struct {
		*Rec
	}
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
			t.Fatalf("expected ErrBadArgument, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "recursive struct injection depth limit exceeded") {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	}()
	New().InjectStruct(&Rec{})
}

