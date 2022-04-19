package dscope

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func FuzzFork(f *testing.F) {

	type Type struct {
		Type  reflect.Type
		Value func(r int) any
	}

	allTypes := []Type{
		{
			Type:  reflect.TypeOf((*int)(nil)).Elem(),
			Value: func(r int) any { return r },
		},
		{
			Type:  reflect.TypeOf((*int64)(nil)).Elem(),
			Value: func(r int) any { return int64(r) },
		},
		{
			Type:  reflect.TypeOf((*uint64)(nil)).Elem(),
			Value: func(r int) any { return uint64(r) },
		},
		{
			Type:  reflect.TypeOf((*string)(nil)).Elem(),
			Value: func(r int) any { return fmt.Sprintf("%d", r) },
		},
	}

	scope := New()

	f.Fuzz(func(t *testing.T, r int) {

		rand.Shuffle(len(allTypes), func(i, j int) {
			allTypes[i], allTypes[j] = allTypes[j], allTypes[i]
		})
		types := allTypes[:1+rand.Intn(len(allTypes)-1)]

		fn := reflect.MakeFunc(
			reflect.FuncOf(
				[]reflect.Type{},
				func() (ret []reflect.Type) {
					for _, t := range types {
						ret = append(ret, t.Type)
					}
					return
				}(),
				false,
			),
			func(args []reflect.Value) (ret []reflect.Value) {
				for _, t := range types {
					ret = append(ret, reflect.ValueOf(t.Value(r)))
				}
				return
			},
		).Interface()
		scope = scope.Fork(fn)

		for _, typ := range types {
			ptr := reflect.New(typ.Type)
			scope.Assign(ptr.Interface())
			if !reflect.DeepEqual(
				ptr.Elem().Interface(),
				typ.Value(r),
			) {
				t.Fatal()
			}
		}

	})

}
