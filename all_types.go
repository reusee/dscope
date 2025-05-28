package dscope

import (
	"iter"
	"reflect"
)

func (s Scope) AllTypes() iter.Seq[reflect.Type] {
	return func(yield func(reflect.Type) bool) {
		for value := range s.values.IterValues() {
			if !yield(typeIDToType(value.typeInfo.TypeID)) {
				return
			}
		}
	}
}
