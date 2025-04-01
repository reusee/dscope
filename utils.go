package dscope

import (
	"reflect"
)

func Methods(objects ...any) (ret []any) {
	for _, object := range objects {
		v := reflect.ValueOf(object)
		// method sets
		for i := range v.NumMethod() {
			ret = append(ret, v.Method(i).Interface())
		}
		// from fields
		t := v.Type()
		if t.Kind() == reflect.Struct {
			for i := range t.NumField() {
				field := t.Field(i)
				directive := field.Tag.Get("dscope")
				if directive == "." || directive == "methods" {
					ret = append(ret, Methods(v.Field(i).Interface())...)
				}
			}
		}
	}
	return
}

func Provide[T any](v T) *T {
	return &v
}
