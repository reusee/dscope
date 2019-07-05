package dscope

import "testing"

func TestSubFuncsMap(t *testing.T) {
	m := make(_SubFuncsMap, 0)

	m = m.Insert(_SubFuncsSpec{
		Sig1: []byte("x"),
		Sig2: []byte("x"),
	})
	// <x x>
	if len(m) != 1 || string(m[0].Sig1) != "x" {
		t.Fatal()
	}

	m = m.Insert(_SubFuncsSpec{
		Sig1: []byte("m"),
		Sig2: []byte("m"),
	})
	// <m m> <x x>
	if len(m) != 2 || string(m[0].Sig2) != "m" {
		t.Fatal()
	}

	m = m.Insert(_SubFuncsSpec{
		Sig1: []byte("y"),
		Sig2: []byte("y"),
	})
	// <m m> <x x> <y y>
	if len(m) != 3 || string(m[2].Sig2) != "y" {
		t.Fatal()
	}

	m = m.Insert(_SubFuncsSpec{
		Sig1: []byte("m"),
		Sig2: []byte("m"),
	})
	// <m m> <x x> <y y>
	if len(m) != 3 {
		t.Fatal()
	}

	m = m.Insert(_SubFuncsSpec{
		Sig1: []byte("x"),
		Sig2: []byte("r"),
	})
	// <m m> <x r> <x x> <y y>
	if len(m) != 4 || string(m[1].Sig2) != "r" {
		t.Fatal()
	}

	m = m.Insert(_SubFuncsSpec{
		Sig1: []byte("x"),
		Sig2: []byte("y"),
	})
	// <m m> <x r> <x x> <x y> <y y>
	if len(m) != 5 || string(m[3].Sig2) != "y" {
		t.Fatal()
	}

	_, ok := m.Find([]byte("m"), []byte("m"))
	if !ok {
		t.Fatal()
	}
	_, ok = m.Find([]byte("x"), []byte("r"))
	if !ok {
		t.Fatal()
	}
	_, ok = m.Find([]byte("x"), []byte("x"))
	if !ok {
		t.Fatal()
	}
	_, ok = m.Find([]byte("x"), []byte("y"))
	if !ok {
		t.Fatal()
	}
	_, ok = m.Find([]byte("y"), []byte("y"))
	if !ok {
		t.Fatal()
	}

	_, ok = m.Find([]byte("a"), []byte("a"))
	if ok {
		t.Fatal()
	}

}
