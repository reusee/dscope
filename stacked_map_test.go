package dscope

import "testing"

func TestStackedMap(t *testing.T) {
	var m *_StackedMap
	m = m.Append([]_Value{
		{_TypeInfo: &_TypeInfo{TypeID: 1, Position: 1}},
		{_TypeInfo: &_TypeInfo{TypeID: 2, Position: 2}},
		{_TypeInfo: &_TypeInfo{TypeID: 2, Position: 3}},
		{_TypeInfo: &_TypeInfo{TypeID: 2, Position: 4}},
		{_TypeInfo: &_TypeInfo{TypeID: 3, Position: 5}},
	})
	m = m.Append([]_Value{
		{_TypeInfo: &_TypeInfo{TypeID: 3, Position: 6}},
	})

	vs, ok := m.Load(1)
	if !ok {
		t.Fatal()
	}
	if len(vs) != 1 {
		t.Fatal()
	}
	if vs[0].Position != 1 {
		t.Fatal()
	}

	vs, ok = m.Load(2)
	if !ok {
		t.Fatal()
	}
	if len(vs) != 3 {
		t.Fatalf("got %d", len(vs))
	}
	if vs[0].Position != 2 {
		t.Fatal()
	}
	if vs[1].Position != 3 {
		t.Fatal()
	}
	if vs[2].Position != 4 {
		t.Fatal()
	}

	_, ok = m.Load(42)
	if ok {
		t.Fatal()
	}

	n := 0
	if err := m.Range(func(ds []_Value) error {
		for _, d := range ds {
			n++
			if d.TypeID == 3 {
				if d.Position != 6 {
					t.Fatal()
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Fatalf("got %d", n)
	}

}
