package dscope

import (
	"reflect"
	"testing"
)

func TestGetForkValue(t *testing.T) {
	scope := New(func() int {
		return 42
	})

	f := Get[Fork](scope)
	if f == nil {
		t.Fatal("got nil Fork")
	}

	child := f(func() string { return "hello" })
	if Get[int](child) != 42 {
		t.Fatal("child scope did not inherit parent definitions")
	}
	if Get[string](child) != "hello" {
		t.Fatal("child scope did not add new definitions")
	}
}

func TestCallWithForkDependency(t *testing.T) {
	scope := New(func() int {
		return 42
	})

	scope.Call(func(f Fork) {
		if f == nil {
			t.Fatal("Fork dependency was nil")
		}
		child := f(func() string { return "from child" })
		if Get[int](child) != 42 {
			t.Fatal("child did not inherit int")
		}
		if Get[string](child) != "from child" {
			t.Fatal("child did not add string")
		}
	})
}

func TestAssignFork(t *testing.T) {
	scope := New()
	var f Fork
	scope.Assign(&f)
	if f == nil {
		t.Fatal("Assign got nil Fork")
	}
	child := f(func() string { return "assigned" })
	if Get[string](child) != "assigned" {
		t.Fatal("assigned Fork did not create child correctly")
	}
}

func TestForkValueInAllTypes(t *testing.T) {
	scope := New()
	found := false
	for typ := range scope.AllTypes() {
		if typ == reflect.TypeFor[Fork]() {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("Fork not found in AllTypes")
	}
}

func TestForkValueNoDuplicateInAllTypes(t *testing.T) {
	scope := New(func() Fork {
		return func(defs ...any) Scope { return New() }
	})
	var count int
	for typ := range scope.AllTypes() {
		if typ == reflect.TypeFor[Fork]() {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Fork should appear exactly once in AllTypes, got %d", count)
	}
}

func TestForkValueIgnoredDefinition(t *testing.T) {
	// Providing a custom Fork definition should be ignored; the built-in
	// binding to the current scope's Fork method takes precedence.
	scope := New(
		func() int {
			return 42
		},
		func() Fork {
			return func(defs ...any) Scope { return New() }
		},
	)
	f := Get[Fork](scope)
	child := f(func() string { return "hello" })
	// If the user definition were used, child would not have int=42.
	if Get[int](child) != 42 {
		t.Fatal("user-provided Fork definition was used instead of built-in")
	}
}

func TestForkValueReflectGet(t *testing.T) {
	scope := New(func() int {
		return 42
	})
	v, ok := scope.Get(reflect.TypeFor[Fork]())
	if !ok {
		t.Fatal("Get(reflect.Type) returned ok=false")
	}
	f := v.Interface().(Fork)
	if f == nil {
		t.Fatal("got nil Fork from reflect.Value")
	}
	child := f(func() string { return "reflect" })
	if Get[int](child) != 42 {
		t.Fatal("reflect-obtained Fork did not inherit parent")
	}
}

func TestForkDependencyResetForNewDefs(t *testing.T) {
	type Config int
	type Service int
	var counter int
	scope := New(
		func() Config { return 1 },
		func(f Fork) Service {
			counter++
			child := f()
			return Service(Get[Config](child))
		},
	)

	if s := Get[Service](scope); s != 1 {
		t.Fatalf("expected 1, got %d", s)
	}
	if counter != 1 {
		t.Fatalf("expected provider to run once, got %d", counter)
	}

	// A fork without new definitions must not re-evaluate the provider.
	noNewDefs := scope.Fork()
	if s := Get[Service](noNewDefs); s != 1 {
		t.Fatalf("expected 1, got %d", s)
	}
	if counter != 1 {
		t.Fatalf("provider re-evaluated without new definitions, got %d", counter)
	}

	// A fork adding definitions must pessimistically re-evaluate the opaque
	// Fork dependency binding against the new scope.
	child := scope.Fork(func() Config { return 3 })
	if s := Get[Service](child); s != 3 {
		t.Fatalf("expected 3, got %d", s)
	}
	if counter != 2 {
		t.Fatalf("expected provider to run twice, got %d", counter)
	}
}
