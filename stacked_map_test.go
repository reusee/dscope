package dscope

import "testing"

func TestStackedMap(t *testing.T) {
	var m _StackedMap
	m = append(m, []_Decl{
		{TypeID: 1, ValueIndex: 1},
		{TypeID: 2, ValueIndex: 2},
		{TypeID: 2, ValueIndex: 3},
		{TypeID: 2, ValueIndex: 4},
		{TypeID: 3, ValueIndex: 5},
	})
	m = append(m, []_Decl{
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

	vs, ok = m.Load(2)
	if !ok {
		t.Fatal()
	}
	if len(vs) != 3 {
		t.Fatalf("got %d", len(vs))
	}
	if vs[0].ValueIndex != 2 {
		t.Fatal()
	}
	if vs[1].ValueIndex != 3 {
		t.Fatal()
	}
	if vs[2].ValueIndex != 4 {
		t.Fatal()
	}

	_, ok = m.Load(42)
	if ok {
		t.Fatal()
	}

	n := 0
	m.Range(func(ds []_Decl) error {
		for _, d := range ds {
			n++
			if d.TypeID == 3 {
				if d.ValueIndex != 6 {
					t.Fatal()
				}
			}
		}
		return nil
	})
	if n != 5 {
		t.Fatalf("got %d", n)
	}

}
