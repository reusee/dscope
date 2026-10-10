package dscope

import "testing"

func TestStackedMap(t *testing.T) {
	var m *_StackedMap
	m = m.Append([]_Value{
		{id: 1, typeInfo: &_TypeInfo{Position: 1}},
		{id: 2, typeInfo: &_TypeInfo{Position: 3}},
		{id: 3, typeInfo: &_TypeInfo{Position: 5}},
	})
	m = m.Append([]_Value{
		{id: 3, typeInfo: &_TypeInfo{Position: 6}},
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
		if value.id == 3 {
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
			id: _TypeID(i),
			typeInfo: &_TypeInfo{
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

// TestStackedMapLargeHeight verifies that Append accumulates the height of a
// deep stack without overflow, so the flattening threshold stays reachable.
func TestStackedMapLargeHeight(t *testing.T) {
	var m *_StackedMap
	m = m.Append([]_Value{
		{id: 1, typeInfo: &_TypeInfo{Position: 1}},
	})

	// A height far above the range of a narrow height field.
	m.Height = 200

	m2 := m.Append([]_Value{
		{id: 2, typeInfo: &_TypeInfo{Position: 2}},
	})

	if m2.Height != 201 {
		t.Fatalf("Height calculation failed: expected 201, got %d", m2.Height)
	}
}

// TestStackedMapMemoKeepsTypesApart verifies that the memo of a layer answers a
// lookup only with the value of the type that was asked for. The layer resolves
// a rotation of types, so every type but the remembered one misses the memo, and
// a wrong pairing would surface as the position of another type.
func TestStackedMapMemoKeepsTypesApart(t *testing.T) {
	var m *_StackedMap
	var values []_Value
	for i := range 64 {
		values = append(values, _Value{
			id:       _TypeID(i + 1),
			typeInfo: &_TypeInfo{Position: i},
		})
	}
	m = m.Append(values)

	for round := range 3 {
		for i := range 64 {
			v, ok := m.Load(_TypeID(i + 1))
			if !ok {
				t.Fatalf("round %d: type %d not found", round, i+1)
			}
			if v.typeInfo.Position != i {
				t.Fatalf("round %d: type %d resolved to position %d, want %d", round, i+1, v.typeInfo.Position, i)
			}
		}
	}
}

// TestStackedMapIndexMissFallsThrough verifies that a layer with an index answers
// a lookup it does not hold with the value below it, and reports a type no layer
// holds as absent. A probe that reaches an empty slot must stop the search of
// that layer and leave the rest of the stack reachable.
func TestStackedMapIndexMissFallsThrough(t *testing.T) {
	var m *_StackedMap
	m = m.Append([]_Value{
		{id: 1, typeInfo: &_TypeInfo{Position: 99}},
	})
	var top []_Value
	for i := range 32 {
		top = append(top, _Value{
			id:       _TypeID(i*2 + 2),
			typeInfo: &_TypeInfo{Position: i},
		})
	}
	m = m.Append(top) // 32 values: this layer builds an index

	v, ok := m.Load(1)
	if !ok {
		t.Fatal("type of the base layer not found below an indexed layer")
	}
	if v.typeInfo.Position != 99 {
		t.Fatalf("base type resolved to position %d, want 99", v.typeInfo.Position)
	}

	if _, ok := m.Load(3); ok {
		t.Fatal("absent type reported as found")
	}

	for i := range 32 {
		v, ok := m.Load(_TypeID(i*2 + 2))
		if !ok {
			t.Fatalf("type %d not found", i*2+2)
		}
		if v.typeInfo.Position != i {
			t.Fatalf("type %d resolved to position %d, want %d", i*2+2, v.typeInfo.Position, i)
		}
	}
}
