package dscope

import (
	"fmt"
	"reflect"
)

var (
	pt = fmt.Printf
)

func Methods(o interface{}) (ret []interface{}) {
	v := reflect.ValueOf(o)
	for i := 0; i < v.NumMethod(); i++ {
		ret = append(ret, v.Method(i))
	}
	return
}
