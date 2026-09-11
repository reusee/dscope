package dscope

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAssign(t *testing.T) {
	// types
	type IntA int
	type IntB int
	type IntC int

	// New
	scope := New().Fork(
		func(b IntB) IntA {
			return IntA(42 + b)
		},
		func() IntB {
			return IntB(24)
		},
		func() IntC {
			return IntC(42)
		},
	)

	// Assign
	var a IntA
	scope.Assign(&a)
	if a != 66 {
		t.Fatal()
	}

	var c IntC
	scope.Assign(&c)
	if c != 42 {
		t.Fatal()
	}

}

func TestPanic(t *testing.T) {
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrBadArgument) {
				t.Fatal()
			}
			if !strings.Contains(err.Error(), "returns nothing") {
				t.Fatal()
			}
		}()
		New(
			func(i int) {
			},
		)
	}()

	scope := New()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrBadArgument) {
				t.Fatal()
			}
			if !strings.Contains(err.Error(), "not a valid definition") {
				t.Fatal()
			}
		}()
		scope.Fork(42)
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrBadArgument) {
				t.Fatal()
			}
			if !strings.Contains(err.Error(), "nil pointer target") {
				t.Fatal()
			}
		}()
		var p *int
		scope.Assign(p)
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
			if !strings.Contains(err.Error(), "not found") {
				t.Fatal()
			}
		}()
		var s string
		scope.Assign(&s)
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
		}()
		scope.Fork(func(string) int64 {
			return 0
		}).Get[int64]()
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
		}()
		scope.Fork(
			func(string) int32 {
				return 0
			},
		)
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrDependencyLoop) {
				t.Fatal()
			}
		}()
		scope = scope.Fork(
			func(s string) string {
				return "42"
			},
		)
		var s string
		scope.Assign(&s)
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrBadDefinition) {
				t.Fatal()
			}
			if !strings.Contains(err.Error(), "has multiple definitions") {
				t.Fatal()
			}
		}()
		New(
			func() int {
				return 1
			},
			func() int {
				return 2
			},
		)
	}()

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatal()
			}
			if !errors.Is(err, ErrBadDefinition) {
				t.Fatalf("expected ErrBadDefinition, got %T: %v", p, p)
			}
			if !strings.Contains(err.Error(), "has multiple definitions") {
				t.Fatalf("unexpected error message: %v", err)
			}
		}()
		i := 42
		New(
			&i,
			func() int {
				return 2
			},
		)
	}()

}

func TestDuplicateDefinitionErrorDetails(t *testing.T) {
	t.Run("pointer and function", func(t *testing.T) {
		i := 42
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			if !errors.Is(err, ErrBadDefinition) {
				t.Fatalf("expected ErrBadDefinition, got %T: %v", err, err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "int has multiple definitions") {
				t.Fatalf("missing type in error message: %s", msg)
			}
			if !strings.Contains(msg, "*int") {
				t.Fatalf("missing pointer definition in error message: %s", msg)
			}
			if !strings.Contains(msg, "func() int") {
				t.Fatalf("missing function definition in error message: %s", msg)
			}
			if !strings.Contains(msg, "definition #1") || !strings.Contains(msg, "definition #2") {
				t.Fatalf("missing definition indices in error message: %s", msg)
			}
		}()
		New(
			&i,
			func() int { return 2 },
		)
	})

	t.Run("same function duplicate output", func(t *testing.T) {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			if !errors.Is(err, ErrBadDefinition) {
				t.Fatalf("expected ErrBadDefinition, got %T: %v", err, err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "int has multiple definitions") {
				t.Fatalf("missing type in error message: %s", msg)
			}
			if !strings.Contains(msg, "output 0") || !strings.Contains(msg, "output 1") {
				t.Fatalf("missing output indices in error message: %s", msg)
			}
		}()
		New(func() (int, int) { return 1, 2 })
	})
}

func TestForkScope(t *testing.T) {
	type Foo int
	type Bar int
	type Baz int
	scope := New().Fork(
		func(
			bar Bar,
			baz Baz,
		) Foo {
			return Foo(bar) + Foo(baz)
		},
		func() Bar {
			return 42
		},
		func() Baz {
			return 24
		},
	)
	var foo Foo
	scope.Assign(&foo)
	if foo != 66 {
		t.Fatal("bad foo")
	}
}

