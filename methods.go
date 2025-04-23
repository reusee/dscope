package dscope

import "reflect"

// Methods extracts all exported methods from the provided objects and any
// fields tagged with `dscope:"."` or `dscope:"methods"`.
// It recursively traverses struct fields marked for extension to collect their methods as well.
// This prevents infinite loops by keeping track of visited types.
func Methods(objects ...any) (ret []any) {
	visitedTypes := make(map[reflect.Type]bool)
	var extend func(reflect.Value)
	extend = func(v reflect.Value) {
		t := v.Type()
		if visitedTypes[t] {
			return
		}
		visitedTypes[t] = true

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
				directive := field.Tag.Get("dscope")
				if directive == "." ||
					directive == "methods" ||
					field.Type.Implements(isModuleType) {
					extend(v.Field(i))
				}
			}
		}

	}

	for _, object := range objects {
		extend(reflect.ValueOf(object))
	}

	return
}
