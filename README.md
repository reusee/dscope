# Getting Started with `dscope`

`dscope` is a dependency injection library for Go designed with immutability and lazy initialization in mind. It helps manage dependencies between different parts of your application in a structured and testable way.

## Installation

To use `dscope` in your project, you can install it using the standard `go get` command:

```bash
go get github.com/reusee/dscope
```

*(Note: The exact import path might differ based on the actual repository location. The path `github.com/reusee/dscope` is inferred from the context provided.)*

## Core Concepts

1.  **Scope:** The central concept in `dscope` is the `Scope`. It acts as an immutable container holding definitions for various types. You retrieve dependencies *from* a scope.
2.  **Definitions:** These are instructions on *how* to provide a value of a certain type. `dscope` primarily supports two kinds of definitions:
    *   **Provider Functions:** Regular Go functions where the arguments are dependencies (resolved from the scope) and the return values are the types being provided.
    *   **Pointer Values:** Providing a pointer to an existing value (e.g., `&myConfig`) makes the *value itself* available in the scope.
3.  **Immutability & Forking:** Scopes are immutable. To add or override definitions, you `Fork` an existing scope. This creates a *new* child scope layered on top of the parent, without modifying the original. This makes scope management predictable.
4.  **Lazy Initialization:** Provider functions are not executed until a value they provide (or a value that depends on them) is actually requested from the scope (via `Get`, `Assign`, or `Call`). The result is then cached within that scope instance for subsequent requests.

## Basic Usage

### 1. Creating a Scope (`New`)

You start by creating a root scope using `dscope.New()`, passing your initial definitions.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

// Define some types
type Port int
type Host string
type DatabaseURL string

func main() {
	// Create a new scope with initial definitions
	scope := dscope.New(
		// Provider function for Port
		func() Port {
			fmt.Println("Providing Port...")
			return 8080
		},

		// Provider function for Host (depends on Port)
		func(p Port) Host {
			fmt.Println("Providing Host...")
			return Host(fmt.Sprintf("localhost:%d", p))
		},

		// Provide a configuration value directly using a pointer
		// This makes the DatabaseURL value available in the scope.
		&DatabaseURL{"postgres://user:pass@db:5432/mydb"},
	)

	fmt.Println("Scope created.")

	// Dependencies are not resolved yet because nothing has been requested.
}
```

### 2. Retrieving Values (`Get` and `Assign`)

You can retrieve values from the scope using type-safe generics (`Get[T]`) or populate variables using `Assign`.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Port int
type Host string
type DatabaseURL string

func main() {
	scope := dscope.New(
		func() Port {
			fmt.Println("Providing Port...")
			return 8080
		},
		func(p Port) Host {
			fmt.Println("Providing Host...")
			return Host(fmt.Sprintf("localhost:%d", p))
		},
		&DatabaseURL{"postgres://user:pass@db:5432/mydb"},
	)
	fmt.Println("Scope created.")

	// --- Retrieve using Get[T] ---
	fmt.Println("Requesting Host...")
	// Requesting Host requires Port, so both provider functions will run now.
	host := dscope.Get[Host](scope)
	fmt.Printf("Retrieved Host: %s\n", host)

	// Requesting Host again uses the cached value
	fmt.Println("Requesting Host again...")
	host2 := dscope.Get[Host](scope)
	fmt.Printf("Retrieved Host again: %s\n", host2)

	// --- Retrieve using Assign ---
	fmt.Println("Assigning variables...")
	var dbURL DatabaseURL
	var port Port
	// Assign resolves dependencies as needed (Port is already cached)
	scope.Assign(&dbURL, &port)
	fmt.Printf("Assigned dbURL: %s, port: %d\n", dbURL, port)

	// --- Panic on Not Found ---
	// Uncommenting this will panic because float64 is not defined.
	// f := dscope.Get[float64](scope)
	// fmt.Println(f)
}

/* Example Output:
Scope created.
Requesting Host...
Providing Port...
Providing Host...
Retrieved Host: localhost:8080
Requesting Host again...
Retrieved Host again: localhost:8080
Assigning variables...
Assigned dbURL: postgres://user:pass@db:5432/mydb, port: 8080
*/
```

