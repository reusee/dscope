package dscope

import (
	"iter"
	"reflect"
)

func (s Scope) AllTypes() iter.Seq[reflect.Type] {
	return func(yield func(reflect.Type) bool) {
		if !yield(reflect.TypeFor[InjectStruct]()) {
			return
		}
		if !yield(reflect.TypeFor[Fork]()) {
			return
		}
		for value := range s.values.IterValues() {
			// Always-provided types (e.g. InjectStruct, Fork) are emitted above as
			// built-ins. They may also appear in s.values if a user provides a
			// definition for them, but Scope.get ignores such definitions and
			// always returns the built-in. Skip them here to avoid yielding the
			// same type more than once.
			if isAlwaysProvided(value.typeInfo.TypeID) {
				continue
			}
			if !yield(typeIDToType(value.typeInfo.TypeID)) {
				return
			}
		}
	}
}
