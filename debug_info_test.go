package dscope

import "testing"

func TestDebugInfo(t *testing.T) {
	s := New(func() int {
		return 42
	})
	s = s.Fork(DebugDefs)

	var debugInfo DebugInfo
	s.Assign(&debugInfo)
}
