package dscope

type UnionMap [][]_TypeDecl

func (u UnionMap) Load(id _TypeID) (d _TypeDecl, ok bool) {
	var left, right, idx uint
	var id2 _TypeID
	var m []_TypeDecl
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
				d = m[idx]
				ok = true
				return
			}
		}
	}
	return
}

func (u UnionMap) Range(fn func(_TypeDecl)) {
	keys := make(map[_TypeID]struct{})
	var m []_TypeDecl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		for _, d := range m {
			if _, ok := keys[d.TypeID]; ok {
				continue
			}
			keys[d.TypeID] = struct{}{}
			fn(d)
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
