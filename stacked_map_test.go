package dscope

import "testing"

func TestStackedMap(t *testing.T) {
	var m *_StackedMap
	m = m.Append([]_Value{
		{typeInfo: &_TypeInfo{TypeID: 1, Position: 1}},
		{typeInfo: &_TypeInfo{TypeID: 2, Position: 2}},
		{typeInfo: &_TypeInfo{TypeID: 2, Position: 3}},
		{typeInfo: &_TypeInfo{TypeID: 2, Position: 4}},
		{typeInfo: &_TypeInfo{TypeID: 3, Position: 5}},
	})
	m = m.Append([]_Value{
		{typeInfo: &_TypeInfo{TypeID: 3, Position: 6}},
	})

	vs, ok := m.Load(1)
	if !ok {
		t.Fatal()
	}
	if len(vs) != 1 {
		t.Fatal()
	}
	if vs[0].typeInfo.Position != 1 {
		t.Fatal()
	}

	vs, ok = m.Load(2)
	if !ok {
		t.Fatal()
	}
	if len(vs) != 3 {
		t.Fatalf("got %d", len(vs))
	}
	if vs[0].typeInfo.Position != 2 {
		t.Fatal()
	}
	if vs[1].typeInfo.Position != 3 {
		t.Fatal()
	}
	if vs[2].typeInfo.Position != 4 {
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
			if d.typeInfo.TypeID == 3 {
				if d.typeInfo.Position != 6 {
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

func BenchmarkStackedMapLoad(b *testing.B) {
	var m *_StackedMap
	var values []_Value
	for i := range 1024 {
		values = append(values, _Value{
			typeInfo: &_TypeInfo{
				TypeID:   _TypeID(i),
				Position: i,
			},
		})
	}
	m = m.Append(values)
	b.ResetTimer()
	for b.Loop() {
		_, ok := m.Load(3)
		if !ok {
			b.Fatal()
		}
	}
}

func BenchmarkStackedMapLoadOne(b *testing.B) {
	var m *_StackedMap
	var values []_Value
	for i := range 1024 {
		values = append(values, _Value{
			typeInfo: &_TypeInfo{
				TypeID:   _TypeID(i),
				Position: i,
			},
		})
	}
	m = m.Append(values)
	b.ResetTimer()
	for b.Loop() {
		_, ok := m.LoadOne(3)
		if !ok {
			b.Fatal()
		}
	}
}
