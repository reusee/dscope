package dscope

import (
	"reflect"
)

func Methods(objects ...any) (ret []any) {
	for _, object := range objects {
		v := reflect.ValueOf(object)
		t := v.Type()
		typeName := t.String()
		for i := 0; i < v.NumMethod(); i++ {
			ret = append(ret, NamedDef{
				Name: typeName + "." + t.Method(i).Name,
				Def:  v.Method(i).Interface(),
			})
		}
	}
	return
}
