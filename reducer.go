package dscope

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Reducer interface {
	Reduce(Scope, []reflect.Value) reflect.Value
}

var reducerType = reflect.TypeOf((*Reducer)(nil)).Elem()

var ErrNoValues = errors.New("no values")

func Reduce(vs []reflect.Value) reflect.Value {
	if len(vs) == 0 {
		panic(ErrNoValues)
	}

	t := vs[0].Type()
	ret := reflect.New(t).Elem()

	switch t.Kind() {

	case reflect.Slice:
		for _, v := range vs {
			var elems []reflect.Value
			for i := 0; i < v.Len(); i++ {
				elems = append(elems, v.Index(i))
			}
			ret = reflect.Append(ret, elems...)
		}

	case reflect.String:
		var b strings.Builder
		for _, v := range vs {
			b.WriteString(v.String())
		}
		ret = reflect.New(t)
		ret.Elem().SetString(b.String())
		ret = ret.Elem()

	case reflect.Func:
		ret = reflect.MakeFunc(
			t,
			func(args []reflect.Value) (rets []reflect.Value) {
				for _, v := range vs {
					rets = v.Call(args)
				}
				return
			},
		)

	case reflect.Map:
		ret = reflect.MakeMap(t)
		for _, v := range vs {
			iter := v.MapRange()
			for iter.Next() {
				ret.SetMapIndex(iter.Key(), iter.Value())
			}
		}

	case reflect.Int:
		var i int64
		for _, v := range vs {
			i += v.Int()
		}
		ret = reflect.New(t).Elem()
		ret.SetInt(i)

	default: // NOCOVER
		panic(fmt.Errorf("don't know how to reduce %v", t))
	}

	return ret
}
