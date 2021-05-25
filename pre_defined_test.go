package dscope

import (
	"reflect"
	"testing"
)

func TestPredefined(t *testing.T) {
	New().Call(func(
		sub Sub,
	) {

		sub(func() int {
			return 42
		}).Call(func(
			assign Assign,
			call Call,
		) {

			var i int
			assign(&i)
			if i != 42 {
				t.Fatal()
			}

			call(func(
				get Get,
				callValue CallValue,
			) {

				v, err := get(reflect.TypeOf((*int)(nil)).Elem())
				if err != nil {
					t.Fatal(err)
				}
				if v.Int() != 42 {
					t.Fatal()
				}

				callValue(reflect.ValueOf(func(
					i int,
				) {
					if i != 42 {
						t.Fatal()
					}
				}))

			})

		})
	})
}
