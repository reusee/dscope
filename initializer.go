package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/reusee/e5"
)

type _Initializer struct {
	Def         any
	Values      []reflect.Value
	ID          int64
	once        sync.Once
	ReducerKind reducerKind
}

type reducerKind uint8

const (
	notReducer reducerKind = iota
	isReducer
	isCustomReducer
)

func newInitializer(def any, reducerKind reducerKind) *_Initializer {
	return &_Initializer{
		ID:          atomic.AddInt64(&nextInitializerID, 1),
		Def:         def,
		ReducerKind: reducerKind,
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

	// reducer
	if i.ReducerKind != notReducer {
		typ := i.Def.(reflect.Type)
		typeID := getTypeID(typ)
		values, ok := scope.values.Load(typeID)
		if !ok { // NOCOVER
			panic("impossible")
		}
		vs := make([]reflect.Value, len(values))
		for i, value := range values {
			var values []reflect.Value
			values = value.initializer.get(scope, typeID)
			vs[i] = values[value.typeInfo.Position]
		}
		switch i.ReducerKind {
		case isReducer:
			ret = []reflect.Value{
				Reduce(vs),
			}
		case isCustomReducer:
			ret = []reflect.Value{
				vs[0].Interface().(CustomReducer).Reduce(scope, vs),
			}
		}
		return ret
	}

	// non-reducer
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
		ID:          s.ID,
		Def:         s.Def,
		ReducerKind: s.ReducerKind,
	}
}
