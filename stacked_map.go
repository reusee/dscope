package dscope

type _StackedMap struct {
	Next   *_StackedMap
	Values []_Value
	Height int8
}

// Load loads values with specified id
// MUST NOT modify returned slice
func (s *_StackedMap) Load(id _TypeID) ([]_Value, bool) {
	var left, right, idx, l uint
	var start, end int
	var id2 _TypeID
	for s != nil {
		// do binary search
		left = 0
		l = uint(len(s.Values))
		right = l
		// find left bound
		for left < right {
			idx = (left + right) >> 1
			id2 = s.Values[idx].typeInfo.TypeID
			// need to find the first value
			if id2 >= id {
				right = idx
			} else {
				left = idx + 1
			}
		}
		start = -1
		for ; idx < l; idx++ {
			id2 = s.Values[idx].typeInfo.TypeID
			if id2 == id {
				if start < 0 {
					// found start
					start = int(idx)
					end = start + 1
				} else {
					// expand
					end++
				}
			} else if id2 > id {
				// exceed right bound
				break
			}
			// id2 < id, skip
		}
		if start != -1 {
			// found
			return s.Values[start:end], true
		}
		s = s.Next
	}
	return nil, false
}

func (s *_StackedMap) LoadOne(id _TypeID) (ret _Value, ok bool) {
	var left, right, idx uint
	var id2 _TypeID
	for s != nil {
		left = 0
		right = uint(len(s.Values))
		for left < right {
			idx = (left + right) >> 1
			id2 = s.Values[idx].typeInfo.TypeID
			if id2 > id {
				right = idx
			} else if id2 < id {
				left = idx + 1
			} else {
				return s.Values[idx], true
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
