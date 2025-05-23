package dscope

import (
	"reflect"
	"sync"

	"github.com/reusee/e5"
)

type InjectStruct func(target any)

var injectStructTypeID = getTypeID(reflect.TypeFor[InjectStruct]())

func (scope Scope) InjectStruct(target any) {
	injectStruct(scope, target)
}

type _InjectStructFunc = func(scope Scope, value reflect.Value)

// reflect.Type -> _InjectStructFunc
var injectStructFuncs sync.Map

func injectStruct(scope Scope, target any) {
	v := reflect.ValueOf(target)
	targetType := v.Type()
	if fn, ok := injectStructFuncs.Load(targetType); ok {
		fn.(_InjectStructFunc)(scope, v)
		return
	}
	injectFunc := makeInjectStructFunc(targetType)
	fn, _ := injectStructFuncs.LoadOrStore(targetType, injectFunc)
	fn.(_InjectStructFunc)(scope, v)
}

func makeInjectStructFunc(t reflect.Type) _InjectStructFunc {
	numDeref := 0
l:
	for {
		switch t.Kind() {
		case reflect.Pointer:
			numDeref++
			t = t.Elem()
		case reflect.Struct:
			break l
		default:
			_ = throw(we.With(
				e5.Info("target type %v is not a struct or pointer to struct", t),
			)(
				ErrBadArgument,
			))
		}
	}

	type FieldInfo struct {
		Field    reflect.StructField
		IsInject bool
		Type     reflect.Type
	}
	var infos []FieldInfo
	for i := range t.NumField() {
		field := t.Field(i)
		directive := field.Tag.Get("dscope")
		if directive == "." || directive == "inject" {
			infos = append(infos, FieldInfo{
				Field: field,
				Type:  field.Type,
			})
		} else if field.Type.Implements(isInjectType) {
			infos = append(infos, FieldInfo{
				Field:    field,
				IsInject: true,
				Type:     field.Type.Out(0),
			})
		}
	}

	return func(scope Scope, value reflect.Value) {
		for range numDeref {
			value = value.Elem()
		}
		for _, info := range infos {
			info := info

			if info.IsInject {
				var ret []reflect.Value
				var once sync.Once
				value.FieldByIndex(info.Field.Index).Set(
					reflect.MakeFunc(
						info.Field.Type,
						func(_ []reflect.Value) []reflect.Value {
							once.Do(func() {
								value, ok := scope.Get(info.Type)
								if !ok {
									throwErrDependencyNotFound(info.Type)
								}
								ret = []reflect.Value{value}
							})
							return ret
						},
					),
				)

			} else {
				v, ok := scope.Get(info.Type)
				if !ok {
					throwErrDependencyNotFound(info.Type)
				}
				value.FieldByIndex(info.Field.Index).Set(v)
			}

		}
	}
}
