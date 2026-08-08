package dscope

// TheoryOfConstructors documents the constructor convention for dscope users:
// how constructor types, their providers, and the built values are named and
// declared.
const TheoryOfConstructors = `
dscope constructor theory:
- A constructor is a named function type that returns the value it builds and
  an error: type NewFoo func() (*Foo, error).
- The scope receives a constructor through a provider with the same name. The
  provider declares its dependencies as parameters and returns the constructor
  as its result:
      func (m Module) NewFoo(dep1 Dep1, dep2 Dep2) NewFoo { ... }
- The constructed type, the constructor type, and the provider share the name
  NewFoo. Factory-style names such as xyzFactory are avoided: one concept, one
  name.
- Consumers depend on the constructor type, not on the concrete value. They
  call the constructor and handle the error where the value is needed, which
  keeps construction visible, testable, and replaceable.
`