func TestForkScope2(t *testing.T) {
	scope := New()
	scope1 := scope.Fork(func() int {
		return 42
	})
	scope2 := scope.Fork(func() int {
		return 36
	})
	var i int
	scope1.Assign(&i)
	if i != 42 {
		t.Fail()
	}
	scope2.Assign(&i)
	if i != 36 {
		t.Fail()
	}
}

func TestLoadOnce(t *testing.T) {
	var scope Scope
	scope = New()

	n := 0
	type Foo int
	scope = scope.Fork(
		func() Foo {
			n++
			scope = scope.Fork(func() Foo {
				return 44
			})
			return 42
		},
	)

	var f Foo
	scope.Assign(&f)
	if f != 42 {
		t.Fatal()
	}
	if n != 1 {
		t.Fatal()
	}
	scope.Assign(&f)
	if f != 44 {
		t.Fatal()
	}
	if n != 1 {
		t.Fatal()
	}
	scope.Assign(&f)
	if f != 44 {
		t.Fatal()
	}
	if n != 1 {
		t.Fatal()
	}
	scope.Assign(&f)
	if f != 44 {
		t.Fatal()
	}
	if n != 1 {
		t.Fatal()
	}

}

func TestLoadFunc(t *testing.T) {
	scope := New().Fork(
		func() func() {
			return func() {}
		},
	)
	var f func()
	scope.Assign(&f)
	f()
}

