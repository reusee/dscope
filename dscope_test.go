package dscope

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
			if !is(err, ErrBadArgument) {
				t.Fatal()
			}
			var argInfo ArgInfo
			if !as(err, &argInfo) {
				t.Fatal()
			}
			if reflect.TypeOf(argInfo.Value) != reflect.TypeOf((*func(int))(nil)).Elem() {
				t.Fatal()
			}
			var reason Reason
			if !as(err, &reason) {
				t.Fatal()
			}
			if reason != "function returns nothing" {
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
			if !is(err, ErrBadArgument) {
				t.Fatal()
			}
			var argInfo ArgInfo
			if !as(err, &argInfo) {
				t.Fatal()
			}
			if reflect.TypeOf(argInfo.Value) != reflect.TypeOf((*int)(nil)).Elem() {
				t.Fatal()
			}
			var reason Reason
			if !as(err, &reason) {
				t.Fatal()
			}
			if reason != "not a function or a pointer" {
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
			if !is(err, ErrBadArgument) {
				t.Fatal()
			}
			var argInfo ArgInfo
			if !as(err, &argInfo) {
				t.Fatal()
			}
			if reflect.TypeOf(argInfo.Value) != reflect.TypeOf((*int)(nil)).Elem() {
				t.Fatal()
			}
			var reason Reason
			if !as(err, &reason) {
				t.Fatal()
			}
			if reason != "must be a pointer" {
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
			if !is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
			var typeInfo TypeInfo
			if !as(err, &typeInfo) {
				t.Fatal()
			}
			if typeInfo.Type != reflect.TypeOf((*string)(nil)).Elem() {
				t.Fatal()
			}
			if typeInfo.Error() != "type: string" {
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
			if !is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
			var typeInfo TypeInfo
			if !as(err, &typeInfo) {
				t.Fatal()
			}
			if typeInfo.Type != reflect.TypeOf((*acc)(nil)).Elem() {
				t.Fatal()
			}
			if typeInfo.Error() != "type: dscope.acc" {
				t.Fatal()
			}
		}()
		var a acc
		scope.Assign(&a)
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
			if !is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
			var typeInfo TypeInfo
			if !as(err, &typeInfo) {
				t.Fatal()
			}
			if typeInfo.Type != reflect.TypeOf((*string)(nil)).Elem() {
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
			if !is(err, ErrDependencyNotFound) {
				t.Fatal()
			}
			var typeInfo TypeInfo
			if !as(err, &typeInfo) {
				t.Fatal()
			}
			if typeInfo.Type != reflect.TypeOf((*string)(nil)).Elem() {
				t.Fatal()
			}
			var initInfo InitInfo
			if !as(err, &initInfo) {
				t.Fatal()
			}
			if reflect.TypeOf(initInfo.Value) !=
				reflect.TypeOf((*func(string) int32)(nil)).Elem() {
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
			if !is(err, ErrDependencyLoop) {
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
			if !is(err, ErrBadDeclaration) {
				t.Fatal()
			}
			var typeInfo TypeInfo
			if !as(err, &typeInfo) {
				t.Fatal()
			}
			if typeInfo.Type != reflect.TypeOf((*int)(nil)).Elem() {
				t.Fatal()
			}
			var reason Reason
			if !as(err, &reason) {
				t.Fatal()
			}
			if reason != "non-reducer type has multiple declarations" {
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
		func(scope Scope) Foo {
			var bar Bar
			var baz Baz
			scope.Assign(&bar)
			scope.Assign(&baz)
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
	type Setscope func(Scope)
	var scope Scope
	scope = New().Fork(
		func() Setscope {
			return func(c Scope) {
				scope = c
			}
		},
	)

	n := 0
	type Foo int
	scope = scope.Fork(
		func(scope Scope, setscope Setscope) Foo {
			n++
			setscope(scope.Fork(
				func() Foo {
					return 44
				},
			))
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
	for i := 0; i < n; i++ {
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
	for i := 0; i < n; i++ {
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
	for i := 0; i < n; i++ {
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

func TestDeclareInterface(t *testing.T) {
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
			if !is(err, ErrBadArgument) {
				t.Fatal()
			}
			var argInfo ArgInfo
			if !as(err, &argInfo) {
				t.Fatal()
			}
			if reflect.TypeOf(argInfo.Value) != reflect.TypeOf((*int)(nil)).Elem() {
				t.Fatal()
			}
			if s := argInfo.Error(); s != "arg: int" {
				t.Fatalf("got %s", s)
			}
			var reason Reason
			if !as(err, &reason) {
				t.Fatal()
			}
			if reason != "must be a pointer" {
				t.Fatal()
			}
			if s := reason.Error(); s != "reason: must be a pointer" {
				t.Fatalf("got %s", s)
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
			if !is(err, ErrBadArgument) {
				t.Fatal()
			}
			var argInfo ArgInfo
			if !as(err, &argInfo) {
				t.Fatal()
			}
			if reflect.TypeOf(argInfo.Value) != reflect.TypeOf((**string)(nil)).Elem() {
				t.Fatal()
			}
			if s := argInfo.Error(); s != "arg: *string" {
				t.Fatalf("got %s", s)
			}
			var reason Reason
			if !as(err, &reason) {
				t.Fatal()
			}
			if reason != "must be pointer to string" {
				t.Fatal()
			}
			if s := reason.Error(); s != "reason: must be pointer to string" {
				t.Fatalf("got %s", s)
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
			reflect.TypeOf((*int)(nil)).Elem(),
			reflect.TypeOf((*string)(nil)).Elem(),
		},
		[]reflect.Type{
			reflect.TypeOf((*S)(nil)).Elem(),
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
	for i := 0; i < n; i++ {
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

func TestAssignScope(t *testing.T) {
	scope := New()
	var scope2 Scope
	scope.Assign(&scope2)
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

func TestIsZero(t *testing.T) {
	if New().IsZero() {
		t.Fatal()
	}
	var s Scope
	if !s.IsZero() {
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
	scope = scope.Fork(
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
	for i := 0; i < 128; i++ {
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
	scope = scope.Fork(
		func() int {
			return 24
		},
		func(i int) string {
			return "foo"
		},
	)
	var s string
	scope.Assign(&s)
	if s != "foo" {
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
	for i := 0; i < 512; i++ {
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

	s2 = s2.Fork(func() I {
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
	intType := reflect.TypeOf((*int)(nil)).Elem()
	for i := 0; i <= 50; i++ {
		var args []reflect.Type
		for j := 0; j < i; j++ {
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

type acc int

var _ Reducer = acc(0)

func (a acc) Reduce(_ Scope, vs []reflect.Value) reflect.Value {
	var ret acc
	for _, v := range vs {
		ret += v.Interface().(acc)
	}
	return reflect.ValueOf(ret)
}

func TestReducer(t *testing.T) {
	s := New(
		func() acc {
			return 1
		},
	)
	var a acc
	s.Assign(&a)
	if a != 1 {
		t.Fatal()
	}

	s = s.Fork(
		func() acc {
			return 2
		},
		func() string {
			return "foo"
		},
		func() acc {
			return 3
		},
	)
	s.Assign(&a)
	if a != 5 {
		t.Fatal()
	}

	s = s.Fork(
		func() acc {
			return 3
		},
		func() string {
			return "foo"
		},
		func() acc {
			return 4
		},
	)
	s.Assign(&a)
	if a != 7 {
		t.Fatal()
	}

}

type testFunc func(*int)

var _ Reducer = testFunc(nil)

func (t testFunc) Reduce(_ Scope, vs []reflect.Value) reflect.Value {
	return Reduce(vs)
}

type testFuncDef struct{}

func (_ testFuncDef) Foo() testFunc {
	return func(p *int) {
		*p++
	}
}

func (_ testFuncDef) Bar() testFunc {
	return func(p *int) {
		*p += 2
	}
}

func TestMultipleDispatch(t *testing.T) {
	s := New(Methods(new(testFuncDef))...)
	var i int
	s.Call(func(
		fn testFunc,
	) {
		fn(&i)
	})
	if i != 3 {
		t.Fatal()
	}
}

func TestRuntimeLoop(t *testing.T) {
	s := New(
		func(s Scope) int {
			var err error
			defer func() {
				if err == nil {
					t.Fatal()
				}
				if !is(err, ErrDependencyLoop) {
					t.Fatal()
				}
			}()
			defer he(&err)
			var i int
			s.Assign(&i)
			return 42
		},
	)
	done := make(chan struct{})
	go func() {
		defer func() {
			close(done)
		}()
		var i int
		s.Assign(&i)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal()
	}
}

type extendAcc int

func (_ extendAcc) Reduce(_ Scope, vs []reflect.Value) reflect.Value {
	return Reduce(vs)
}

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

func TestCallResultAssign(t *testing.T) {
	var i int
	New(func() int {
		return 42
	}).Call(func(i int) (int, int) {
		return 42, 1
	}).Assign(nil, &i)
	if i != 1 {
		t.Fatal()
	}
}

type noShadowInt int

var _ NoShadow = noShadowInt(42)

func (_ noShadowInt) IsNoShadow() {}

func TestNoShadow(t *testing.T) {
	i := noShadowInt(42)
	s := New(&i)

	check := func(fn func()) {
		func() {
			var err error
			defer func() {
				if err == nil {
					t.Fatal("should error")
				}
				if !is(err, ErrBadShadow) {
					t.Fatal()
				}
				var typeInfo TypeInfo
				if !as(err, &typeInfo) {
					t.Fatal()
				}
				if typeInfo.Type != reflect.TypeOf((*noShadowInt)(nil)).Elem() {
					t.Fatal()
				}
				var argInfo ArgInfo
				if !as(err, &argInfo) {
					t.Fatal()
				}
				var reason Reason
				if !as(err, &reason) {
					t.Fatal()
				}
			}()
			defer he(&err)
			fn()
		}()
	}

	check(func() {
		s.Fork(&i)
	})

	check(func() {
		s.Fork(func() noShadowInt {
			return 42
		})
	})

}

func TestScopeAsDependency(t *testing.T) {
	s := New(
		func(
			scope DependentScope,
		) int64 {
			var i int32
			scope.Assign(&i)
			return int64(i)
		},
		func() int32 {
			return 42
		},
	)
	var i64 int64
	s.Assign(&i64)
	if i64 != 42 {
		t.Fatal()
	}
	s = s.Fork(func() int32 {
		return 2
	})
	s.Assign(&i64)
	if i64 != 2 {
		t.Fatal()
	}
}

type acc2 int

var _ Reducer = acc2(0)

func (_ acc2) Reduce(scope Scope, vs []reflect.Value) reflect.Value {
	return Reduce(vs)
}

func TestReducerRunOnce(t *testing.T) {
	n := 0
	m := 0
	scope := New(func() acc {
		n++
		return 1
	}, func() acc {
		return 2
	}, func() acc2 {
		m++
		return 1
	}, func() acc2 {
		return 2
	})

	var a acc
	scope.Assign(&a)
	if n != 1 {
		t.Fatal()
	}
	scope.Assign(&a)
	if n != 1 {
		t.Fatal()
	}
	if a != 3 {
		t.Fatal()
	}

	var b acc
	scope.Assign(&b)
	if n != 1 {
		t.Fatal()
	}

	var a2 acc2
	scope.Assign(&a2)
	if a2 != 3 {
		t.Fatal()
	}
	if m != 1 {
		t.Fatal()
	}

	scope = scope.Fork(func() acc {
		return 42
	})
	scope.Assign(&a)
	if a != 42 {
		t.Fatal()
	}

	scope.Assign(&a2)
	if a2 != 3 {
		t.Fatal()
	}
	if m != 1 {
		t.Fatal()
	}

}

func TestReducerIndirectUpdate(t *testing.T) {
	scope := New(func() acc {
		return 1
	}, func(a acc) acc2 {
		return acc2(a)
	})
	var a2 acc2
	scope.Assign(&a2)
	if a2 != 1 {
		t.Fatal()
	}

	scope = scope.Fork(func() acc {
		return 2
	})
	scope.Assign(&a2)
	if a2 != 2 {
		t.Fatal()
	}

}

func TestCallManyArgs(t *testing.T) {
	New(func() int {
		return 42
	}).Call(func(
		_ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int, _ int,
	) {
	})
}

func TestProxy(t *testing.T) {
	s := New(func() int {
		return 42
	})
	s.SetProxy(func(
		i int,
	) int {
		return i * 2
	})
	var i int
	s.Assign(&i)
	if i != 84 {
		t.Fatalf("got %d\n", i)
	}

	s = s.Fork(func() string {
		return "foo"
	})
	s.SetProxy(func(
		i int,
		s string,
	) string {
		return fmt.Sprintf("%s-%d", s, i)
	})
	var str string
	s.Assign(&str)
	if str != "foo-84" {
		t.Fatal()
	}
}

func TestProxy2(t *testing.T) {
	type Foo func(fn any)
	type Bar func(int) int
	s := New(
		func(
			scope Scope,
		) Foo {
			return func(fn any) {
				scope.Fork(
					func() int {
						return 42
					},
				).Call(fn)
			}
		},
		func() Bar {
			return func(i int) int {
				return i + 1
			}
		},
	)
	s.SetProxy(func(
		foo Foo,
	) Bar {
		return func(i int) (ret int) {
			foo(func(
				bar Bar,
			) {
				ret = bar(i) * 2
			})
			return
		}
	})
	var b Bar
	s.Assign(&b)
	if b(42) != 86 {
		t.Fatal()
	}
}

func TestFillStruct(t *testing.T) {
	s := New(
		func() int {
			return 42
		},
		func() string {
			return "foo"
		},
	)
	var foo struct {
		S string
		I int
	}
	s.FillStruct(&foo)
	if foo.I != 42 {
		t.Fatal()
	}
	if foo.S != "foo" {
		t.Fatal()
	}
}
