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
			if !strings.Contains(err.Error(), "not a pointer") {
				t.Fatal()
			}
		}()
		scope.Assign(42)
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
		scope.Call(func(string) {})
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

func TestCall(t *testing.T) {
	scope := New().Fork(
		func() int {
			return 42
		},
		func() float64 {
			return 42
		},
	)
	res := scope.Call(func(i int, f float64) int {
		return 42 + i + int(f)
	})
	if len(res.Values) != 1 {
		t.Fatalf("bad returns")
	}
	if res.Values[0].Kind() != reflect.Int {
		t.Fatalf("bad return type")
	}
	if res.Values[0].Int() != 126 {
		t.Fatalf("bad return")
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
	scope.Assign(&i, &s)
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

func TestCallReturn(t *testing.T) {
	scope := New().Fork(
		func() int {
			return 42
		},
		func() error {
			return fmt.Errorf("foo")
		},
	)

	var i int
	var err error
	scope.Call(func(
		i int,
		err error,
	) (int, error) {
		return i, err
	}).Assign(&i, &err)
	if i != 42 {
		t.Fatal()
	}
	if err.Error() != "foo" {
		t.Fatal()
	}

	var i2 int
	scope.Call(func(
		i int,
		err error,
	) (int, error) {
		return i, err
	}).Assign(&i2)
	if i != 42 {
		t.Fatal()
	}

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
			if !strings.Contains(err.Error(), "is not a pointer") {
				t.Fatal()
			}
		}()
		scope.Call(func() (int, error) {
			return 42, nil
		}).Assign(42)
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
			msg := err.Error()
			if !strings.Contains(msg, "cannot assign value of type int to target of type string") {
				t.Fatalf("got %s", msg)
			}
		}()
		var s string
		scope.Call(func() (int, error) {
			return 42, nil
		}).Extract(&s)
	}()

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
	scope := New(
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
	s2.Assign(&i, &s)
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

	if Get[int](scope) != 42 {
		t.Fatal()
	}
	if Get[string](scope) != "42" {
		t.Fatal()
	}
	if Get[int](scope2) != 24 {
		t.Fatal()
	}
	if Get[string](scope2) != "foo" {
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
	scope.Call(func(
		i int,
	) {
		if i != 42 {
			t.Fatal()
		}
	})

	scope = New(
		func(f float64) int {
			return int(f)
		},
		&i,
	)
	scope.Call(func(
		i int,
	) {
		if i != 42 {
			t.Fatal()
		}
	})

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
	scope.Call(func(
		i int,
	) {
		if i != 24 {
			t.Fatal()
		}
	})

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
	scope.Call(func(
		i int,
	) {
		if i != 24 {
			t.Fatal()
		}
	})

	scope = New(func() int {
		return 42
	}).Fork(func() *int {
		i := 42
		return &i
	}())
	scope.Call(func(
		i int,
	) {
		if i != 42 {
			t.Fatal()
		}
	})

}

func TestRacyCall(t *testing.T) {
	s := New(func() int {
		return 42
	})
	for range 512 {
		go func() {
			s.Call(func(i int) {
			})
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

func TestPcallValueArgs(t *testing.T) {
	scope := New(func() int {
		return 42
	})
	intType := reflect.TypeFor[int]()
	for i := 0; i <= 50; i++ {
		var args []reflect.Type
		for range i {
			args = append(args, intType)
		}
		fn := reflect.MakeFunc(
			reflect.FuncOf(
				args,
				[]reflect.Type{},
				false,
			),
			func(args []reflect.Value) (rets []reflect.Value) {
				for _, arg := range args {
					if arg.Int() != 42 {
						t.Fatal()
					}
				}
				return
			},
		).Interface()
		scope.Call(fn)
	}
}

type testFuncDef struct{}

func TestCallResultFork(t *testing.T) {
	var i int
	New(func() int {
		return 42
	}).Call(func(i int) int {
		return i * 2
	}).Extract(&i)
	if i != 84 {
		t.Fatal()
	}
}

type acc2 int

func TestCallManyArgs(t *testing.T) {
	New(func() int {
		return 42
	}).Call(func(
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
	) {
	})
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
	i := Get[int](s)
	if i != 42 {
		t.Fatal()
	}
	var i2 int
	Assign(s, &i2)
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
	v := Get[I](scope)
	if v != 42 {
		t.Fatal()
	}
}

func TestCallResultAssignDuplicateReturns(t *testing.T) {
	scope := New()

	var i1, i2 int
	scope.Call(func() (int, string, int) {
		return 1, "foo", 2
	}).Assign(&i1, &i2)

	if i1 != 1 || i2 != 2 {
		t.Fatalf("Expected i1=1, i2=2, got i1=%d, i2=%d", i1, i2)
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
		Get[int](scope)
	}()
}

func TestPointerProviderMutated(t *testing.T) {
	i := 42
	scope := New(&i)
	scope.Call(func(
		i int,
	) {
		if i != 42 {
			t.Fatal()
		}
	})
	i = 1
	scope.Call(func(
		i int,
	) {
		if i != 42 {
			t.Fatalf("got %v", i)
		}
	})
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

	// Two different functions get the service injected.
	var s1, s2 *Service
	scope.Call(func(s *Service) {
		s1 = s
	})
	scope.Call(func(s *Service) {
		s2 = s
	})

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