func TestOnce(t *testing.T) {
	var numCalled int64
	scope := New().Fork(
		func() int {
			atomic.AddInt64(&numCalled, 1)
			return 42
		},
	)
	n := 1024
	wg := new(sync.WaitGroup)
	wg.Add(n)
	for range n {
		go func() {
			var i int
			scope.Assign(&i)
			if i != 42 {
				panic("fail")
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if numCalled != 1 {
		t.Fatal()
	}
}

func TestIndirectDependencyLoop(t *testing.T) {
	type A int
	type B int
	type C int
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			if !strings.Contains(
				fmt.Sprintf("%v", p),
				"dependency loop",
			) {
				t.Fatalf("unexpected: %v", p)
			}
		}()
		New().Fork(
			func(a A) B {
				return 42
			},
			func(b B) C {
				return 42
			},
			func(c C) A {
				return 42
			},
		)
	}()
}

func TestOverride(t *testing.T) {
	scope := New().Fork(
		func() int {
			return 42
		},
	).Fork(
		func() int {
			return 24
		},
	)
	var i int
	scope.Assign(&i)
	if i != 24 {
		t.Fatal()
	}
}

func TestOnceFunc(t *testing.T) {
	var numCalled int64
	scope := New().Fork(
		func() func() int {
			atomic.AddInt64(&numCalled, 1)
			return func() int {
				return 42
			}
		},
	)
	n := 1024
	wg := new(sync.WaitGroup)
	wg.Add(n)
	for range n {
		go func() {
			var fn func() int
			scope.Assign(&fn)
			if fn() != 42 {
				panic("fail")
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if numCalled != 1 {
		t.Fatal()
	}
}

func TestMultiProvide(t *testing.T) {
	scope := New().Fork(
		func() (int, string) {
			return 42, "42"
		},
	)
	var i int
	var s string
	scope.Assign(&i)
	scope.Assign(&s)
}

func TestForkLazyMulti(t *testing.T) {
	var numCalled int64
	scope := New().Fork(
		func() (int, string) {
			atomic.AddInt64(&numCalled, 1)
			return 42, "42"
		},
	)
	n := 1024
	wg := new(sync.WaitGroup)
	wg.Add(n * 2)
	for range n {
		go func() {
			var i int
			scope.Assign(&i)
			if i != 42 {
				panic("fail")
			}
			wg.Done()
		}()
		go func() {
			var i int
			scope.Assign(&i)
			if i != 42 {
				panic("fail")
			}
			wg.Done()
		}()
	}
	wg.Wait()
	if numCalled != 1 {
		t.Fatal()
	}
}

func TestInterfaceDef(t *testing.T) {
	type Foo any
	scope := New().Fork(
		func() Foo {
			return Foo(42)
		},
	)
	var f Foo
	scope.Assign(&f)
	if f != 42 {
		t.Fatal()
	}
	type Bar any
	s := scope.Fork(
		func() Bar {
			return Bar(24)
		},
	)
	var b Bar
	s.Assign(&b)
	if b != 24 {
		t.Fatal()
	}
}

func TestBadOnceSharing(t *testing.T) {
	scope := New().Fork(
		func() int {
			return 1
		},
	)
	scope2 := scope.Fork()
	var a int
	scope.Assign(&a)
	scope2.Assign(&a)
}

func TestGeneratedFunc(t *testing.T) {
	type S string
	fnType := reflect.FuncOf(
		[]reflect.Type{
			reflect.TypeFor[int](),
			reflect.TypeFor[string](),
		},
		[]reflect.Type{
			reflect.TypeFor[S](),
		},
		false,
	)
	fn := reflect.MakeFunc(
		fnType,
		func(args []reflect.Value) []reflect.Value {
			i := args[0].Int()
			s := args[1].String()
			return []reflect.Value{
				reflect.ValueOf(
					S(fmt.Sprintf("%d-%s", i, s)),
				),
			}
		},
	).Interface()
	scope := New().Fork(
		func() int {
			return 42
		},
		func() string {
			return "42"
		},
		fn,
	)
	var s S
	scope.Assign(&s)
	if s != "42-42" {
		t.Fail()
	}
}

func TestRecalculate(t *testing.T) {
	type A int
	type B1 int
	type B2 int
	type C1 int
	type C2 int
	type D int
	type Foo int
	numFooCalled := 0

	scope := New().Fork(
		func() A {
			return 1
		},
		func(a A) B1 {
			return B1(a) + 1
		},
		func(a A) B2 {
			return B2(a) + 2
		},
		func(b1 B1, b2 B2) C1 {
			return C1(b1) + C1(b2)
		},
		func(b1 B1, b2 B2) C2 {
			return C2(b1) * C2(b2)
		},
		func(a A, b1 B1, b2 B2, c1 C1, c2 C2) D {
			return D(a) + D(b1) + D(b2) + D(c1) + D(c2)
		},
		func() Foo {
			numFooCalled++
			return 42
		},
	)

	var d D
	scope.Assign(&d)
	if d != 17 {
		t.Fatal()
	}
	var f Foo
	scope.Assign(&f)
	if f != 42 {
		t.Fatal()
	}
	if numFooCalled != 1 {
		t.Fatal()
	}

	scope2 := scope.Fork(
		func() A {
			return 2
		},
	)
	scope2.Assign(&d)
	// a = 2
	// b1 = 3
	// b2 = 4
	// c1 = 7
	// c2 = 12
	if d != 28 {
		t.Fatal()
	}
	// not affected
	scope.Assign(&f)
	if f != 42 {
		t.Fatal()
	}
	if numFooCalled != 1 {
		t.Fatal()
	}

	// scope not affected
	scope.Assign(&d)
	if d != 17 {
		t.Fatal()
	}

	// partial update
	scope3 := scope2.Fork(
		func() B2 {
			return 1
		},
	)
	// a = 2
	// b1 = 3
	// b2 = 1
	// c1 = 4
	// c2 = 3
	scope3.Assign(&d)
	if d != 13 {
		t.Fatal()
	}

	scope.Assign(&f)
	scope2.Assign(&f)
	scope3.Assign(&f)
	if numFooCalled != 1 {
		t.Fatal()
	}

	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			if !strings.Contains(
				fmt.Sprintf("%v", p),
				"dependency loop",
			) {
				t.Fatalf("unexpected: %v", p)
			}
		}()
		scope3.Fork(
			func(d D) A {
				return A(d) + 1
			},
		)
	}()

}

func TestPartialOverride(t *testing.T) {
	type A int
	type B int
	type C int

	scope := New().Fork(
		func() (A, B) {
			return 1, 2
		},
		func(a A, b B) C {
			return C(a) + C(b)
		},
	)
	var c C
	scope.Assign(&c)
	if c != 3 {
		t.Fatal()
	}

	scope2 := scope.Fork(
		func() A {
			return 10
		},
	)
	scope2.Assign(&c)
	if c != 12 {
		t.Fatal()
	}

	scope.Assign(&c)
	if c != 3 {
		t.Fatal()
	}
}

func TestRecalculateMultipleProvide(t *testing.T) {
	type A int
	type B int
	type C int

	scope := New().Fork(
		func() A {
			return 42
		},
		func(a A) (B, C) {
			return B(a * 2), C(a * 3)
		},
	)
	var b B
	scope.Assign(&b)
	if b != 84 {
		t.Fatal()
	}

	scope2 := scope.Fork(
		func() A {
			return 31
		},
	)
	scope2.Assign(&b)
	if b != 62 {
		t.Fatal()
	}

	scope.Assign(&b)
	if b != 84 {
		t.Fatal()
	}

	var c C
	scope.Assign(&c)
	if c != 126 {
		t.Fatal()
	}
	scope2.Assign(&c)
	if c != 93 {
		t.Fatal()
	}
}

func TestRacing(t *testing.T) {
	scope := New(
		func() int {
			return 2
		},
	)
	n := 16
	wg := new(sync.WaitGroup)
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			s := scope.Fork(
				func() int {
					return 42
				},
			)
			var i int
			s.Assign(&i)
		}()
	}
	wg.Wait()
}

