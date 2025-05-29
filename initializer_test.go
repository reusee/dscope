package dscope

import "testing"

func BenchmarkNewInitializerPointer(b *testing.B) {
	i := 42
	for b.Loop() {
		newInitializer(&i, true)
	}
}
