package dscope

import "testing"

func TestUnionMap(t *testing.T) {
	var m UnionMap
	m = append(m, []_TypeDecl{
		{TypeID: 1, ValueIndex: 1},
		{TypeID: 2, ValueIndex: 2},
		{TypeID: 3, ValueIndex: 3},
	})
	m = append(m, []_TypeDecl{
		{TypeID: 3, ValueIndex: 6},
	})

	vs, ok := m.Load(1)
	if !ok {
		t.Fatal()
	}
	if len(vs) != 1 {
		t.Fatal()
	}
	if vs[0].ValueIndex != 1 {
		t.Fatal()
	}

	_, ok = m.Load(42)
	if ok {
		t.Fatal()
	}

	n := 0
	m.Range(func(ds []_TypeDecl) {
		for _, d := range ds {
			n++
			if d.TypeID == 3 {
				if d.ValueIndex != 6 {
					t.Fatal()
				}
			}
		}
	})
	if n != 3 {
		t.Fatal()
	}

}
