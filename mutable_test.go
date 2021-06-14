package dscope

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestMutateCall(t *testing.T) {
	scope := NewMutable(func() int {
		return 0
	})

	n := 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			scope.Call(func(
				call MutateCall,
			) {
				call(func(n int) *int {
					n += i + 1
					return &n
				})
			})
		}()
	}
	wg.Wait()

	scope.Call(func(
		get GetScope,
	) {
		scope = get()
		var i int
		scope.Assign(&i)
		if i != (n*(n+1))/2 {
			t.Fatal()
		}
	})

}

func TestMutate(t *testing.T) {
	scope := NewMutable()

	n := 100
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, 1)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			scope.Call(func(
				mutate Mutate,
			) {
				s := mutate(&i)
				var x int
				s.Assign(&x)
				if x != i {
					select {
					case errCh <- fmt.Errorf("bad i"):
					default:
					}
				}
			})
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

}

func TestMutateLoop(t *testing.T) {
	s := NewMutable()
	s.Call(func(
		call MutateCall,
	) {
		func() {
			defer func() {
				p := recover()
				if p == nil {
					t.Fatal("should panic")
				}
				s, ok := p.(string)
				if !ok {
					t.Fatal("should be string")
				}
				if !strings.Contains(s, "dependency loop") {
					t.Fatal()
				}
			}()

			call(func() {
				call(func() *int {
					i := 1
					return &i
				})
			})
		}()
	})
}

func BenchmarkMutatableGet(b *testing.B) {
	s := NewMutable()
	for i := 0; i < b.N; i++ {
		s.Call(func(
			get GetScope,
		) {
			get()
		})
	}
}

func BenchmarkMutable(b *testing.B) {
	s := NewMutable()
	for i := 0; i < b.N; i++ {
		s.Call(func(
			mutate Mutate,
		) {
			i := 42
			mutate(&i)
		})
	}
}
func BenchmarkMutateCall(b *testing.B) {
	s := NewMutable()
	for i := 0; i < b.N; i++ {
		s.Call(func(
			call MutateCall,
		) {
			call(func() *int {
				i := 42
				return &i
			})
		})
	}
}
