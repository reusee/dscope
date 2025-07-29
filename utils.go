package dscope

// Provide is a helper that returns a pointer to a value, suitable for use as a
// definition in a Scope. The value is copied when the scope is created.
func Provide[T any](v T) *T {
	return &v
}
