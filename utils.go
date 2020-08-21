package dscope

import (
	"reflect"
)

func Methods(objects ...any) (ret []any) {
	for _, object := range objects {
		v := reflect.ValueOf(object)
		for i := 0; i < v.NumMethod(); i++ {
			ret = append(ret, v.Method(i).Interface())
		}
	}
	return
}
