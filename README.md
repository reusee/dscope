```markdown
# dscope - Immutable Dependency Injection for Go

`dscope` is a dependency injection container for Go, built with a focus on immutability, lazy initialization, and performance. It leverages Go's type system to provide a type-safe way to manage dependencies within your application.

[![Go Reference](https://pkg.go.dev/badge/github.com/reusee/dscope.svg)](https://pkg.go.dev/github.com/reusee/dscope)
[![Go Report Card](https://goreportcard.com/badge/github.com/reusee/dscope)](https://goreportcard.com/report/github.com/reusee/dscope)

## Key Features

*   **Immutability:** Scopes are immutable. Operations like `Fork` create new scopes, leaving the original unchanged. This makes scopes safe to share across goroutines without locking after creation.
*   **Lazy Initialization:** Provider functions are executed only when the value they provide is first requested within a scope lineage, and the result is cached for subsequent requests within that scope.
*   **Type Safety:** Relies on Go's static typing. Dependencies are resolved based on their types. Generic functions `Get[T]` and `Assign[T]` provide compile-time safety.
*   **Value Overriding:** Child scopes created via `Fork` can provide new definitions for types already present in parent scopes, effectively overriding them for the child and its descendants.
*   **Multiple Value Providers:** Provider functions can return multiple values. Each return value becomes an independently injectable dependency.
*   **Function Calls with Injection:** `scope.Call(fn)` resolves the arguments of `fn` from the scope and executes it.
*   **Value Assignment:** `scope.Assign(&target)` resolves dependencies based on the type of the target pointer and assigns the value.
*   **Pointer Providers:** You can provide concrete values directly by passing pointers (e.g., `New(&myConfig)` makes `MyConfig` available).
*   **Reducers:** A mechanism to combine multiple definitions of the same type into a single value (e.g., merging configuration slices, aggregating handler functions). Supports default behaviors (slices, maps, funcs, ints, strings) and custom reducer logic via the `CustomReducer` interface.
*   **Dependency Cycle Detection:** Automatically detects and panics on circular dependencies during scope creation or value resolution.
*   **Performance Conscious:** Uses internal caching (`_Forker`, type info, argument resolvers), optimized data structures (`_StackedMap` with binary search), and lazy evaluation to minimize overhead. Benchmarks are included (`bench_test.go`).

## Installation

```bash
go get github.com/reusee/dscope
```

## Usage

### Basic Example

Define dependencies using provider functions. `Fork` creates a new scope with these definitions. `Assign` or `Get` retrieves values.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Config struct {
	ConnectionString string
}

type Service struct {
	Config Config
}

func main() {
	// Create a scope with initial definitions
	scope := dscope.New().Fork(

		// Provider for Config
		func() Config {
			fmt.Println("Initializing Config...") // Will be printed only once
			return Config{
				ConnectionString: "localhost:5432",
			}
		},

		// Provider for Service, depends on Config
		func(cfg Config) Service {
			fmt.Println("Initializing Service...") // Will be printed only once
			return Service{
				Config: cfg,
			}
		},
	)

	// Retrieve a Service instance
	var service1 Service
	scope.Assign(&service1)
	fmt.Printf("Service 1 Config: %s\n", service1.Config.ConnectionString)

	// Retrieve another Service instance (uses cached values)
	var service2 Service
	scope.Assign(&service2)
	fmt.Printf("Service 2 Config: %s\n", service2.Config.ConnectionString)

	// Using generic Get
	config := dscope.Get[Config](scope)
	fmt.Printf("Direct Config: %s\n", config.ConnectionString)
}

// Output:
// Initializing Config...
// Initializing Service...
// Service 1 Config: localhost:5432
// Service 2 Config: localhost:5432
// Direct Config: localhost:5432
```

### Overriding Dependencies

Use `Fork` to create a child scope with modified or additional dependencies.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Config string // Simple config type

func main() {
	// Parent scope
	parentScope := dscope.New(func() Config { return "parent_config" })

	var parentConfig Config
	parentScope.Assign(&parentConfig)
	fmt.Printf("Parent scope config: %s\n", parentConfig)

	// Child scope overriding Config
	childScope := parentScope.Fork(func() Config { return "child_config" })

	var childConfig Config
	childScope.Assign(&childConfig)
	fmt.Printf("Child scope config: %s\n", childConfig)

	// Parent scope remains unchanged
	parentScope.Assign(&parentConfig)
	fmt.Printf("Parent scope config (after fork): %s\n", parentConfig)
}

