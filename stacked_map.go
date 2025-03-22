package dscope

type _StackedMap struct {
	Next   *_StackedMap
	Values []_Value
	Height int8
}

// Load loads values with specified id
// MUST NOT modify returned slice
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

		// Check if we found a match
		if left < l && values[left].typeInfo.TypeID == id {
			// Found first match, now find the range
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
				return values[mid], true
			}
		}
		s = s.Next
	}
	return
}

// Range iterates all values
// MUST NOT modify []_Value argument in callback function
func (s *_StackedMap) Range(fn func([]_Value) error) error {
	keys := make(map[_TypeID]struct{})
	var start, end int
	for s != nil {
		for j, d := range s.Values {
			if _, ok := keys[d.typeInfo.TypeID]; ok {
				continue
			}
			keys[d.typeInfo.TypeID] = struct{}{}
			start = j
			end = start + 1
			for _, follow := range s.Values[j+1:] {
				if follow.typeInfo.TypeID == d.typeInfo.TypeID {
					end++
				} else {
					break
				}
			}
			if err := fn(s.Values[start:end]); err != nil {
				return err
			}
		}
		s = s.Next
	}
	return nil
}

// Append appends a new layer to the map. values must be sorted by type id.
func (s *_StackedMap) Append(values []_Value) *_StackedMap {
	if s != nil {
		return &_StackedMap{
			Values: values,
			Next:   s,
			Height: s.Height + 1,
		}
	}
	return &_StackedMap{
		Values: values,
		Next:   s,
		Height: 1,
	}
}

func (s *_StackedMap) Len() int {
	ret := 0
	for s != nil {
		ret += len(s.Values)
		s = s.Next
	}
	return ret
}
