package dscope

import "iter"

// _StackedMap implements an immutable, singly-linked list acting as a stack of key-value maps.
// Each node holds a batch of sorted _Value entries. Searches start from the head.
type _StackedMap struct {
	Next   *_StackedMap // Previous layer in the stack.
	Values []_Value     // Values in this layer, sorted by TypeID.
	Height int8         // Height of the stack from this node downwards.
}

// Load finds all values associated with a given TypeID.
// It searches from the top layer downwards.
// The returned slice MUST NOT be modified.
func (s *_StackedMap) Load(id _TypeID) ([]_Value, bool) {
	for s != nil {
		values := s.Values
		l := uint(len(values))
		if l == 0 {
			s = s.Next
			continue
		}

		// Binary search to find the first matching value
		left, right := uint(0), l
		for left < right {
			mid := (left + right) >> 1
			midID := values[mid].typeInfo.TypeID
			if midID >= id {
				right = mid
			} else {
				left = mid + 1
			}
		}

		// Check if we found a match and return the range
		if left < l && values[left].typeInfo.TypeID == id {
			start := int(left)
			end := start + 1
			for end < int(l) && values[end].typeInfo.TypeID == id {
				end++
			}
			return values[start:end], true
		}

		s = s.Next
	}
	return nil, false
}

// LoadOne finds the first occurrence of a value with the specified TypeID.
// It's generally faster than Load when only one value is expected
func (s *_StackedMap) LoadOne(id _TypeID) (ret _Value, ok bool) {
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

func (s *_StackedMap) AllValues() iter.Seq[[]_Value] {
	return func(yield func([]_Value) bool) {
		keys := make(map[_TypeID]struct{}) // Track visited TypeIDs
		var start, end int
		for s != nil {
			for j, d := range s.Values {
				// Skip if this TypeID was already processed from a higher layer
				if _, ok := keys[d.typeInfo.TypeID]; ok {
					continue
				}
				keys[d.typeInfo.TypeID] = struct{}{}

				// Find the range of values for this TypeID in the current layer
				start = j
				end = start + 1
				for _, follow := range s.Values[j+1:] {
					if follow.typeInfo.TypeID == d.typeInfo.TypeID {
						end++
					} else {
						break
					}
				}

				if !yield(s.Values[start:end]) {
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
	var height int8 = 1
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
