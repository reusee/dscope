package dscope

type UnionMap [][]_TypeDecl

func (u UnionMap) Load(id _TypeID) (ds []_TypeDecl, ok bool) {
	var left, right, idx uint
	var id2 _TypeID
	var m []_TypeDecl
	typeID := id
	id-- // to find left bound of target id
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		left = 0
		right = uint(len(m))
		for left < right {
			idx = (left + right) >> 1
			id2 = m[idx].TypeID
			if id < id2 {
				right = idx
			} else if id > id2 {
				left = idx + 1
			} else {
				break
			}
		}
		right := uint(len(m))
		for ; idx < right; idx++ {
			id2 := m[idx].TypeID
			if id2 > typeID {
				break
			}
			if id2 != typeID {
				continue
			}
			ds = append(ds, m[idx])
		}
		if len(ds) > 0 {
			ok = true
			return
		}
	}
	return
}

func (u UnionMap) Range(fn func([]_TypeDecl)) {
	keys := make(map[_TypeID]struct{})
	var m []_TypeDecl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		for j, d := range m {
			if _, ok := keys[d.TypeID]; ok {
				continue
			}
			keys[d.TypeID] = struct{}{}
			ds := []_TypeDecl{d}
			for _, follow := range m[j+1:] {
				if follow.TypeID == d.TypeID {
					ds = append(ds, follow)
				} else {
					break
				}
			}
			fn(ds)
		}
	}
}

func (u UnionMap) Len() int {
	ret := 0
	for _, decls := range u {
		ret += len(decls)
	}
	return ret
}