// Output:
// Parent scope config: parent_config
// Child scope config: child_config
// Parent scope config (after fork): parent_config
```

### Calling Functions with Injection

Use `scope.Call` to execute a function, automatically injecting its dependencies.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Foo int
type Bar string

func main() {
	scope := dscope.New(
		func() Foo { return 42 },
		func() Bar { return "hello" },
	)

	// Call a function requiring Foo and Bar
	scope.Call(func(f Foo, b Bar) {
		fmt.Printf("Called with Foo=%d, Bar=%s\n", f, b)
	})

	// Call can also return values
	result := scope.Call(func(f Foo) string {
		return fmt.Sprintf("Foo is %d", f)
	})

	var message string
	result.Assign(&message) // Assign return value by type
	fmt.Println(message)
}

// Output:
// Called with Foo=42, Bar=hello
// Foo is 42
```

### Reducers

Reducers combine multiple definitions of the same type.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
	"reflect"
)

// Define a type that implements dscope.CustomReducer
type ConfigSlice []string

func (c ConfigSlice) Reduce(_ dscope.Scope, vs []reflect.Value) reflect.Value {
	var result ConfigSlice
	for _, v := range vs {
		result = append(result, v.Interface().(ConfigSlice)...)
	}
	return reflect.ValueOf(result)
}

// Alternatively, use a marker interface for default reduction (slices, maps, etc.)
type Handler func()
func (Handler) IsReducer() {} // Implements dscope.Reducer

func main() {
	scope := dscope.New(
		// Multiple providers for ConfigSlice
		func() ConfigSlice { return []string{"a", "b"} },
		func() ConfigSlice { return []string{"c"} },

		// Multiple providers for Handler
		func() Handler { return func() { fmt.Println("Handler 1") } },
		func() Handler { return func() { fmt.Println("Handler 2") } },
	)

	// Retrieve the combined ConfigSlice
	var configs ConfigSlice
	scope.Assign(&configs)
	fmt.Printf("Combined Configs: %v\n", configs) // Output: Combined Configs: [a b c]

	// Retrieve the combined Handler
	var handler Handler
	scope.Assign(&handler)
	handler() // Output: Handler 1 \n Handler 2
}
```

### Pointer Providers

Provide concrete values directly using pointers.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type MyConfig struct {
	Value int
}

func main() {
	config := MyConfig{Value: 123}

	// Provide the config instance via its pointer
	scope := dscope.New(&config)

	// Retrieve it
	retrievedConfig := dscope.Get[MyConfig](scope)
	fmt.Printf("Retrieved Config Value: %d\n", retrievedConfig.Value) // Output: 123

	// Can also retrieve the pointer itself if needed
	ptrConfig := dscope.Get[*MyConfig](scope)
	fmt.Printf("Pointer Config Value: %d\n", ptrConfig.Value)        // Output: 123
	fmt.Printf("Pointers Match: %t\n", &config == ptrConfig)        // Output: true
}
```

## Performance Considerations

*   **Immutability:** Forking creates new `Scope` structs but shares underlying data structures where possible. The `_StackedMap` uses a persistent structure.
*   **Lazy Loading:** Values are only computed when first needed.
*   **Caching:**
    *   Initialized values are cached within a `Scope`.
    *   The process of creating a child scope (`_Forker`) is cached based on the parent signature and new definition types.
    *   Reflection-based information (type IDs, argument resolvers) is cached globally.
*   **Flattening:** To prevent performance degradation from deeply nested scopes, the underlying stack map is occasionally flattened during `Fork`.
*   **Benchmarks:** Run `go test -bench=. ./...` to see performance metrics for various operations.

## Error Handling

`dscope` generally uses panics for errors that occur during scope configuration or dependency resolution. These typically indicate a programming error (like a missing dependency or a dependency cycle) rather than a runtime error.

Common panic types (errors):

*   `ErrDependencyNotFound`: A required type could not be found in the scope or its parents.
*   `ErrDependencyLoop`: A circular dependency was detected.
*   `ErrBadArgument`: An invalid argument was passed to `Assign`, `Call`, etc. (e.g., non-pointer to `Assign`).
*   `ErrBadDefinition`: An invalid provider was used (e.g., a function returning no values, multiple providers for a non-reducer type).

It's recommended to set up scopes during application initialization. If a panic occurs, it will likely happen early, making it easier to diagnose.

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

