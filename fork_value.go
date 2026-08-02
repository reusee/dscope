package dscope

import "reflect"

const TheoryOfScopeForkValue = `
dscope fork value theory:
- The Fork type represents the Fork method bound to a specific scope instance.
- It is always provided as a built-in dependency so that consumers can create
  child scopes dynamically through dependency injection.
- Like other always-provided types, user definitions for Fork are ignored in
  favor of the built-in binding to the current scope.
- Fork is opaque to dependency analysis: a provider receiving it can resolve
  any type from the scope through child scopes. Providers depending on Fork
  are therefore pessimistically re-evaluated whenever a scope is forked with
  new definitions, mirroring the InjectStruct treatment.
`

// Fork is the type of a scope's Fork method, bound to the scope that provides
// it. It is always provided as a built-in dependency.
type Fork func(defs ...any) Scope

var forkTypeID = getTypeID(reflect.TypeFor[Fork]())
