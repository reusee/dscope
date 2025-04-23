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
	once         sync.Once
}

func newInitializer(def any, isPointer bool) *_Initializer {
	return &_Initializer{
		ID:           atomic.AddInt64(&nextInitializerID, 1),
		Def:          def,
		DefIsPointer: isPointer,
	}
}

// reset make the initializer re-evaluate Values
func (s *_Initializer) reset() *_Initializer {
	return &_Initializer{
		// these fields recognize the provided type and def to get the values, so not changing
		ID:           s.ID,
		Def:          s.Def,
		DefIsPointer: s.DefIsPointer,
	}
}

var nextInitializerID int64 = 42

func (i *_Initializer) get(scope Scope, position int) (ret reflect.Value) {
	i.once.Do(func() {
		if i.DefIsPointer {
			i.Values = []reflect.Value{reflect.ValueOf(i.Def).Elem()}
		} else {
			i.Values = scope.CallValue(reflect.ValueOf(i.Def)).Values
		}
	})
	return i.Values[position]
}
