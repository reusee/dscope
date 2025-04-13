package dscope

import (
	"iter"
	"reflect"
)

func (s Scope) AllTypes() iter.Seq[reflect.Type] {
	return func(yield func(reflect.Type) bool) {
		for values := range s.values.AllValues() {
			if !yield(typeIDToType(values[0].typeInfo.TypeID)) {
				return
			}
		}
	}
}
