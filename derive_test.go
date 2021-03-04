package dscope

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestDeriveCall(t *testing.T) {
	scope := NewDeriving(func() int {
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
				derive DeriveCall,
			) {
				derive(func(n int) *int {
					n += i + 1
					return &n
				})
			})
		}()
	}
	wg.Wait()

	scope.Call(func(
		latest GetLatest,
	) {
		scope = latest()
		var i int
		scope.Assign(&i)
		if i != (n*(n+1))/2 {
			t.Fatal()
		}
	})

}

func TestDerive(t *testing.T) {
	scope := NewDeriving()

	n := 100
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, 1)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			scope.Call(func(
				derive Derive,
			) {
				s := derive(&i)
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

func TestDeriveLoop(t *testing.T) {
	s := NewDeriving()
	s.Call(func(
		derive DeriveCall,
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
				if !strings.Contains(s, "derive loop") {
					t.Fatal()
				}
			}()

			derive(func() {
				derive(func() *int {
					i := 1
					return &i
				})
			})
		}()
	})
}
