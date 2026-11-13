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

	inject := scope.Get[InjectStruct]()
	var s2 struct {
		I int `dscope:"."`
	}
	inject(&s2)
	if s2.I != 42 {
		t.Fatal()
	}

	// The "inject" tag is an alias of ".".
	var s3 struct {
		I int `dscope:"inject"`
	}
	inject(&s3)
	if s3.I != 42 {
		t.Fatal()
	}
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

func TestInjectStructNotFound(t *testing.T) {
	t.Run("wrapped", func(t *testing.T) {
		inject := New().Get[InjectStruct]()
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

	t.Run("tagged", func(t *testing.T) {
		inject := New().Get[InjectStruct]()
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
}

// TestInjectStructBadTarget verifies that a nil, non-pointer, or non-struct
// injection target panics with a structured bad-argument error.
func TestInjectStructBadTarget(t *testing.T) {
	scope := New(Provide(42))

	checkPanic := func(t *testing.T, target any, message string) {
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
			if !strings.Contains(err.Error(), message) {
				t.Fatalf("unexpected error message: %s", err.Error())
			}
		}()
		scope.InjectStruct(target)
	}

	t.Run("nil target", func(t *testing.T) {
		checkPanic(t, nil, "target must be a pointer")
	})

	t.Run("non-pointer target", func(t *testing.T) {
		checkPanic(t, struct {
			I int `dscope:"."`
		}{}, "target must be a pointer")
	})

	t.Run("non-struct target type", func(t *testing.T) {
		checkPanic(t, Provide(Provide(Provide(42))), "is not a struct or pointer to struct")
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

func TestInjectStructPointerToInject(t *testing.T) {
	// This test reproduces a panic where InjectStruct tries to process *Inject[T]
	// because it implements the isInject interface, but it's not a function.
	type S struct {
		I *Inject[int]
	}
	scope := New(Provide(42))
	var s S
	// Should not panic. It should simply ignore the field or handle it gracefully.
	scope.InjectStruct(&s)
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

func TestInjectStructIgnoredTag(t *testing.T) {
	type EmbeddedPtr struct {
		Missing float64 `dscope:"."`
	}
	type EmbeddedVal struct {
		Val int `dscope:"."`
	}
	type Target struct {
		Kept         int         `dscope:"."`
		SkippedVal   int         `dscope:"-"`
		SkippedLazy  Inject[int] `dscope:"-"`
		*EmbeddedPtr `dscope:"-"`
		EmbeddedVal  `dscope:"-"`
	}

	scope := New(Provide(42))
	var target Target
	scope.InjectStruct(&target)

	if target.Kept != 42 {
		t.Fatalf("Kept = %d, want 42", target.Kept)
	}
	if target.SkippedVal != 0 {
		t.Fatalf("SkippedVal = %d, want 0", target.SkippedVal)
	}
	if target.SkippedLazy != nil {
		t.Fatal("SkippedLazy should remain nil when tagged dscope:\"-\"")
	}
	if target.EmbeddedPtr != nil {
		t.Fatal("EmbeddedPtr should remain nil when tagged dscope:\"-\"")
	}
	if target.EmbeddedVal.Val != 0 {
		t.Fatalf("EmbeddedVal.Val = %d, want 0", target.EmbeddedVal.Val)
	}
}

type TestDualBranch1 struct {
	*TestDualRoot
}

type TestDualBranch2 struct {
	*TestDualRoot
}

type TestDualRoot struct {
	*TestDualBranch1
	*TestDualBranch2
}

func TestInjectStructCircularEmbeddingRejectedUpFront(t *testing.T) {
	t.Run("self cycle", func(t *testing.T) {
		type Rec struct {
			*Rec
		}
		var rec Rec
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				err, ok := p.(error)
				if !ok || !errors.Is(err, ErrBadArgument) {
					t.Fatalf("expected ErrBadArgument, got %T: %v", p, p)
				}
			}()
			New().InjectStruct(&rec)
		}()
		if rec.Rec != nil {
			t.Fatal("InjectStruct allocated circular embedded pointers before rejecting circular type")
		}
	})

	t.Run("branching cycle", func(t *testing.T) {
		var root TestDualRoot
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic on branching circular embedding")
				}
				err, ok := p.(error)
				if !ok || !errors.Is(err, ErrBadArgument) {
					t.Fatalf("expected ErrBadArgument, got %T: %v", p, p)
				}
				if !strings.Contains(err.Error(), "recursive struct injection depth limit exceeded") {
					t.Fatalf("unexpected error message: %v", err)
				}
			}()
			New().InjectStruct(&root)
		}()
		if root.TestDualBranch1 != nil || root.TestDualBranch2 != nil {
			t.Fatal("InjectStruct allocated branching circular embedded pointers before rejecting circular type")
		}
	})

	t.Run("diamond non-cyclic embedding allowed", func(t *testing.T) {
		type SharedLeaf struct {
			V int `dscope:"."`
		}
		type Left struct {
			*SharedLeaf
		}
		type Right struct {
			*SharedLeaf
		}
		type Diamond struct {
			*Left
			*Right
		}
		var d Diamond
		New(Provide(77)).InjectStruct(&d)
		if d.Left == nil || d.Left.SharedLeaf == nil || d.Left.V != 77 {
			t.Fatalf("Left branch not injected: %+v", d.Left)
		}
		if d.Right == nil || d.Right.SharedLeaf == nil || d.Right.V != 77 {
			t.Fatalf("Right branch not injected: %+v", d.Right)
		}
	})
}
