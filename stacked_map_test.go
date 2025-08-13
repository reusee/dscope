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

func TestStackedMapLargeHeight(t *testing.T) {
	// Test that demonstrates the integer overflow fix
	// With int8, height would overflow at 127, but with int it should work fine

	// Create a StackedMap with artificially high height to test boundary conditions
	var m *_StackedMap
	m = m.Append([]_Value{
		{typeInfo: &_TypeInfo{TypeID: 1, Position: 1}},
	})

	// Manually set height to a value that would overflow int8 (> 127)
	m.Height = 200

	// Verify Load operation works correctly
	v, ok := m.Load(1)
	if !ok {
		t.Fatal("Load failed with large height")
	}
	if v.typeInfo.Position != 1 {
		t.Fatal("Load returned wrong value with large height")
	}

	// Verify IterValues works correctly
	count := 0
	for range m.IterValues() {
		count++
	}
	if count != 1 {
		t.Fatal("IterValues failed with large height")
	}

	// Test that Append correctly handles large heights
	m2 := m.Append([]_Value{
		{typeInfo: &_TypeInfo{TypeID: 2, Position: 2}},
	})

	if m2.Height != 201 {
		t.Fatalf("Height calculation failed: expected 201, got %d", m2.Height)
	}

	// Verify both values are accessible
	v1, ok1 := m2.Load(1)
	v2, ok2 := m2.Load(2)

	if !ok1 || !ok2 {
		t.Fatal("Load failed after append with large height")
	}

	if v1.typeInfo.Position != 1 || v2.typeInfo.Position != 2 {
		t.Fatal("Load returned wrong values after append with large height")
	}
}
