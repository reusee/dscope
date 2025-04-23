package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/reusee/e5"
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

func (i *_Initializer) get(scope Scope, id _TypeID, position int) (ret reflect.Value) {
	i.once.Do(func() {
		if i.DefIsPointer {
			i.initializePointer()
		} else {
			i.initializeFunction(scope.appendPath(id))
		}
	})
	return i.Values[position]
}

func (i *_Initializer) initializePointer() {
	// pointer provider does not introduce loops
	i.Values = []reflect.Value{
		reflect.ValueOf(i.Def).Elem(),
	}
}

func (i *_Initializer) initializeFunction(scope Scope) {
	// detect dependency loop
	for p := scope.path.Prev; p != nil; p = p.Prev {
		if p.TypeID != scope.path.TypeID {
			continue
		}
		panic(we.With(
			e5.Info("found dependency loop when calling %T", i.Def),
			e5.Info("path: %+v", scope.path),
		)(
			ErrDependencyLoop,
		))
	}

	i.Values = scope.CallValue(reflect.ValueOf(i.Def)).Values
}
