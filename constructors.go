package dscope

const TheoryOfConstructors = `
dscope constructor theory:
- A process that needs one or more scope values is modeled as a named
  function type. The scope receives an implementation through a provider:
  the provider's parameters are precisely the values the process needs, the
  scope injects them when the provider is built, and the returned function
  closes over them for later calls.
- The common case is a constructor: a named function type that returns the
  value it builds and an error, type NewFoo func() (*Foo, error). The
  provider of a constructor has the same name and declares its dependencies
  as parameters:
      func (m Module) NewFoo(dep1 Dep1, dep2 Dep2) NewFoo { ... }
- The constructed type, the constructor type, and the provider share the name
  NewFoo. Factory-style names such as xyzFactory are avoided: one concept, one
  name.
- Consumers depend on the function type, not on a concrete value or provider.
  Adding the type to another provider's parameters makes the process callable
  there. A constructor is invoked where its value is needed, and its error is
  handled at that call site, keeping construction visible, testable, and
  replaceable.
`
