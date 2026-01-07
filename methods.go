package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

func Methods(objects ...any) (ret []any) {
	visitedTypes := make(map[reflect.Type]bool)
	var extend func(reflect.Value)
	extend = func(v reflect.Value) {
		if !v.IsValid() {
			panic(errors.Join(
				fmt.Errorf("invalid value"),
				ErrBadArgument,
			))
		}

		// nil interface
		if v.Kind() == reflect.Interface && v.IsNil() {
			panic(errors.Join(
				fmt.Errorf("invalid value: nil interface %v", v.Type()),
				ErrBadArgument,
			))
		}

		t := v.Type()
		if visitedTypes[t] {
			return
		}
		visitedTypes[t] = true

		if t.Kind() == reflect.Pointer && v.IsNil() {
			// If it's a pointer to an interface that's nil, we can't construct.
			// e.g. var ptr *io.Reader; Methods(ptr)
			if t.Elem().Kind() == reflect.Interface {
				panic(errors.Join(
					fmt.Errorf("invalid value: nil pointer to interface %v", t),
					ErrBadArgument,
				))
			}
			// Construct concrete object for typed nil pointers like (*MyStruct)(nil)
			v = reflect.New(t.Elem())
		}

		// method sets
		for i := range v.NumMethod() {
			ret = append(ret, v.Method(i).Interface())
		}

		// from fields
		for t.Kind() == reflect.Pointer {
			// deref
			t = t.Elem()
			if v.IsNil() {
				// Allocate new instance for nil pointers to avoid panic on Elem()
				v = reflect.New(t).Elem()
			} else {
				v = v.Elem()
			}

			if t.Kind() == reflect.Pointer {
				// Collect methods from intermediate pointers (e.g. *T when we started with **T)
				for i := range v.NumMethod() {
					ret = append(ret, v.Method(i).Interface())
				}
			}
		}
		if t.Kind() == reflect.Struct {
			for i := range t.NumField() {
				field := t.Field(i)
				if field.PkgPath != "" {
					continue
				}
				if field.Type.Implements(isModuleType) {
					fv := v.Field(i)
					if fv.Kind() == reflect.Struct {
						if fv.CanAddr() {
							extend(fv.Addr())
						} else {
							// For non-addressable struct fields (e.g. when the parent is passed by value),
							// we create an addressable copy to ensure methods with pointer receivers are found.
							ptr := reflect.New(fv.Type())
							ptr.Elem().Set(fv)
							extend(ptr)
						}
					} else {
						extend(fv)
					}
				}
			}
		}

	}

	for _, object := range objects {
		v := reflect.ValueOf(object)
		if v.IsValid() && v.Kind() == reflect.Struct {
			// For root structs passed by value, create an addressable copy
			// to ensure pointer receiver methods are discovered.
			ptr := reflect.New(v.Type())
			ptr.Elem().Set(v)
			v = ptr
		}
		extend(v)
	}

	return
}