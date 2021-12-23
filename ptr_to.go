package dscope

func PtrTo[T any](value T) *T {
	return &value
}