func TestOverwrite(t *testing.T) {
	scope := New().Fork(
		func() (int, string) {
			return 42, "42"
		},
	)
	n := 0
	s2 := scope.Fork(
		func() (int, string) {
			n++
			return 24, "24"
		},
	)
	var i int
	var s string
	s2.Assign(&i)
	s2.Assign(&s)
	if n != 1 {
		t.Fatal()
	}
}

func TestOverrideAndNewDep(t *testing.T) {
	scope := New(
		func(string) int {
			return 42
		},
		func() string {
			return "42"
		},
	)
	scope.Fork(
		func() string {
			return "42"
		},
		func(string) bool {
			return true
		},
	)
}

func TestFlatten(t *testing.T) {
	scope := New(
		func() int {
			return 42
		},
	)
	s := scope
	for range 128 {
		s = s.Fork(
			func() int {
				return 43
			},
		)
	}
	var i int
	scope.Assign(&i)
	if i != 42 {
		t.Fatal()
	}
	s.Assign(&i)
	if i != 43 {
		t.Fatal()
	}
}

func TestOverwriteNew(t *testing.T) {
	scope := New(
		func() int {
			return 42
		},
		func(i int) string {
			return strconv.Itoa(i)
		},
	)
	scope2 := scope.Fork(
		func() int {
			return 24
		},
		func(i int) string {
			return "foo"
		},
	)

	if scope.Get[int]() != 42 {
		t.Fatal()
	}
	if scope.Get[string]() != "42" {
		t.Fatal()
	}
	if scope2.Get[int]() != 24 {
		t.Fatal()
	}
	if scope2.Get[string]() != "foo" {
		t.Fatal()
	}
}

func TestPointerProvider(t *testing.T) {
	i := float64(42)
	scope := New(
		&i,
		func(f float64) int {
			return int(f)
		},
	)
	if scope.Get[int]() != 42 {
		t.Fatal()
	}

	scope = New(
		func(f float64) int {
			return int(f)
		},
		&i,
	)
	if scope.Get[int]() != 42 {
		t.Fatal()
	}

	scope = New(
		func(f float64) int {
			return int(f)
		},
		&i,
	).Fork(
		func() int {
			return 24
		},
	)
	if scope.Get[int]() != 24 {
		t.Fatal()
	}

	scope = New(
		func(f float64) int {
			return int(f)
		},
		&i,
	).Fork(
		func() float64 {
			return 24
		},
	)
	if scope.Get[int]() != 24 {
		t.Fatal()
	}

	scope = New(func() int {
		return 42
	}).Fork(func() *int {
		i := 42
		return &i
	}())
	if scope.Get[int]() != 42 {
		t.Fatal()
	}

}

func TestRacyGet(t *testing.T) {
	s := New(func() int {
		return 42
	})
	for range 512 {
		go func() {
			s.Get[int]()
		}()
	}
}

func TestForkFunc(t *testing.T) {
	type I int
	type J int
	type K int

	s1 := New(func() I {
		return 42
	}, func(i I) J {
		return J(i) * 2
	})
	s2 := New(func() I {
		return 42
	}, func() J {
		return 42
	})
	var j J
	s1.Assign(&j)
	if j != 84 {
		t.Fatal()
	}

	s1 = s1.Fork(func() K {
		return 42
	})
	s2 = s2.Fork(func() K {
		return 42
	})

	s2.Fork(func() I {
		return 1
	})
	s1 = s1.Fork(func() I {
		return 1
	})

	s1.Assign(&j)
	if j != 2 {
		t.Fatal()
	}

}

