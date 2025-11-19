package dscope

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestTypeIDRace(t *testing.T) {
	// This test verifies the fix for a race condition where getTypeID could
	// return an ID before the corresponding entry in idToType was established.
	// This would cause typeIDToType to panic with "impossible".

	// Number of iterations
	n := 100
	// Number of concurrent routines
	c := 8

	var wg sync.WaitGroup
	for i := range n {
		// Create a unique type for this iteration to force ID generation
		// Using ArrayOf with different sizes guarantees a unique type each time.
		typ := reflect.ArrayOf(i+1, reflect.TypeFor[int]())

		wg.Add(c)
		for range c {
			go func() {
				defer wg.Done()
				// Race starts here: multiple goroutines requesting ID for the same new type.
				id := getTypeID(typ)

				// Immediately try to resolve the ID back to the type.
				// Without the fix, this could panic because idToType[id] might not be set yet
				// even though we got the ID from typeToID.
				res := typeIDToType(id)

				if res != typ {
					panic(fmt.Sprintf("type mismatch: got %v, want %v", res, typ))
				}
			}()
		}
		wg.Wait()
	}
}
