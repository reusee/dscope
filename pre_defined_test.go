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

	New(func(
		sub Sub,
	) int {
		return 42
	})

	New(func() int {
		return 1
	}).Call(func(
		assign Assign,
		sub Sub,
	) {

		sub(func() int {
			return 2
		}).Call(func(
			assign2 Assign,
		) {

			var i int
			assign(&i)
			if i != 1 {
				t.Fatal()
			}
			assign2(&i)
			if i != 2 {
				t.Fatal()
			}
			assign(&i)
			if i != 1 {
				t.Fatal()
			}

		})
	})

}
