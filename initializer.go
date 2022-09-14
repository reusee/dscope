package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/reusee/e5"
)

type _Initializer struct {
	ID          int64
	Def         any
	ReducerType *reflect.Type
	Values      []reflect.Value
	Once        sync.Once
	done        atomic.Bool
	err         error
}

func newInitializer(def any, reducerType *reflect.Type) *_Initializer {
	return &_Initializer{
		ID:          atomic.AddInt64(&nextInitializerID, 1),
		Def:         def,
		ReducerType: reducerType,
	}
}

var nextInitializerID int64 = 42

func (i *_Initializer) get(scope Scope) (ret []reflect.Value, err error) {
	if i.done.Load() {
		return i.Values, i.err
	}
	return i.getSlow(scope)
}

func (i *_Initializer) getSlow(scope Scope) (ret []reflect.Value, err error) {

	// detect dependency loop
	for p := scope.path.Prev; p != nil; p = p.Prev {
		if p.TypeID != scope.path.TypeID {
			continue
		}
		return nil, we.With(
			e5.Info("found dependency loop when calling %T", i.Def),
			e5.Info("path: %+v", scope.path),
		)(
			ErrDependencyLoop,
		)
	}

	i.Once.Do(func() {
		defer func() {
			i.Values = ret
			i.err = err
			i.done.Store(true)
		}()

		// reducer
		if i.ReducerType != nil {
			typ := i.Def.(reflect.Type)
			typeID := getTypeID(typ)
			values, ok := scope.values.Load(typeID)
			if !ok { // NOCOVER
				panic("impossible")
			}
			pathScope := scope.appendPath(typeID)
			vs := make([]reflect.Value, len(values))
			for i, value := range values {
				var values []reflect.Value
				values, err = value.get(pathScope)
				if err != nil { // NOCOVER
					return
				}
				vs[i] = values[value.Position]
			}
			switch *i.ReducerType {
			case reducerType:
				ret = []reflect.Value{
					Reduce(vs),
				}
			case customReducerType:
				ret = []reflect.Value{
					vs[0].Interface().(CustomReducer).Reduce(scope, vs),
				}
			}
			return
		}

		// non-reducer
		defValue := reflect.ValueOf(i.Def)
		defKind := defValue.Kind()

		if defKind == reflect.Func {
			var result CallResult
			result = scope.Call(i.Def)
			ret = result.Values

		} else if defKind == reflect.Ptr {
			ret = []reflect.Value{defValue.Elem()}
		}

	})

	return i.Values, i.err
}

func (s *_Initializer) reset() *_Initializer {
	return &_Initializer{
		ID:          s.ID,
		Def:         s.Def,
		ReducerType: s.ReducerType,
	}
}
