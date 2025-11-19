package dscope

import (
	"errors"
	"fmt"
	"reflect"
)

// Methods extracts all exported methods from the provided objects and any
// It recursively traverses struct fields marked for extension to collect their methods as well.
// This prevents infinite loops by keeping track of visited types.
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
			v = v.Elem()
		}
		if t.Kind() == reflect.Struct {
			for i := range t.NumField() {
				field := t.Field(i)
				if field.PkgPath != "" {
					continue
				}
				if field.Type.Implements(isModuleType) {
					fv := v.Field(i)
					if fv.Kind() == reflect.Struct && fv.CanAddr() {
						extend(fv.Addr())
					} else {
						extend(fv)
					}
				}
			}
		}

	}

	for _, object := range objects {
		extend(reflect.ValueOf(object))
	}

	return
}
