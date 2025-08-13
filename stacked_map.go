package dscope

import "iter"

// _StackedMap implements an immutable, singly-linked list acting as a stack of key-value maps.
// Each node holds a batch of sorted _Value entries. Searches start from the head.
type _StackedMap struct {
	Next   *_StackedMap // Previous layer in the stack.
	Values []_Value     // Values in this layer, sorted by TypeID.
	Height int          // Height of the stack from this node downwards.
}

// Load finds the value with the specified TypeID.
func (s *_StackedMap) Load(id _TypeID) (ret _Value, ok bool) {
	for s != nil {
		values := s.Values
		l := uint(len(values))
		if l == 0 {
			s = s.Next
			continue
		}

		// Binary search
		left, right := uint(0), l
		for left < right {
			mid := (left + right) >> 1
			midID := values[mid].typeInfo.TypeID
			if midID > id {
				right = mid
			} else if midID < id {
				left = mid + 1
			} else {
				return values[mid], true // Found
			}
		}
		s = s.Next
	}
	return // Not found
}

func (s *_StackedMap) IterValues() iter.Seq[_Value] {
	return func(yield func(_Value) bool) {
		keys := make(map[_TypeID]struct{})
		for s != nil {
			for _, d := range s.Values {
				if _, ok := keys[d.typeInfo.TypeID]; ok {
					continue
				}
				keys[d.typeInfo.TypeID] = struct{}{}
				if !yield(d) {
					return
				}
			}
			s = s.Next
		}
	}
}

// Append creates a new _StackedMap layer on top of the current one.
// The provided values must be pre-sorted by TypeID.
func (s *_StackedMap) Append(values []_Value) *_StackedMap {
	var height int = 1
	if s != nil {
		height = s.Height + 1
	}
	return &_StackedMap{
		Values: values,
		Next:   s,
		Height: height,
	}
}

// Len returns the total number of individual _Value entries across all layers.
func (s *_StackedMap) Len() int {
	ret := 0
	for s != nil {
		ret += len(s.Values)
		s = s.Next
	}
	return ret
}
