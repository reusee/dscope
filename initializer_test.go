package dscope

import (
	"fmt"
	"testing"
)

func TestInitializerPanicRetry(t *testing.T) {
	type Foo int
	scope := New(func() Foo {
		panic("provider panic")
	})

	for i := range 2 {
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatalf("call %d: should panic", i)
				}
				if str := fmt.Sprintf("%v", p); str != "provider panic" {
					t.Fatalf("call %d: expected 'provider panic', got %v", i, str)
				}
			}()
			Get[Foo](scope)
		}()
	}
}

func BenchmarkNewInitializerPointer(b *testing.B) {
	i := 42
	for b.Loop() {
		newInitializer(&i, true)
	}
}