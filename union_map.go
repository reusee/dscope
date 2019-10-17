package dscope

type UnionMap [][]*_TypeDecl

func (u UnionMap) Load(id _TypeID) (d _TypeDecl, ok bool) {
	for i := len(u) - 1; i >= 0; i-- {
		m := u[i]
		if len(m) <= int(id) {
			continue
		}
		if m[id] == nil {
			continue
		}
		d = *m[id]
		ok = true
		break
	}
	return
}

func (u UnionMap) Range(fn func(_TypeDecl)) {
	keys := make(map[_TypeID]struct{})
	var m []*_TypeDecl
	for i := len(u) - 1; i >= 0; i-- {
		m = u[i]
		for _, d := range m {
			if d == nil {
				continue
			}
			if _, ok := keys[d.TypeID]; ok {
				continue
			}
			keys[d.TypeID] = struct{}{}
			fn(*d)
		}
	}
}
