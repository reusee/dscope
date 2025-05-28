# dscope - A Dependency Injection Library for Go

`dscope` is a powerful and flexible dependency injection library for Go, designed to promote clean architecture, enhance testability, and manage dependencies with ease. It emphasizes immutability, lazy initialization, and type safety.

## Why use dscope?

Managing dependencies in larger Go applications can become complex. `dscope` offers several advantages:

*   **Type-Safe Dependencies**: Leverages Go's type system to ensure that dependencies are resolved correctly at compile time or with clear runtime panics if a type is missing. Generic functions like `Get[T](scope)` provide compile-time type checking for retrievals.
*   **Define and Depend on Interfaces (or Concrete Types)**: While you can register and request concrete types directly, `dscope` fully supports defining providers that return interfaces and requesting dependencies via those interfaces, promoting loose coupling.
*   **Cleaner Function Signatures**: The `scope.Call(yourFunction)` feature automatically resolves the arguments for `yourFunction` from the scope. This means `yourFunction` only needs to declare its essential operational arguments, not a long list of dependencies it needs to acquire manually.
*   **Enhanced Testability**:
    *   **Immutability**: Scopes are immutable. Operations like `Fork` create new scopes, leaving the original untouched. This predictability is great for testing.
    *   **Easy Overriding**: You can easily `Fork` a scope and provide alternative (mock or stub) implementations for specific types, making unit and integration testing more straightforward.
*   **Immutable and Predictable Scopes**: Each scope is an immutable container. Modifying a scope (e.g., adding new definitions or overriding existing ones) results in a new scope instance. This makes the state of dependencies predictable and easier to reason about.
*   **Lazy Initialization**: Values within a scope are initialized lazily. A provider function is only called when the value it provides (or a dependant value) is actually requested for the first time. This can improve application startup time and resource usage.

## Core Concepts

### Scope
A `Scope` is an immutable container holding definitions for various types. It's the central piece from which you resolve dependencies. The `Universe` is the initial empty scope.

### Definitions
Definitions are functions or pointers that tell a scope how to create or provide an instance of a type.
*   **Provider Functions**: Functions that return one or more values. Their arguments are themselves resolved as dependencies from the scope.
    ```go
    func NewMyService(db Database) MyService { /* ... */ }
    func NewConfigAndLogger() (Config, Logger) { /* ... */ }
    ```
*   **Pointer Values**: Pointers to existing instances can also be used as definitions. The pointed-at value will be provided.
    ```go
    cfg := &MyConfig{Value: "example"}
    scope := dscope.New(cfg) // MyConfig is now available
    ```

### Fork
`Fork` is the primary mechanism for creating new scopes. When you `Fork` an existing scope, you create a new child scope that inherits all definitions from the parent. You can add new definitions or override existing ones in the child scope without affecting the parent.

```go
parentScope := dscope.New(func() int { return 42 })
childScope := parentScope.Fork(func() string { return "hello" }) // inherits int, adds string
overrideScope := parentScope.Fork(func() int { return 100 })   // overrides int
```

## Basic Usage Examples

### 1. Creating a New Scope

You can create a new scope from scratch using `dscope.New()` (which is equivalent to `dscope.Universe.Fork()`).

```go
package main

import (
	"fmt"
	"github.com/reusee/dscope"
)

// Define some types
type Greeter string
type Message string

// Define provider functions
func provideGreeter() Greeter {
	return "Hello"
}

func provideMessage(g Greeter) Message {
	return Message(fmt.Sprintf("%s, dscope!", g))
}

func main() {
	scope := dscope.New(
		provideGreeter,
		provideMessage,
	)
	// Scope is now configured
}
```

### 2. Getting Values by Type

You can retrieve values from the scope using `scope.Get(reflect.Type)`, the generic `dscope.Get[T](scope)`, or `scope.Assign(pointers...)`.

*   **`dscope.Get[T](scope)` (Recommended for type safety):**
    ```go
    msg := dscope.Get[Message](scope)
    fmt.Println(msg) // Output: Hello, dscope!
    ```

*   **`scope.Assign(pointers...)`:**
    ```go
    var m Message
    var g Greeter
    scope.Assign(&m, &g)
    fmt.Println(g, m) // Output: Hello Hello, dscope!
    ```

*   **`scope.Get(reflect.Type)`:**
    ```go
    import "reflect"
    // ...
    msgVal, ok := scope.Get(reflect.TypeOf(Message("")))
    if ok {
        fmt.Println(msgVal.Interface().(Message))
    }
    ```

### 3. Calling Functions in a Scope

`scope.Call(fn)` executes `fn`, automatically resolving its arguments from the scope. Return values are wrapped in a `CallResult`.

