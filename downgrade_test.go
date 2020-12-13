package dscope

import (
	"reflect"
	"testing"
)

func TestDowngrade(t *testing.T) {

	rs := []any{
		func() (_ int) { return },
	}
	rs = Downgrade(rs, reflect.TypeOf((*int)(nil)).Elem())
	if len(rs) != 0 {
		t.Fatal()
	}

	rs = []any{
		func() (_ string) { return },
	}
	rs = Downgrade(rs, reflect.TypeOf((*int)(nil)).Elem())
	if len(rs) != 1 {
		t.Fatal()
	}

	rs = []any{
		func(_ string) (_ int) { return },
		func() (_ string) { return },
	}
	rs = Downgrade(rs, reflect.TypeOf((*int)(nil)).Elem())
	if len(rs) != 1 {
		for _, r := range rs {
			pt("%T\n", r)
		}
		t.Fatal()
	}

	rs = []any{
		func(_ string) (_ int) { return },
		func() (_ string) { return },
	}
	rs = Downgrade(rs, reflect.TypeOf((*string)(nil)).Elem())
	if len(rs) != 0 {
		for _, r := range rs {
			pt("%T\n", r)
		}
		t.Fatal()
	}

	rs = []any{
		func(_ string) (_ int) { return },
		func(_ int32) (_ string) { return },
		func() (_ int32) { return },
	}
	rs = Downgrade(rs, reflect.TypeOf((*int32)(nil)).Elem())
	if len(rs) != 0 {
		for _, r := range rs {
			pt("%T\n", r)
		}
		t.Fatal()
	}

}
