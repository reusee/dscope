# dscope - An Immutable Dependency Injection Library for Go

`dscope` is a dependency injection library for Go focused on immutability, lazy initialization, and type safety. It provides a flexible way to manage dependencies within your application.

## Why Use `dscope`?

*   **Type-Safe Dependencies:** Leverages Go's type system. Generic functions like `Get[T]` and `Assign[T]` provide compile-time safety, while other operations perform runtime type checks.
*   **Promotes Loose Coupling:** Dependencies are resolved based on their type (including interface types), allowing you to depend on abstractions rather than concrete implementations.
*   **Enhanced Testability:** Easily swap implementations for testing by forking scopes and providing mock definitions.
*   **Immutable and Predictable:** Scopes are immutable. Operations like `Fork` create new `Scope` values, preserving the state of the original. This makes reasoning about dependency states simpler and safer, especially in concurrent environments.
*   **Lazy Initialization:** Values are computed only when they are first requested, thanks to `sync.Once`. This improves performance by avoiding unnecessary initialization work.
*   **Concurrency Safe:** Designed for concurrent use. Reading values (`Get`, `Assign`, `Call`) and creating new scopes (`Fork`) from existing scopes can be done safely from multiple goroutines.
*   **Performance Conscious:** While using reflection, `dscope` employs internal caching for fork operations, function argument lookups, and return type analysis to mitigate performance overhead in common scenarios.

## Core Concepts

1.  **Scope:**
    *   The central concept in `dscope`. A `Scope` is an immutable container that holds dependency definitions and resolved values.
    *   The root scope is `dscope.Universe`. New scopes are typically created using `dscope.New(...)` (which is equivalent to `dscope.Universe.Fork(...)`).

2.  **Definitions:**
    *   These tell the scope how to create or provide values. Definitions are passed as arguments to `New` or `Fork`.
    *   **Functions:** The most common way. A function like `func(depA A, depB B) C { ... }` defines how to create a value of type `C`, declaring its dependencies (`A` and `B`) as arguments. Functions can also return multiple values (`func() (A, B)`), defining providers for both `A` and `B`.
    *   **Pointers:** Providing a pointer like `*T` makes the pointed-to value available in the scope with type `T`. Example: `i := 42; scope := dscope.New(&i)` makes an `int` available.

3.  **Fork:**
    *   The primary operation for creating new scopes from existing ones. `parentScope.Fork(newDefs...)` creates a new child scope.
    *   Definitions in the child scope *layer on top* of the parent's definitions.
    *   If a child defines a provider for a type already defined in the parent, the child's definition *overrides* the parent's for that child scope and its descendants. The parent scope remains unchanged.
    *   Forking is efficient due to internal caching (`_Forker`).

## Installation

```bash
go get github.com/reusee/dscope
```

## Basic Usage Examples

### Create a New Scope

```go
package main

import "github.com/reusee/dscope"

type Config struct {
    ConnectionString string
}

func provideConfig() Config {
    return Config{ConnectionString: "localhost:5432"}
}

func main() {
    // Create a scope with a Config provider
    scope := dscope.New(
        provideConfig, // Function provider
    )
    // scope now knows how to provide Config
}
```

### Get Values by Type

Values are retrieved lazily when requested.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type ServiceA struct {
    Dep ServiceB
}

type ServiceB struct{}

func provideServiceA(b ServiceB) ServiceA {
    fmt.Println("Providing ServiceA")
    return ServiceA{Dep: b}
}

func provideServiceB() ServiceB {
    fmt.Println("Providing ServiceB")
    return ServiceB{}
}

func main() {
    scope := dscope.New(
        provideServiceA,
        provideServiceB,
    )

    // Using generic Get (panics if not found)
    fmt.Println("Getting ServiceA...")
    serviceA := dscope.Get[ServiceA](scope)
    fmt.Printf("Got ServiceA: %+v\n", serviceA)

    // Using Assign (panics if not found or target is not pointer)
    fmt.Println("\nAssigning ServiceB...")
    var serviceB ServiceB
    scope.Assign(&serviceB) // Resolves ServiceB
    fmt.Printf("Assigned ServiceB: %+v\n", serviceB)

    // Requesting ServiceA again - provider is NOT called again
    fmt.Println("\nGetting ServiceA again...")
    serviceA2 := dscope.Get[ServiceA](scope)
    fmt.Printf("Got ServiceA again: %+v\n", serviceA2)
}