```go
type Salutation string

func provideSalutation() Salutation {
	return "Greetings"
}

scope = scope.Fork(provideSalutation) // Add Salutation to the scope

result := scope.Call(func(s Salutation, m Message) string {
	return fmt.Sprintf("%s! %s", s, m)
})

var finalMsg string
result.Assign(&finalMsg) // Assigns the string return value
// or result.Extract(&finalMsg) if order matters and you know the return position

fmt.Println(finalMsg) // Output: Greetings! Hello, dscope!```

### 4. Forking a Scope

Forking creates a new scope that inherits from the parent, allowing you to add or override definitions.

```go
baseScope := dscope.New(func() int { return 10 })

// Fork 1: Add a new type
childScope1 := baseScope.Fork(func(i int) string {
	return fmt.Sprintf("Number: %d", i)
})
fmt.Println(dscope.Get[string](childScope1)) // Output: Number: 10

// Fork 2: Override an existing type
childScope2 := baseScope.Fork(func() int { return 20 })
fmt.Println(dscope.Get[int](childScope2)) // Output: 20

// Original scope is unaffected
fmt.Println(dscope.Get[int](baseScope)) // Output: 10
```

### 5. Modules

Modules help organize definitions. You can embed `dscope.Module` in your structs and then use `dscope.Methods(moduleInstances...)` to add all exported methods of those instances (and their embedded modules) as providers to the scope.

```go
type DatabaseModule struct {
	dscope.Module
}

func (dbm *DatabaseModule) ProvideDBConnection() string { // Becomes a provider
	return "db_connection_string"
}

type ServiceModule struct {
	dscope.Module
	DBDep DatabaseModule // Embedded module's methods will also be added
}

func (sm *ServiceModule) ProvideMyService(dbConn string) string { // Becomes a provider
	return "service_using_" + dbConn
}

func main() {
	// Using concrete instance
	scope := dscope.New(
		dscope.Methods(new(ServiceModule))...,
	)
	service := dscope.Get[string](scope) // Will try to get "service_using_db_connection_string"
	fmt.Println(service)
}
```
*Output:*
```
service_using_db_connection_string
```

You can also pass instances of structs that embed `dscope.Module` directly to `New` or `Fork`:
```go
type ModA struct {
    dscope.Module
}
func (m ModA) GetA() string { return "A from ModA" }

type ModB struct {
    dscope.Module
    MyModA ModA // ModA's methods will be included
}
func (m ModB) GetB() string { return "B from ModB" }


func main() {
    scope := dscope.New(
        new(ModB), // Automatically uses Methods() for types embedding dscope.Module
    )
    fmt.Println(dscope.Get[string](scope, dscope.WithTypeQualifier("GetA"))) // Assuming a way to qualify if GetA and GetB return string
    // For distinct return types, direct Get[T] works:
    // e.g. if GetA returns type AVal and GetB returns type BVal
    // aVal := dscope.Get[AVal](scope)
    // bVal := dscope.Get[BVal](scope)
}
```
Note: The example with `dscope.WithTypeQualifier` is illustrative if multiple providers return the same type. If return types are unique, `dscope.Get[ReturnType]` is sufficient. `dscope` primarily resolves by type.

### 6. Struct Field Injection

`dscope` can inject dependencies into the fields of a struct.

*   **Using `dscope:"."` or `dscope:"inject"` tag:**
    ```go
    type MyStruct struct {
        Dep1 Greeter `dscope:"."` // or dscope:"inject"
        Dep2 Message `dscope:"."`
    }

    scope := dscope.New(provideGreeter, provideMessage)
    var myInstance MyStruct
    // scope.Call(func(inject dscope.InjectStruct) { inject(&myInstance) })
    // OR directly:
    scope.InjectStruct(&myInstance)


    fmt.Printf("Injected: Greeter='%s', Message='%s'\n", myInstance.Dep1, myInstance.Dep2)
    // Output: Injected: Greeter='Hello', Message='Hello, dscope!'
    ```

*   **Using `dscope.Inject[T]` for lazy field injection:**
    Fields of type `dscope.Inject[T]` are populated with a function that, when called, resolves `T` from the scope. This is useful for optional dependencies or dependencies needed much later.

    ```go
    type AnotherStruct struct {
        LazyGreeter dscope.Inject[Greeter]
        RegularMsg  Message `dscope:"."`
    }

    scope := dscope.New(provideGreeter, provideMessage)
    var anotherInstance AnotherStruct
    scope.InjectStruct(&anotherInstance)

    fmt.Printf("Regular Message: %s\n", anotherInstance.RegularMsg)
    // LazyGreeter is not resolved yet.
    // To get the greeter:
    actualGreeter := anotherInstance.LazyGreeter()
    fmt.Printf("Lazy Greeter: %s\n", actualGreeter)
    // Output:
    // Regular Message: Hello, dscope!
    // Lazy Greeter: Hello
    ```

This covers the core features and usage patterns of `dscope`. Its design promotes modularity and testability in Go applications.
