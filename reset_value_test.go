package dscope

import (
	"testing"
)

func TestGetResetValue(t *testing.T) {
	value := 0
	scope := New(func() int {
		value++
		return value
	})

	if v := scope.Get[int](); v != 1 {
		t.Fatalf("expected 1, got %d", v)
	}

	r := scope.Get[Reset]()
	if r == nil {
		t.Fatal("got nil Reset")
	}

	resetScope := r()
	if v := resetScope.Get[int](); v != 2 {
		t.Fatalf("expected 2 from reset scope, got %d", v)
	}

	// The original scope is unaffected by the reset scope.
	if v := scope.Get[int](); v != 1 {
		t.Fatalf("expected 1 from original scope, got %d", v)
	}

	// Assign reaches the same built-in binding.
	var assigned Reset
	scope.Assign(&assigned)
	if assigned == nil {
		t.Fatal("Assign got nil Reset")
	}
	if v := assigned().Get[int](); v != 3 {
		t.Fatalf("expected 3 from assigned reset scope, got %d", v)
	}
}

func TestResetValueIgnoredDefinition(t *testing.T) {
	// Providing a custom Reset definition should be ignored; the built-in
	// binding to the current scope's Reset method takes precedence.
	scope := New(
		func() int {
			return 42
		},
		func() Reset {
			return func() Scope { return New() }
		},
	)
	r := scope.Get[Reset]()
	resetScope := r()
	// If the user definition were used, resetScope would be New() without int.
	if resetScope.Get[int]() != 42 {
		t.Fatal("user-provided Reset definition was used instead of built-in")
	}
}

func TestResetDependencyResetForNewDefs(t *testing.T) {
	type Config int
	type Service int
	var counter int
	scope := New(
		func() Config { return 1 },
		func(r Reset) Service {
			counter++
			return Service(r().Get[Config]())
		},
	)

	if s := scope.Get[Service](); s != 1 {
		t.Fatalf("expected 1, got %d", s)
	}
	if counter != 1 {
		t.Fatalf("expected provider to run once, got %d", counter)
	}

	// A fork without new definitions must not re-evaluate the provider.
	noNewDefs := scope.Fork()
	if s := noNewDefs.Get[Service](); s != 1 {
		t.Fatalf("expected 1, got %d", s)
	}
	if counter != 1 {
		t.Fatalf("provider re-evaluated without new definitions, got %d", counter)
	}

	// A fork adding definitions must pessimistically re-evaluate the opaque
	// Reset dependency binding against the new scope.
	child := scope.Fork(func() Config { return 3 })
	if s := child.Get[Service](); s != 3 {
		t.Fatalf("expected 3, got %d", s)
	}
	if counter != 2 {
		t.Fatalf("expected provider to run twice, got %d", counter)
	}
}
