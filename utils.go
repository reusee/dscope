package dscope

import (
	"reflect"
)

func Methods(objects ...any) (ret []any) {
	for _, object := range objects {
		v := reflect.ValueOf(object)
		for i := range v.NumMethod() {
			ret = append(ret, v.Method(i).Interface())
		}
	}
	return
}

func Provide[T any](v T) *T {
	return &v
}