### 3. Calling Functions (`Call`)

You can execute functions whose arguments are automatically resolved from the scope using `scope.Call()`.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Port int
type Host string
type ServiceName string

func main() {
	scope := dscope.New(
		func() Port { return 8080 },
		func(p Port) Host { return Host(fmt.Sprintf("localhost:%d", p)) },
		&ServiceName{"MyWebService"},
	)

	// Call a function whose arguments are dependencies from the scope
	scope.Call(func(host Host, name ServiceName) {
		fmt.Printf("Service '%s' running at %s\n", name, host)
	})

	// Call can also return values, wrapped in a CallResult
	result := scope.Call(func(p Port) string {
		return fmt.Sprintf("Configuration uses port %d", p)
	})

	var configSummary string
	// Use Assign on the result to extract return values by type
	result.Assign(&configSummary)
	fmt.Println(configSummary)

	// Or use Extract to get return values by position
	var configSummary2 string
	result.Extract(&configSummary2) // Extracts the first return value
	fmt.Println("(Extract) " + configSummary2)
}

/* Example Output:
Service 'MyWebService' running at localhost:8080
Configuration uses port 8080
(Extract) Configuration uses port 8080
*/
```

### 4. Forking Scopes (`Fork`)

Use `Fork` to create a new scope with additional or overriding definitions.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Config string
type Service struct {
	Conf Config
}

func main() {
	// Base configuration
	baseScope := dscope.New(
		func() Config {
			fmt.Println("Providing base Config")
			return "BaseConfig"
		},
		func(c Config) Service {
			fmt.Println("Creating Service with config:", c)
			return Service{Conf: c}
		},
	)

	fmt.Println("--- Using baseScope ---")
	service1 := dscope.Get[Service](baseScope)
	fmt.Printf("Service 1 Config: %s\n", service1.Conf)

	// Create a derived scope with an overridden Config
	devScope := baseScope.Fork(
		func() Config {
			fmt.Println("Providing DEV Config")
			return "DevConfig"
		},
	)

	fmt.Println("\n--- Using devScope ---")
	// Requesting Service from devScope will use the overridden Config.
	// The Service provider function itself is inherited but will be re-run
	// because its dependency (Config) has changed in this scope lineage.
	service2 := dscope.Get[Service](devScope)
	fmt.Printf("Service 2 Config: %s\n", service2.Conf)

	fmt.Println("\n--- Using baseScope again ---")
	// The original baseScope is unaffected
	service3 := dscope.Get[Service](baseScope)
	fmt.Printf("Service 3 Config: %s\n", service3.Conf)
}

/* Example Output:
--- Using baseScope ---
Providing base Config
Creating Service with config: BaseConfig
Service 1 Config: BaseConfig

--- Using devScope ---
Providing DEV Config
Creating Service with config: DevConfig
Service 2 Config: DevConfig

--- Using baseScope again ---
Service 3 Config: BaseConfig
*/

```

## Advanced Features (Brief Overview)

*   **Modules:** Group related definitions within structs using the `dscope:"."` tag on fields (see `module_test.go`, `methods.go`).
*   **Struct Field Injection:** Automatically populate fields of a struct tagged with `dscope:"."` using `InjectStruct` (see `inject_struct.go`, `inject_struct_test.go`).
*   **Debugging:** Visualize the dependency graph using `ToDOT` (requires Graphviz) or inspect definitions with `GetDebugInfo` (see `dot.go`, `debug_info.go`).

## Next Steps

*   Explore the `*_test.go` files in the `dscope` package for more detailed examples and usage patterns of various features.
*   Review the public API in `dscope.go` and other files to understand the available functions and types.
