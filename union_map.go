package dscope

type _UnionMap [][]_Decl

// Load loads decls with specified id
// MUST NOT modify returned slice
func (u _UnionMap) Load(id _TypeID) ([]_Decl, bool) {
	var left, right, idx, l uint
	var start, end int
	var id2 _TypeID
	var m []_Decl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		// do binary search
		left = 0
		l = uint(len(m))
		right = l
		// find left bound
		for left < right {
			idx = (left + right) >> 1
			id2 = m[idx].TypeID
			// need to find the first decl
			if id2 >= id {
				right = idx
			} else {
				left = idx + 1
			}
		}
		start = -1
		for ; idx < l; idx++ {
			id2 = m[idx].TypeID
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
			return m[start:end], true
		}
	}
	return nil, false
}

func (u _UnionMap) LoadOne(id _TypeID) (ret _Decl, ok bool) {
	var left, right, idx uint
	var id2 _TypeID
	var m []_Decl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		left = 0
		right = uint(len(m))
		for left < right {
			idx = (left + right) >> 1
			id2 = m[idx].TypeID
			if id2 > id {
				right = idx
			} else if id2 < id {
				left = idx + 1
			} else {
				return m[idx], true
			}
		}
	}
	return
}

// Range iterates all decls
// MUST NOT modify []_Decl argument in callback function
func (u _UnionMap) Range(fn func([]_Decl) error) error {
	keys := make(map[_TypeID]struct{})
	var m []_Decl
	var start, end int
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		for j, d := range m {
			if _, ok := keys[d.TypeID]; ok {
				continue
			}
			keys[d.TypeID] = struct{}{}
			start = j
			end = start + 1
			for _, follow := range m[j+1:] {
				if follow.TypeID == d.TypeID {
					end++
				} else {
					break
				}
			}
			if err := fn(m[start:end]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (u _UnionMap) Len() int {
	ret := 0
	for _, decls := range u {
		ret += len(decls)
	}
	return ret
}
