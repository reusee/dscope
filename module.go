package dscope

import "reflect"

// TheoryOfModules documents the module pattern: grouping providers as methods
// on a struct that embeds dscope.Module.
const TheoryOfModules = `
dscope module theory:
- A module is a struct embedding dscope.Module; it groups related providers
  as methods around one capability — a database, an HTTP server, a domain
  service — instead of scattering provider functions.
- Each exported method of the module instance becomes a provider. A method
  must return at least one value.
- Modules compose: embedding one module in another, or holding module-typed
  fields, pulls in the embedded module's methods recursively.
- A module reaches the scope either as a definition passed to New or Fork
  (recognized by the embedded Module), or explicitly through Methods(module)...,
  which supports conditional composition.
- Method values are bound to the module instance, so providers built from
  methods can read state the module owns.
`

type Module struct{}

type isModule interface {
	isDscopeModule()
}

var isModuleType = reflect.TypeFor[isModule]()

func (Module) isDscopeModule() {}
