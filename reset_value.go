package dscope

import "reflect"

const TheoryOfScopeResetValue = `
dscope reset value theory:
- The Reset type represents the Reset method bound to a specific scope instance.
- It is always provided as a built-in dependency so that consumers can create
  reset scopes dynamically through dependency injection.
- Like other always-provided types, user definitions for Reset are ignored in
  favor of the built-in binding to the current scope.
- Reset is opaque to dependency analysis: a provider receiving it can resolve
  any type from the scope through reset scopes. Providers depending on Reset
  are therefore pessimistically re-evaluated whenever a scope is forked with
  new definitions, mirroring the InjectStruct and Fork treatment.
`

// Reset is the type of a scope's Reset method, bound to the scope that provides
// it. It is always provided as a built-in dependency.
type Reset func() Scope

var resetTypeID = getTypeID(reflect.TypeFor[Reset]())
