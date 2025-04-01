package dscope

func Provide[T any](v T) *T {
	return &v
}
