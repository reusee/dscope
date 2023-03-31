package dscope

import "testing"

func TestDebugInfo(t *testing.T) {
	s := New(func() int {
		return 42
	})
	s.GetDebugInfo()
}
