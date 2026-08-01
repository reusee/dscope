package dscope

import (
	"reflect"
	"sync/atomic"
	"testing"
)

func TestResetRecomputesValues(t *testing.T) {
	var counter int64
	scope := New(func() int {
		return int(atomic.AddInt64(&counter, 1))
	})

	if v := Get[int](scope); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	if v := Get[int](scope); v != 1 {
		t.Fatalf("expected cached 1, got %d", v)
	}

	r := scope.Reset()
	if v := Get[int](r); v != 2 {
		t.Fatalf("expected 2 after reset, got %d", v)
	}
	if v := Get[int](r); v != 2 {
		t.Fatalf("expected cached 2, got %d", v)
	}
	if v := Get[int](scope); v != 1 {
		t.Fatalf("original affected: expected 1, got %d", v)
	}
}

func TestResetLazy(t *testing.T) {
	var fooCounter, barCounter int64
	scope := New(
		func() int {
			return int(atomic.AddInt64(&fooCounter, 1))
		},
		func() string {
			_ = atomic.AddInt64(&barCounter, 1)
			return "bar"
		},
	)

	_ = Get[int](scope)
	if fooCounter != 1 || barCounter != 0 {
		t.Fatalf("foo=%d, bar=%d", fooCounter, barCounter)
	}

	r := scope.Reset()
	_ = Get[int](r)
	if fooCounter != 2 {
		t.Fatalf("expected foo 2, got %d", fooCounter)
	}
	if barCounter != 0 {
		t.Fatalf("bar should not be evaluated: %d", barCounter)
	}

	_ = Get[string](r)
	if barCounter != 1 {
		t.Fatalf("expected bar 1, got %d", barCounter)
	}
}

func TestResetChain(t *testing.T) {
	var counter int64
	scope := New(func() int {
		return int(atomic.AddInt64(&counter, 1))
	})

	_ = Get[int](scope) // 1
	r1 := scope.Reset()
	_ = Get[int](r1) // 2
	r2 := r1.Reset()
	_ = Get[int](r2) // 3

	if v := Get[int](scope); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	if v := Get[int](r1); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
	if v := Get[int](r2); v != 3 {
		t.Fatalf("expected 3, got %d", v)
	}
}

func TestResetFork(t *testing.T) {
	var counter int64
	scope := New(func() int {
		return int(atomic.AddInt64(&counter, 1))
	})
	_ = Get[int](scope) // 1

	r := scope.Reset()
	child := r.Fork(func() string {
		return "hello"
	})

	if v := Get[int](child); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
	if v := Get[string](child); v != "hello" {
		t.Fatalf("expected hello, got %s", v)
	}
	if v := Get[int](scope); v != 1 {
		t.Fatalf("original affected: expected 1, got %d", v)
	}
}

func TestResetDependencyChain(t *testing.T) {
	var intCounter, stringCounter int64
	scope := New(
		func() int {
			return int(atomic.AddInt64(&intCounter, 1))
		},
		func(i int) string {
			c := atomic.AddInt64(&stringCounter, 1)
			return string(rune('A'-1+c)) + string(rune('0'+i))
		},
	)

	if s := Get[string](scope); s != "A1" {
		t.Fatalf("expected A1, got %s", s)
	}

	r := scope.Reset()
	if s := Get[string](r); s != "B2" {
		t.Fatalf("expected B2, got %s", s)
	}
	if intCounter != 2 {
		t.Fatalf("expected intCounter 2, got %d", intCounter)
	}
	if stringCounter != 2 {
		t.Fatalf("expected stringCounter 2, got %d", stringCounter)
	}
}

func TestResetPointerProvider(t *testing.T) {
	val := 42
	scope := New(&val)
	r := scope.Reset()
	if v := Get[int](r); v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
}

func TestResetAssignAndCall(t *testing.T) {
	var counter int64
	scope := New(func() int {
		return int(atomic.AddInt64(&counter, 1))
	})

	var v int
	scope.Assign(&v)

	r := scope.Reset()
	r.Assign(&v)
	if v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}

	r.Call(func(i int) {
		if i != 2 {
			t.Fatalf("expected 2 from Call, got %d", i)
		}
	})
}

func TestResetEmptyScope(t *testing.T) {
	r := New().Reset()
	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("should panic")
			}
		}()
		Get[int](r)
	}()
}

func TestResetAllTypes(t *testing.T) {
	scope := New(
		func() int { return 42 },
		func() string { return "hello" },
	)
	r := scope.Reset()

	types := make(map[reflect.Type]bool)
	for typ := range r.AllTypes() {
		types[typ] = true
	}
	if !types[reflect.TypeFor[int]()] {
		t.Fatal("int not found in reset scope AllTypes")
	}
	if !types[reflect.TypeFor[string]()] {
		t.Fatal("string not found in reset scope AllTypes")
	}
}

func BenchmarkReset(b *testing.B) {
	scope := New().Fork(assignBenchDefs...)
	for b.Loop() {
		_ = scope.Reset()
	}
}

func BenchmarkResetAccess(b *testing.B) {
	scope := New().Fork(assignBenchDefs...)
	r := scope.Reset()
	var t30 T30
	for b.Loop() {
		r.Assign(&t30)
	}
}