func TestForkFuncKey(t *testing.T) {
	s := New()
	s1 := New()
	if s.forkFuncKey != s1.forkFuncKey {
		t.Fatal()
	}

	s1 = s1.Fork(func() int {
		return 42
	})
	s = s.Fork(func() int {
		return 42
	})
	if s.forkFuncKey != s1.forkFuncKey {
		t.Fatal()
	}

	s1 = s1.Fork(func() string {
		return "foo"
	})
	s = s.Fork(func() int {
		return 42
	})
	if s.forkFuncKey == s1.forkFuncKey {
		t.Fatal()
	}
}

func TestSignature(t *testing.T) {
	s := New().Fork(
		func() int {
			return 42
		},
	).Fork(
		func() string {
			return "foo"
		},
	)
	s2 := New().Fork(
		func() int {
			return 42
		},
		func() string {
			return "foo"
		},
	)
	if s.signature != s2.signature {
		t.Fatal()
	}

	s = s.Fork(func() int {
		return 1
	})
	s2 = s2.Fork(func() int {
		return 1
	})
	var i int
	s.Assign(&i)
	if i != 1 {
		t.Fatal()
	}
	s2.Assign(&i)
	if i != 1 {
		t.Fatal()
	}

}

func TestProviderManyArgs(t *testing.T) {
	type Result int
	base := New(func() int {
		return 42
	})
	intType := reflect.TypeFor[int]()
	resultType := reflect.TypeFor[Result]()
	for i := 0; i <= 50; i++ {
		var args []reflect.Type
		for range i {
			args = append(args, intType)
		}
		fn := reflect.MakeFunc(
			reflect.FuncOf(
				args,
				[]reflect.Type{resultType},
				false,
			),
			func(args []reflect.Value) (rets []reflect.Value) {
				for _, arg := range args {
					if arg.Int() != 42 {
						t.Fatal()
					}
				}
				return []reflect.Value{reflect.ValueOf(Result(42))}
			},
		).Interface()
		if base.Fork(fn).Get[Result]() != 42 {
			t.Fatal()
		}
	}
}

type testFuncDef struct{}

type acc2 int

func TestForkManyArgs(t *testing.T) {
	type Result int
	got := New(func() int {
		return 42
	}).Fork(func(
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
		_ int, _ int, _ int, _ int, _ int,
	) Result {
		return 42
	}).Get[Result]()
	if got != 42 {
		t.Fatal(got)
	}
}

func TestResetSameInitializer(t *testing.T) {
	n := 0
	s := New(
		func(_ int) (int8, int16) {
			n++
			return 42, 42
		},
		func() int {
			return 42
		},
	)
	s = s.Fork(func() int {
		return 1
	})
	var i8 int8
	var i16 int16
	s.Assign(&i8)
	s.Assign(&i16)
	if n != 1 {
		t.Fatal()
	}
}

func TestGenericFuncs(t *testing.T) {
	s := New(func() int {
		return 42
	})
	i := s.Get[int]()
	if i != 42 {
		t.Fatal()
	}
	var i2 int
	s.Assign(&i2)
	if i2 != 42 {
		t.Fatal()
	}
}

func TestNilFuncDef(t *testing.T) {
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			if str := fmt.Sprintf("%v", p); !strings.Contains(str, "nil function provided") {
				t.Fatalf("got %v", str)
			}
		}()
		type I int
		New((func() I)(nil))
	}()
}

func TestNilPointerDef(t *testing.T) {
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			if str := fmt.Sprintf("%v", p); !strings.Contains(str, "nil pointer provided") {
				t.Fatalf("got %v", str)
			}
		}()
		type I int
		New((*I)(nil))
	}()
}

func TestGetInterface(t *testing.T) {
	type I any
	scope := New(func() I {
		return 42
	})
	v := scope.Get[I]()
	if v != 42 {
		t.Fatal()
	}
}

func TestNilInterfacePointerDefinition(t *testing.T) {
	var provided any
	scope := New(&provided)
	provided = "changed after scope creation"

	value, ok := scope.get(getTypeID(reflect.TypeFor[any]()))
	if !ok {
		t.Fatal("nil interface definition not found")
	}
	if !value.IsValid() {
		t.Fatal("nil interface resolved to an invalid reflect.Value")
	}
	if !value.IsNil() {
		t.Fatalf("resolved value is not nil: %v", value.Interface())
	}

	if resolved := scope.Get[any](); resolved != nil {
		t.Fatalf("generic Get returned %v, want nil", resolved)
	}

	var assigned any = 42
	scope.Assign(&assigned)
	if assigned != nil {
		t.Fatalf("Assign returned %v, want nil", assigned)
	}
}

