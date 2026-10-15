package dscope

// Fork is the type of a scope's Fork method, bound to the scope that provides
// it. It is always provided as a built-in dependency.
type Fork func(defs ...any) Scope

const forkTypeID _TypeID = 1
