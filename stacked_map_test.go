package dscope

import "testing"

func TestStackedMap(t *testing.T) {
	var m *_StackedMap
	m = m.Append([]_Value{
		{typeInfo: &_TypeInfo{TypeID: 1, Position: 1}},
		{typeInfo: &_TypeInfo{TypeID: 2, Position: 3}},
		{typeInfo: &_TypeInfo{TypeID: 3, Position: 5}},
	})
	m = m.Append([]_Value{
		{typeInfo: &_TypeInfo{TypeID: 3, Position: 6}},
	})

	v, ok := m.Load(1)
	if !ok {
		t.Fatal()
	}
	if v.typeInfo.Position != 1 {
		t.Fatal()
	}

	v, ok = m.Load(2)
	if !ok {
		t.Fatal()
	}
	if v.typeInfo.Position != 3 {
		t.Fatal()
	}

	_, ok = m.Load(42)
	if ok {
		t.Fatal()
	}

	n := 0
	for value := range m.IterValues() {
		n++
		if value.typeInfo.TypeID == 3 {
			if value.typeInfo.Position != 6 {
				t.Fatal()
			}
		}
	}
	if n != 3 {
		t.Fatalf("got %d", n)
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
		_, ok := m.Load(3)
		if !ok {
			b.Fatal()
		}
	}
}