func TestGetDependencyNotFound(t *testing.T) {
	scope := New()
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
		scope.Get[int]()
	}()
}

func TestTryGet(t *testing.T) {
	scope := New(
		Provide(42),
		func(i int) string { return strconv.Itoa(i) },
	)

	t.Run("found", func(t *testing.T) {
		v, ok := scope.TryGet[int]()
		if !ok || v != 42 {
			t.Fatalf("got (%v, %v), want (42, true)", v, ok)
		}
	})

	t.Run("found derived", func(t *testing.T) {
		v, ok := scope.TryGet[string]()
		if !ok || v != "42" {
			t.Fatalf("got (%q, %v), want (\"42\", true)", v, ok)
		}
	})

	t.Run("missing", func(t *testing.T) {
		v, ok := scope.TryGet[float64]()
		if ok || v != 0 {
			t.Fatalf("got (%v, %v), want (0, false)", v, ok)
		}
	})

	t.Run("nil interface", func(t *testing.T) {
		var provided any
		s := New(&provided)
		v, ok := s.TryGet[any]()
		if !ok || v != nil {
			t.Fatalf("got (%v, %v), want (nil, true)", v, ok)
		}
	})

	t.Run("builtins", func(t *testing.T) {
		empty := New()
		if _, ok := empty.TryGet[InjectStruct](); !ok {
			t.Fatal("InjectStruct should always be provided")
		}
		if _, ok := empty.TryGet[Fork](); !ok {
			t.Fatal("Fork should always be provided")
		}
		if _, ok := empty.TryGet[Reset](); !ok {
			t.Fatal("Reset should always be provided")
		}
	})
}

func TestGetType(t *testing.T) {
	scope := New(
		Provide(42),
		func(i int) string { return strconv.Itoa(i) },
	)

	t.Run("found", func(t *testing.T) {
		value := scope.GetType(reflect.TypeFor[int]())
		if value.Kind() != reflect.Int || value.Int() != 42 {
			t.Fatalf("got %v, want 42", value)
		}
	})

	t.Run("found derived", func(t *testing.T) {
		value := scope.GetType(reflect.TypeFor[string]())
		if value.String() != "42" {
			t.Fatalf("got %q, want %q", value.String(), "42")
		}
	})

	t.Run("builtins", func(t *testing.T) {
		value := scope.GetType(reflect.TypeFor[Fork]())
		if value.Type() != reflect.TypeFor[Fork]() {
			t.Fatalf("got type %v", value.Type())
		}
	})

	t.Run("missing", func(t *testing.T) {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("panic value not an error: %v", p)
			}
			if !errors.Is(err, ErrDependencyNotFound) {
				t.Fatalf("expected ErrDependencyNotFound, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "no definition for float64") {
				t.Fatalf("unexpected error message: %s", err.Error())
			}
		}()
		scope.GetType(reflect.TypeFor[float64]())
	})

	t.Run("nil type", func(t *testing.T) {
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
			if !strings.Contains(err.Error(), "nil reflect.Type provided") {
				t.Fatalf("unexpected error message: %s", err.Error())
			}
		}()
		scope.GetType(nil)
	})
}

func TestTryGetType(t *testing.T) {
	scope := New(
		Provide(42),
		func(i int) string { return strconv.Itoa(i) },
	)

	t.Run("found", func(t *testing.T) {
		value, ok := scope.TryGetType(reflect.TypeFor[int]())
		if !ok {
			t.Fatal("int not found")
		}
		if value.Int() != 42 {
			t.Fatalf("got %d, want 42", value.Int())
		}
	})

	t.Run("found derived", func(t *testing.T) {
		value, ok := scope.TryGetType(reflect.TypeFor[string]())
		if !ok {
			t.Fatal("string not found")
		}
		if value.String() != "42" {
			t.Fatalf("got %q, want %q", value.String(), "42")
		}
	})

	t.Run("missing", func(t *testing.T) {
		value, ok := scope.TryGetType(reflect.TypeFor[float64]())
		if ok || value.IsValid() {
			t.Fatalf("got (%v, %v), want (zero Value, false)", value, ok)
		}
	})

	t.Run("nil interface", func(t *testing.T) {
		var provided any
		s := New(&provided)
		value, ok := s.TryGetType(reflect.TypeFor[any]())
		if !ok {
			t.Fatal("any not found")
		}
		if !value.IsValid() || !value.IsNil() {
			t.Fatalf("got %v, want a nil interface value", value)
		}
	})

	t.Run("nil type", func(t *testing.T) {
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
		}()
		scope.TryGetType(nil)
	})
}

