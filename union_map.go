package dscope

type _UnionMap [][]_Decl

func (u _UnionMap) Load(id _TypeID) (ds []_Decl, ok bool) {
	var left, right, idx, l uint
	var id2 _TypeID
	var m []_Decl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		left = 0
		l = uint(len(m))
		right = l
		// find left bound
		for left < right {
			idx = (left + right) >> 1
			id2 = m[idx].TypeID
			if id2 >= id {
				right = idx
			} else {
				left = idx + 1
			}
		}
		for ; idx < l; idx++ {
			id2 = m[idx].TypeID
			if id2 == id {
				ds = append(ds, m[idx])
			} else if id2 > id {
				break
			}
		}
		if len(ds) > 0 {
			ok = true
			return
		}
	}
	return
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

func (u _UnionMap) Range(fn func([]_Decl) error) error {
	keys := make(map[_TypeID]struct{})
	var m []_Decl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		for j, d := range m {
			if _, ok := keys[d.TypeID]; ok {
				continue
			}
			keys[d.TypeID] = struct{}{}
			ds := []_Decl{d}
			for _, follow := range m[j+1:] {
				if follow.TypeID == d.TypeID {
					ds = append(ds, follow)
				} else {
					break
				}
			}
			if err := fn(ds); err != nil {
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
