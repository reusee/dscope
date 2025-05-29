package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"
)

type _Initializer struct {
	Def          any
	DefIsPointer bool
	Values       []reflect.Value
	ID           int64
	once         sync.Once // only for function definitions
}

func newInitializer(def any, isPointer bool) *_Initializer {
	ret := &_Initializer{
		ID:           atomic.AddInt64(&nextInitializerID, 1),
		Def:          def,
		DefIsPointer: isPointer,
	}
	if isPointer {
		ret.Values = []reflect.Value{
			reflect.ValueOf(
				// make a copy
				reflect.ValueOf(def).Elem().Interface(),
			),
		}
	}
	return ret
}

// reset make the initializer re-evaluate Values
func (s *_Initializer) reset() *_Initializer {
	if s.DefIsPointer {
		// no need to re-evaluate
		return s
	}
	return &_Initializer{
		// these fields recognize the provided type and def to get the values, so not changing
		ID:           s.ID,
		Def:          s.Def,
		DefIsPointer: s.DefIsPointer,
	}
}

var nextInitializerID int64 = 42

func (i *_Initializer) get(scope Scope, position int) (ret reflect.Value) {
	if !i.DefIsPointer {
		i.once.Do(func() {
			i.Values = scope.CallValue(reflect.ValueOf(i.Def)).Values
		})
	}
	return i.Values[position]
}
