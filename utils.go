package dscope

import (
	"fmt"
	"reflect"
)

var (
	pt = fmt.Printf
)

func Methods(objects ...interface{}) (ret []interface{}) {
	for _, object := range objects {
		v := reflect.ValueOf(object)
		for i := 0; i < v.NumMethod(); i++ {
			ret = append(ret, v.Method(i).Interface())
		}
	}
	return
}
