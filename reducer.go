package dscope

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/reusee/e5"
)

// CustomReducer defines an interface for types that provide their own logic
// for combining multiple values of the same type into a single value.
type CustomReducer interface {
	// Reduce takes the current scope and a slice of values to be reduced,
	// returning the single combined value.
	Reduce(Scope, []reflect.Value) reflect.Value
}

var customReducerType = reflect.TypeFor[CustomReducer]()

// Reducer is a marker interface for types that should be combined using
// the default reduction logic provided by the Reduce function.
type Reducer interface {
	IsReducer()
}

var reducerType = reflect.TypeFor[Reducer]()

// getReducerKind checks if a type implements Reducer or CustomReducer.
func getReducerKind(t reflect.Type) reducerKind {
	if t.Implements(reducerType) {
		return isReducer
	}
	if t.Implements(customReducerType) {
		return isCustomReducer
	}
	return notReducer
}

// reducerMark is an internal marker type used in generated function types
// to uniquely identify the storage location for reduced values.
type reducerMark struct{}

var reducerMarkType = reflect.TypeFor[reducerMark]()

// reducerMarkTypes caches generated reducer marker function types.
// Key: _TypeID of the original reducer type.
// Value: reflect.Type of the generated function type func(reducerMark) T.
// _TypeID -> reflect.Type
var reducerMarkTypes sync.Map

// getReducerMarkType generates or retrieves a unique function type based on the
// original reducer type `t`. This marker type (func(reducerMark) T) is used
// internally to store the result of a reduction for type `T`.
func getReducerMarkType(id _TypeID) reflect.Type {
	if v, ok := reducerMarkTypes.Load(id); ok {
		return v.(reflect.Type)
	}
	// Generate func(reducerMark) T
	markType := reflect.FuncOf(
		[]reflect.Type{
			reducerMarkType,
		},
		[]reflect.Type{
			typeIDToType(id),
		},
		false,
	)
	v, _ := reducerMarkTypes.LoadOrStore(id, markType)
	return v.(reflect.Type)
}

var ErrNoValues = errors.New("no values")

// Reduce provides default logic for combining multiple values (`vs`) of the same type
// into a single value. It supports slices (concatenation), strings (concatenation),
// maps (merging), ints (summing), and functions (calling all functions sequentially).
// Panics if `vs` is empty or if the type is not supported.
func Reduce(vs []reflect.Value) reflect.Value {
	if len(vs) == 0 {
		panic(ErrNoValues)
	}

	t := vs[0].Type()
	ret := reflect.New(t).Elem()

	switch t.Kind() {

	case reflect.Slice:
		// Concatenate elements from all input slices.
		for _, v := range vs {
			var elems []reflect.Value
			for i := range v.Len() {
				elems = append(elems, v.Index(i))
			}
			ret = reflect.Append(ret, elems...)
		}

	case reflect.String:
		// Concatenate strings.
		var b strings.Builder
		for _, v := range vs {
			b.WriteString(v.String())
		}
		ret = reflect.New(t)
		ret.Elem().SetString(b.String())
		ret = ret.Elem()

	case reflect.Func:
		// Combine functions into one that calls all input functions.
		// Ensure function has no return values.
		if t.NumOut() > 0 {
			throw(we.With(
				e5.Info("function reducers must have no return values"),
			)(ErrBadDefinition))
		}
		ret = reflect.MakeFunc(
			t,
			func(args []reflect.Value) []reflect.Value {
				for _, v := range vs {
					if v.IsNil() {
						continue
					}
					v.Call(args)
				}
				return nil
			},
		)

	case reflect.Map:
		// Merge map entries; later values overwrite earlier ones for the same key.
		ret = reflect.MakeMap(t)
		for _, v := range vs {
			iter := v.MapRange()
			for iter.Next() {
				ret.SetMapIndex(iter.Key(), iter.Value())
			}
		}

	case reflect.Int:
		// Sum integers.
		var i int64
		for _, v := range vs {
			i += v.Int()
		}
		ret = reflect.New(t).Elem()
		ret.SetInt(i)

	default:
		panic(fmt.Errorf("don't know how to reduce %v", t))
	}

	return ret
}
