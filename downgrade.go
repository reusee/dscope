package dscope

import "reflect"

func Downgrade(rs []any, t reflect.Type) []any {
	inOut := make(map[reflect.Type]map[reflect.Type]bool)
	outIn := make(map[reflect.Type]map[reflect.Type]bool)
	for _, r := range rs {
		t := reflect.TypeOf(r)
		for i := 0; i < t.NumIn(); i++ {
			in := t.In(i)
			for j := 0; j < t.NumOut(); j++ {
				out := t.Out(j)

				m, ok := inOut[in]
				if !ok {
					m = make(map[reflect.Type]bool)
					inOut[in] = m
				}
				m[out] = true

				m, ok = outIn[out]
				if !ok {
					m = make(map[reflect.Type]bool)
					outIn[out] = m
				}
				m[in] = true

			}
		}
	}

	var using func(in, out reflect.Type) bool
	using = func(in, out reflect.Type) bool {
		if m, ok := outIn[out]; ok {
			if m[in] {
				return true
			}
			for i := range m {
				if using(i, out) {
					return true
				}
			}
		}
		return false
	}

	var ret []any
loop:
	for _, r := range rs {
		rtype := reflect.TypeOf(r)
		for i := 0; i < rtype.NumOut(); i++ {
			out := rtype.Out(i)
			if out == t {
				continue loop
			}
			if using(t, out) {
				continue loop
			}
		}
		ret = append(ret, r)
	}

	return ret
}
