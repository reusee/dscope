package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/reusee/e4"
)

type _Initializer struct {
	ID          int64
	Def         any
	Name        string
	ReducerType *reflect.Type
	Once        sync.Once
	Values      []reflect.Value
}

func newInitializer(def any, name string, reducerType *reflect.Type) *_Initializer {
	return &_Initializer{
		ID:          atomic.AddInt64(&nextInitializerID, 1),
		Def:         def,
		Name:        name,
		ReducerType: reducerType,
	}
}

var nextInitializerID int64 = 42

func (i *_Initializer) Get(scope Scope) (ret []reflect.Value, err error) {
	// detect dependency loop
	for p := scope.path.Prev; p != nil; p = p.Prev {
		if p.Type != scope.path.Type {
			continue
		}
		return nil, we.With(
			e4.Info("found dependency loop when calling %T / %s",
				i.Def, i.Name),
			e4.Info("path: %+v", scope.path),
		)(
			ErrDependencyLoop,
		)
	}

	i.Once.Do(func() {

		// reducer
		if i.ReducerType != nil {
			typ := i.Def.(reflect.Type)
			typeID := getTypeID(typ)
			values, ok := scope.values.Load(typeID)
			if !ok { // NOCOVER
				panic("impossible")
			}
			pathScope := scope.appendPath(typ)
			vs := make([]reflect.Value, len(values))
			names := make(DefNames, len(values))
			for i, value := range values {
				var values []reflect.Value
				values, err = value.Initializer.Get(pathScope)
				if err != nil { // NOCOVER
					return
				}
				if value.Position >= len(values) { // NOCOVER
					panic("impossible")
				}
				vs[i] = values[value.Position]
				names[i] = value.DefName
			}
			scope = scope.Fork(&names)
			switch *i.ReducerType {
			case reducerType:
				i.Values = []reflect.Value{
					Reduce(vs),
				}
			case customReducerType:
				i.Values = []reflect.Value{
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
			func() {
				defer he(&err, e4.WrapFunc(func(err error) error {
					// use a closure to avoid calling getName eagerly
					return e4.NewInfo("dscope: call %s",
						getName(i.Name, i.Def, false))(err)
				}))
				defer func() {
					if p := recover(); p != nil {
						pt("func: %s\n",
							getName(i.Name, i.Def, false))
						panic(p)
					}
				}()
				result = scope.Call(i.Def)
			}()
			i.Values = result.Values

		} else if defKind == reflect.Ptr {
			i.Values = []reflect.Value{defValue.Elem()}
		}

	})

	return i.Values, err
}
