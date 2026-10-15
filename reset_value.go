package dscope

// Reset is the type of a scope's Reset method, bound to the scope that provides
// it. It is always provided as a built-in dependency.
type Reset func() Scope

const resetTypeID _TypeID = 3