func TestPointerProviderMutated(t *testing.T) {
	i := 42
	scope := New(&i)
	if scope.Get[int]() != 42 {
		t.Fatal()
	}
	i = 1
	if scope.Get[int]() != 42 {
		t.Fatalf("got %v", scope.Get[int]())
	}
}

func TestTryGetProviderPanicPropagates(t *testing.T) {
	type Foo int
	scope := New(func() Foo { panic("provider panic") })
	defer func() {
		p := recover()
		if p == nil {
			t.Fatal("provider panic should propagate through TryGet")
		}
		if str := fmt.Sprintf("%v", p); str != "provider panic" {
			t.Fatalf("got %v", str)
		}
	}()
	scope.TryGet[Foo]()
}

func TestSharedInstanceProvider(t *testing.T) {
	type Service struct {
		ID int
	}
	// The singleton instance.
	service := &Service{ID: 1}

	// To provide a shared instance (i.e., a singleton pointer),
	// it must be returned from a provider function.
	scope := New(func() *Service {
		return service
	})

	// Two lookups get the service injected.
	s1 := scope.Get[*Service]()
	s2 := scope.Get[*Service]()

	// Both should have received the *exact same instance*.
	if s1 != service {
		t.Fatal("injected service is not the original instance")
	}
	if s2 != service {
		t.Fatal("injected service is not the original instance")
	}
	if s1 != s2 {
		t.Fatal("different instances were injected")
	}

	// Mutating the shared instance should be reflected everywhere.
	s1.ID = 99
	if s2.ID != 99 {
		t.Errorf("mutation on shared instance was not reflected. got %d, want 99", s2.ID)
	}
	if service.ID != 99 {
		t.Errorf("mutation on shared instance was not reflected on original. got %d, want 99", service.ID)
	}
}

func TestForkDoesNotModifyDefs(t *testing.T) {
	type MyModule struct {
		Module
	}

	defs := []any{
		func() int { return 1 },
		new(MyModule),
		func() string { return "a" },
	}

	// Make a copy for comparison after the call
	defsBefore := make([]any, len(defs))
	copy(defsBefore, defs)

	// Call Fork
	New().Fork(defs...)

	if len(defs) != len(defsBefore) {
		t.Fatal()
	}
}

func TestAssignNilPointer(t *testing.T) {
	scope := New(Provide(42))
	var ptr *int = nil
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
				t.Errorf("expected ErrBadArgument, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "cannot assign to a nil pointer target of type *int") {
				t.Errorf("unexpected error message: %s", err.Error())
			}
		}()
		scope.Assign(ptr)
	}()
}

func TestGenericAssignNilPointer(t *testing.T) {
	scope := New(Provide(42))
	var pointer *int

	defer func() {
		panicValue := recover()
		if panicValue == nil {
			t.Fatal("should panic")
		}
		err, ok := panicValue.(error)
		if !ok {
			t.Fatalf("panic value not an error: %v", panicValue)
		}
		if !errors.Is(err, ErrBadArgument) {
			t.Fatalf("expected ErrBadArgument, got %T: %v", err, err)
		}
		if !strings.Contains(err.Error(), "cannot assign to a nil pointer target of type *int") {
			t.Fatalf("unexpected error message: %s", err.Error())
		}
	}()

	scope.Assign(pointer)
}

func TestForkRedefinitionOptimization(t *testing.T) {
	scope := New(func() int {
		return 1
	})
	// Initial state: 1 value (int)
	if n := scope.values.Len(); n != 1 {
		t.Fatalf("expected 1 value, got %d", n)
	}

	// Fork with override
	scope2 := scope.Fork(func() int {
		return 2
	})

	// Expected state:
	// Parent has 1 value.
	// Child appends 1 new value (override).
	// Child SHOULD NOT append a reset value for int, because it's overridden.
	// So total should be 1 (parent) + 1 (new) = 2.
	//
	// With bug:
	// It appends reset value for int.
	// Total = 1 + 1 + 1 = 3.

	if n := scope2.values.Len(); n != 2 {
		t.Errorf("expected 2 values (optimized), got %d. The redefined value is likely being duplicated in reset layer.", n)
	}
}

