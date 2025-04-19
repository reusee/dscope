package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/reusee/e5"
)

type _Initializer struct {
	Def    any
	Values []reflect.Value
	ID     int64
	once   sync.Once
}

func newInitializer(def any) *_Initializer {
	return &_Initializer{
		ID:  atomic.AddInt64(&nextInitializerID, 1),
		Def: def,
	}
}

var nextInitializerID int64 = 42

func (i *_Initializer) get(scope Scope, id _TypeID) (ret []reflect.Value) {
	i.once.Do(func() {
		i.Values = i.initialize(scope.appendPath(id))
	})
	return i.Values
}

func (i *_Initializer) initialize(scope Scope) (ret []reflect.Value) {
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

	defer func() {
		i.Values = ret
	}()

	defValue := reflect.ValueOf(i.Def)
	defKind := defValue.Kind()

	switch defKind {

	case reflect.Func:
		var result CallResult
		result = scope.CallValue(defValue)
		ret = result.Values

	case reflect.Pointer:
		ret = []reflect.Value{defValue.Elem()}

	}

	return ret
}

// reset make the initializer re-evaluate Values
func (s *_Initializer) reset() *_Initializer {
	return &_Initializer{
		// these fields recognize the provided type and def to get the values, so not changing
		ID:  s.ID,
		Def: s.Def,
	}
}
