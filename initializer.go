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
	ReducerKind reducerKind
	Values      []reflect.Value
	done        uint32
	mu          sync.Mutex
	err         error
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

func (i *_Initializer) get(scope Scope, id _TypeID) (ret []reflect.Value, err error) {
	if atomic.LoadUint32(&i.done) == 1 {
		// init done
		return i.Values, i.err
	}
	return i.getSlow(scope.appendPath(id))
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

	i.mu.Lock()
	defer i.mu.Unlock()
	if i.done == 1 {
		// init done
		return i.Values, i.err
	}

	defer func() {
		i.Values = ret
		i.err = err
		atomic.StoreUint32(&i.done, 1)
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
			values, err = value.initializer.get(scope, typeID)
			if err != nil { // NOCOVER
				return
			}
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
		return ret, nil
	}

	// non-reducer
	defValue := reflect.ValueOf(i.Def)
	defKind := defValue.Kind()

	switch defKind {

	case reflect.Func:
		var result CallResult
		result = scope.Call(i.Def)
		ret = result.Values

	case reflect.Ptr:
		ret = []reflect.Value{defValue.Elem()}

	}

	return ret, nil
}

func (s *_Initializer) reset() *_Initializer {
	return &_Initializer{
		ID:          s.ID,
		Def:         s.Def,
		ReducerKind: s.ReducerKind,
	}
}