func TestNewNilDefinition(t *testing.T) {
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("should panic")
			}
			err, ok := p.(error)
			if !ok {
				t.Fatalf("expected error, got %T: %v", p, p)
			}
			if !errors.Is(err, ErrBadArgument) {
				t.Fatalf("expected ErrBadArgument, got %v", err)
			}
			if !strings.Contains(err.Error(), "nil definition") {
				t.Fatalf("unexpected error message: %v", err)
			}
		}()
		New(nil)
	}()
}

func TestStaleInjectStruct(t *testing.T) {
	type Config struct {
		Val int
	}
	type Service struct {
		Cfg Config `dscope:"."`
	}

	// Parent Scope
	scope := New(
		Provide(Config{Val: 1}),
		func(inject InjectStruct) *Service {
			var s Service
			inject(&s)
			return &s
		},
	)

	s := scope.Get[*Service]()
	if s.Cfg.Val != 1 {
		t.Fatalf("expected 1, got %d", s.Cfg.Val)
	}

	// Child Scope with override
	scope2 := scope.Fork(
		Provide(Config{Val: 2}),
	)

	s2 := scope2.Get[*Service]()
	if s2.Cfg.Val != 2 {
		t.Fatalf("expected 2, got %d", s2.Cfg.Val)
	}
}

func TestStaleInjectField(t *testing.T) {
	type Service struct {
		Val Inject[int] `dscope:"."`
	}
	scope := New(
		Provide(int(1)),
		func(inject InjectStruct) *Service {
			var s Service
			inject(&s)
			return &s
		},
	)
	s := scope.Get[*Service]()
	if s.Val() != 1 {
		t.Fatal()
	}

	scope2 := scope.Fork(Provide(int(2)))
	s2 := scope2.Get[*Service]()
	if s2.Val() != 2 {
		t.Fatalf("expected 2, got %d", s2.Val())
	}
}

type loopPathTypeA int
type loopPathTypeB int
type loopPathTypeC int

// TestDependencyLoopPathShowsCycle verifies that the dependency-loop error
// reports a closed path: the first type on the reported path must repeat at
// the end, so the loop is visible in the message.
func TestDependencyLoopPathShowsCycle(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a dependency loop panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("panic is not an error: %v", r)
		}
		message := err.Error()
		const marker = "path: "
		if !strings.Contains(message, marker) {
			t.Fatalf("error message has no path: %s", message)
		}
		path := strings.TrimSpace(message[strings.Index(message, marker)+len(marker):])
		parts := strings.Split(path, " -> ")
		if len(parts) < 3 {
			t.Fatalf("path too short: %q", path)
		}
		if parts[0] != parts[len(parts)-1] {
			t.Fatalf("path does not close the cycle: %q", path)
		}
	}()
	// The cycle loopA -> loopB -> loopC -> loopA spans two Fork layers:
	// a single call cannot express it because loopA would be defined twice.
	base := New(func() loopPathTypeA { return 0 })
	_ = base.Fork(
		func(a loopPathTypeA) loopPathTypeB { return 0 },
		func(b loopPathTypeB) loopPathTypeC { return 0 },
		func(c loopPathTypeC) loopPathTypeA { return 0 },
	)
}

// TestNilDefinitionOnCachedForker verifies that definition validation runs on
// every Fork call, including calls served by a cached _Forker. The forker
// cache is keyed by definition types only, so a cached path must still reject
// nil function and nil pointer definitions at Fork time instead of deferring
// the panic to the first value access.
func TestNilDefinitionOnCachedForker(t *testing.T) {
	t.Run("nil function", func(t *testing.T) {
		New(func() int { return 1 }) // warm the forker cache for this definition type
		var nilFunc func() int
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("nil function definition must panic at Fork time")
			}
			if !errors.Is(p.(error), ErrBadArgument) {
				t.Fatalf("expected ErrBadArgument, got %v", p)
			}
		}()
		New(nilFunc)
	})
	t.Run("nil pointer", func(t *testing.T) {
		New(Provide(1)) // warm the forker cache for *int
		var nilPtr *int
		defer func() {
			p := recover()
			if p == nil {
				t.Fatal("nil pointer definition must panic at Fork time")
			}
			if !errors.Is(p.(error), ErrBadArgument) {
				t.Fatalf("expected ErrBadArgument, got %v", p)
			}
		}()
		New(nilPtr)
	})
}
