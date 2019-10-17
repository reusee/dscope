package dscope

import "testing"

func TestUnionMap(t *testing.T) {
	var m UnionMap
	m = append(m, []*_TypeDecl{
		nil,
		{TypeID: 1, ValueIndex: 1},
		{TypeID: 2, ValueIndex: 2},
		{TypeID: 3, ValueIndex: 3},
	})
	m = append(m, []*_TypeDecl{
		nil,
		nil,
		nil,
		{TypeID: 3, ValueIndex: 6},
	})

	v, ok := m.Load(1)
	if !ok {
		t.Fatal()
	}
	if v.ValueIndex != 1 {
		t.Fatal()
	}

	_, ok = m.Load(42)
	if ok {
		t.Fatal()
	}

	n := 0
	m.Range(func(d _TypeDecl) {
		n++
		if d.TypeID == 3 {
			if d.ValueIndex != 6 {
				t.Fatal()
			}
		}
	})
	if n != 3 {
		t.Fatal()
	}

}
