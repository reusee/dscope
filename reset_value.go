package dscope

import "reflect"

// Reset is the type of a scope's Reset method, bound to the scope that provides
// it. It is always provided as a built-in dependency.
type Reset func() Scope

var resetTypeID = getTypeID(reflect.TypeFor[Reset]())
