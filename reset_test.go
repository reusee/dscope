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

	if v := scope.Get[int](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	if v := scope.Get[int](); v != 1 {
		t.Fatalf("expected cached 1, got %d", v)
	}

	r := scope.Reset()
	if v := r.Get[int](); v != 2 {
		t.Fatalf("expected 2 after reset, got %d", v)
	}
	if v := r.Get[int](); v != 2 {
		t.Fatalf("expected cached 2, got %d", v)
	}
	if v := scope.Get[int](); v != 1 {
		t.Fatalf("original affected: expected 1, got %d", v)
	}

	// Assign from a reset scope resolves through the same fresh initializers.
	var assigned int
	r.Assign(&assigned)
	if assigned != 2 {
		t.Fatalf("expected 2 from Assign, got %d", assigned)
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

	_ = scope.Get[int]()
	if fooCounter != 1 || barCounter != 0 {
		t.Fatalf("foo=%d, bar=%d", fooCounter, barCounter)
	}

	r := scope.Reset()
	_ = r.Get[int]()
	if fooCounter != 2 {
		t.Fatalf("expected foo 2, got %d", fooCounter)
	}
	if barCounter != 0 {
		t.Fatalf("bar should not be evaluated: %d", barCounter)
	}

	_ = r.Get[string]()
	if barCounter != 1 {
		t.Fatalf("expected bar 1, got %d", barCounter)
	}
}

func TestResetChain(t *testing.T) {
	var counter int64
	scope := New(func() int {
		return int(atomic.AddInt64(&counter, 1))
	})

	_ = scope.Get[int]() // 1
	r1 := scope.Reset()
	_ = r1.Get[int]() // 2
	r2 := r1.Reset()
	_ = r2.Get[int]() // 3

	if v := scope.Get[int](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}
	if v := r1.Get[int](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
	if v := r2.Get[int](); v != 3 {
		t.Fatalf("expected 3, got %d", v)
	}
}

func TestResetFork(t *testing.T) {
	var counter int64
	scope := New(func() int {
		return int(atomic.AddInt64(&counter, 1))
	})
	_ = scope.Get[int]() // 1

	r := scope.Reset()
	child := r.Fork(func() string {
		return "hello"
	})

	if v := child.Get[int](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
	if v := child.Get[string](); v != "hello" {
		t.Fatalf("expected hello, got %s", v)
	}
	if v := scope.Get[int](); v != 1 {
		t.Fatalf("original affected: expected 1, got %d", v)
	}
}

// TestForkResetLayerIsLazy verifies that the reset layer a Fork installs keeps
// the inherited initializers until a value is accessed: the fresh initializer
// of a dependent comes into existence only when the new scope resolves it.
func TestForkResetLayerIsLazy(t *testing.T) {
	type (
		Config  int
		Service int
	)
	scope := New(
		func() Config { return 1 },
		func(c Config) Service { return Service(c) },
	)
	child := scope.Fork(func() Config { return 2 })

	layer := child.values
	if layer == nil || layer.reset == nil || layer.reset.base != nil {
		t.Fatal("fork did not install a partial reset layer")
	}
	count := func() int {
		cache := layer.reset.cache.Load()
		if cache == nil {
			return 0
		}
		n := 0
		cache.Range(func(_, _ any) bool {
			n++
			return true
		})
		return n
	}
	if n := count(); n != 0 {
		t.Fatalf("reset layer holds %d initializers before any access", n)
	}

	if v := child.Get[Service](); v != 2 {
		t.Fatalf("expected 2, got %d", v)
	}
	if n := count(); n != 1 {
		t.Fatalf("reset layer holds %d initializers after one access", n)
	}
}

// TestRefreshKeepsOutputsApart verifies that a scope which hands out fresh
// values answers with the output that was asked for. The outputs of one
// definition share one provider, so a reset layer keeps them apart by the
// requested output and not by the fresh provider it shares.
func TestRefreshKeepsOutputsApart(t *testing.T) {
	type (
		Config int
		A      int
		B      int
	)
	scope := New(
		func() Config { return 1 },
		func(c Config) (A, B) { return A(c), B(c * 2) },
	)

	reset := scope.Reset()
	if v := reset.Get[A](); v != 1 {
		t.Fatalf("expected A 1, got %d", v)
	}
	if v := reset.Get[B](); v != 2 {
		t.Fatalf("expected B 2, got %d", v)
	}

	child := scope.Fork(func() Config { return 3 })
	if v := child.Get[A](); v != 3 {
		t.Fatalf("expected A 3, got %d", v)
	}
	if v := child.Get[B](); v != 6 {
		t.Fatalf("expected B 6, got %d", v)
	}
}

// TestResetLayerKeepsMappingsApart verifies that a reset layer hands out one
// fresh value for each inherited initializer, even when a program resolves
// several refreshed types in turn: a type must never receive the fresh value of
// another type.
func TestResetLayerKeepsMappingsApart(t *testing.T) {
	type (
		Config int
		A      int
		B      int
	)
	scope := New(
		func() Config { return 2 },
		func(c Config) A { return A(c) },
		func(c Config) B { return B(c * 3) },
	)
	reset := scope.Reset()

	for range 4 {
		if v := reset.Get[A](); v != 2 {
			t.Fatalf("expected A 2, got %d", v)
		}
		if v := reset.Get[B](); v != 6 {
			t.Fatalf("expected B 6, got %d", v)
		}
	}

	// The layer holds one fresh value for each refreshed type, and no more.
	cache := reset.values.reset.cache.Load()
	if cache == nil {
		t.Fatal("reset layer created no cache")
	}
	n := 0
	cache.Range(func(_, _ any) bool {
		n++
		return true
	})
	if n != 3 {
		t.Fatalf("reset layer holds %d fresh values, want 3", n)
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

	if s := scope.Get[string](); s != "A1" {
		t.Fatalf("expected A1, got %s", s)
	}

	r := scope.Reset()
	if s := r.Get[string](); s != "B2" {
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
	if v := r.Get[int](); v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
}

func TestResetEmptyScope(t *testing.T) {
	r := New().Reset()
	func() {
		defer func() {
			if p := recover(); p == nil {
				t.Fatal("should panic")
			}
		}()
		r.Get[int]()
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