/* Output:
Getting ServiceA...
Providing ServiceA
Providing ServiceB
Got ServiceA: {Dep:{}}

Assigning ServiceB...
Assigned ServiceB: {}

Getting ServiceA again...
Got ServiceA again: {Dep:{}}
*/
```

### Calling Functions in a Scope

`scope.Call` executes a function, automatically resolving its arguments from the scope.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type UserID string
type Session struct{ User UserID }

func main() {
    scope := dscope.New(
        func() UserID { return "user-123" },
        func(id UserID) Session { return Session{User: id} },
    )

    // Call a function requiring Session and UserID
    scope.Call(func(s Session, id UserID) {
        fmt.Printf("Called function with Session: %+v, UserID: %s\n", s, id)
        // Output: Called function with Session: {User:user-123}, UserID: user-123
    })

    // Call a function that returns values
    var result string
    scope.Call(func(s Session) string {
        return fmt.Sprintf("User from session: %s", s.User)
    }).Assign(&result) // Assign the string return value to 'result'

    fmt.Println(result) // Output: User from session: user-123
}
```

### Fork a Scope

Forking creates a new scope, layering definitions.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Database string

func main() {
    baseScope := dscope.New(
        func() Database { return "ProductionDB" },
        func() int { return 100 }, // Some other value
    )

    // Fork to create a testing scope with a different database
    testScope := baseScope.Fork(
        func() Database { return "TestDB" },
    )

    // Get Database from both scopes
    prodDB := dscope.Get[Database](baseScope)
    testDB := dscope.Get[Database](testScope)

    fmt.Printf("Base Scope DB: %s\n", prodDB) // Output: Base Scope DB: ProductionDB
    fmt.Printf("Test Scope DB: %s\n", testDB) // Output: Test Scope DB: TestDB

    // Other values are inherited
    baseInt := dscope.Get[int](baseScope)
    testInt := dscope.Get[int](testScope)
    fmt.Printf("Base Scope Int: %d\n", baseInt) // Output: Base Scope Int: 100
    fmt.Printf("Test Scope Int: %d\n", testInt) // Output: Test Scope Int: 100
}
```

### Modules

Group related definitions using structs embedding `dscope.Module`.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

// --- Module Definition ---
type AuthModule struct {
	dscope.Module // Embed Module
	Secrets       SecretProvider `dscope:"."` // Include methods from SecretProvider field
}

func (m AuthModule) AuthService(sp SecretProvider) string {
	return "AuthService using " + sp.GetSecret()
}

type SecretProvider struct{}

func (sp SecretProvider) GetSecret() string {
	return "secret-key"
}
// --- End Module Definition ---


func main() {
    // Register the module instance directly
    scope := dscope.New(
        new(AuthModule), // Pass module instance
        // Alternatively: dscope.Methods(new(AuthModule))...
    )

    // Dependencies provided by the module are available
    authSvc := dscope.Get[string](scope) // Assuming AuthService is the only string provider
    fmt.Println(authSvc) // Output: AuthService using secret-key

    secret := dscope.Get[SecretProvider](scope).GetSecret()
    fmt.Println(secret) // Output: secret-key
}
```

### Struct Field Injection

Automatically inject dependencies into struct fields tagged with `dscope`.

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

type Logger struct{ Prefix string }
type Handler struct {
	Log Logger `dscope:"."` // Inject Logger here
	DB  string `dscope:"inject"` // Alternative tag
}

func main() {
    scope := dscope.New(
        func() Logger { return Logger{Prefix: "[INFO]"} },
        func() string { return "PostgresConnection" }, // Provides the DB string
    )

    // Option 1: Get InjectStruct function from scope
    injector := dscope.Get[dscope.InjectStruct](scope)
    var handler1 Handler
    err := injector(&handler1)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Handler 1: %+v\n", handler1)
    // Output: Handler 1: {Log:{Prefix:[INFO]} DB:PostgresConnection}

    // Option 2: Use scope.InjectStruct directly (if available - depends on how scope was created)
    // Note: InjectStruct is implicitly available.
    var handler2 Handler
    err = scope.InjectStruct(&handler2)
     if err != nil {
        panic(err)
    }
    fmt.Printf("Handler 2: %+v\n", handler2)
    // Output: Handler 2: {Log:{Prefix:[INFO]} DB:PostgresConnection}
}
```

## Concurrency

`dscope` is designed to be safe for concurrent use:
*   Reading from a `Scope` (`Get`, `Assign`, `Call`) is safe from multiple goroutines.
*   Forking a `Scope` (`Fork`) is safe from multiple goroutines.
*   Initialization of values is handled safely via `sync.Once`.

## Debugging

`dscope` provides tools to help understand the dependency graph:

*   **`scope.ToDOT(io.Writer)`:** Generates a graph description in the DOT language (usable with Graphviz) showing the effective providers and their dependencies.
*   **`scope.GetDebugInfo()`:** Returns structured information about the types defined in the scope and their provider function types.

