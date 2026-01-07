package dscope

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

type InjectStruct func(target any)

var injectStructTypeID = getTypeID(reflect.TypeFor[InjectStruct]())

func (scope Scope) InjectStruct(target any) {
	injectStruct(scope, target, 0)
}

type _InjectStructFunc = func(scope Scope, value reflect.Value, depth int)

// reflect.Type -> _InjectStructFunc
var injectStructFuncs sync.Map

func injectStruct(scope Scope, target any, depth int) {
	v := reflect.ValueOf(target)
	if !v.IsValid() {
		panic(errors.Join(
			fmt.Errorf("target must be a pointer to a struct, got nil"),
			ErrBadArgument,
		))
	}
	if v.Kind() != reflect.Pointer {
		panic(errors.Join(
			fmt.Errorf("target must be a pointer to a struct, got %v", v.Type()),
			ErrBadArgument,
		))
	}
	targetType := v.Type()
	if fn, ok := injectStructFuncs.Load(targetType); ok {
		fn.(_InjectStructFunc)(scope, v, depth)
		return
	}
	injectFunc := makeInjectStructFunc(targetType)
	fn, _ := injectStructFuncs.LoadOrStore(targetType, injectFunc)
	fn.(_InjectStructFunc)(scope, v, depth)
}

func makeInjectStructFunc(t reflect.Type) _InjectStructFunc {
	numDeref := 0
l:
	for {
		if numDeref > 100 {
			panic(errors.Join(
				fmt.Errorf("too many dereferences or recursive pointer type %v", t),
				ErrBadArgument,
			))
		}
		switch t.Kind() {
		case reflect.Pointer:
			numDeref++
			// This may causes stack overflow for recursive pointer types.
			// Fixing this will introduce extra cost for common cases.
			// So we choose not to handle recursive pointer types, just let it crash.
			t = t.Elem()
		case reflect.Struct:
			break l
		default:
			panic(errors.Join(
				fmt.Errorf("target type %v is not a struct or pointer to struct", t),
				ErrBadArgument,
			))
		}
	}

	type FieldInfo struct {
		Field      reflect.StructField
		IsInject   bool
		IsEmbedded bool
		Type       reflect.Type
	}
	var infos []FieldInfo
	for i := range t.NumField() {
		field := t.Field(i)

		if field.PkgPath != "" {
			// un-exported field
			continue
		}

		directive := field.Tag.Get("dscope")
		if field.Type.Kind() == reflect.Func && field.Type.Implements(isInjectType) {
			// Only treat as Inject[T] if it is a function.
			// Pointers to Inject[T] also implement the interface but cannot be
			// processed by reflect.MakeFunc or Out(0).
			infos = append(infos, FieldInfo{
				Field:    field,
				IsInject: true,
				Type:     field.Type.Out(0),
			})

		} else if directive == "." || directive == "inject" {
			infos = append(infos, FieldInfo{
				Field: field,
				Type:  field.Type,
			})

		} else if field.Anonymous {
			fieldType := field.Type
			if fieldType.Kind() == reflect.Pointer {
				if fieldType.Elem().Kind() != reflect.Struct {
					continue
				}
			} else if fieldType.Kind() != reflect.Struct {
				continue
			}
			infos = append(infos, FieldInfo{
				Field:      field,
				IsEmbedded: true,
				Type:       field.Type,
			})

		}

	}

	return func(scope Scope, value reflect.Value, depth int) {
		if depth > 64 {
			panic(errors.Join(
				fmt.Errorf("recursive struct injection depth limit exceeded"),
				ErrBadArgument,
			))
		}

		// Check if the target pointer is nil before dereferencing
		if value.Kind() == reflect.Pointer && value.IsNil() {
			panic(errors.Join(
				fmt.Errorf("cannot inject into a nil pointer target of type %v", value.Type()),
				ErrBadArgument,
			))
		}

		for range numDeref {
			if value.IsNil() {
				if value.CanSet() {
					value.Set(reflect.New(value.Type().Elem()))
				} else {
					panic(errors.Join(
						fmt.Errorf("cannot inject into a nil pointer target of type %v", value.Type()),
						ErrBadArgument,
					))
				}
			}
			value = value.Elem()
		}
		for _, info := range infos {
			info := info

			if info.IsInject {
				value.FieldByIndex(info.Field.Index).Set(
					reflect.MakeFunc(
						info.Field.Type,
						func(_ []reflect.Value) []reflect.Value {
							value, ok := scope.Get(info.Type)
							if !ok {
								throwErrDependencyNotFound(info.Type)
							}
							return []reflect.Value{value}
						},
					),
				)

			} else if info.IsEmbedded {
				fieldValue := value.FieldByIndex(info.Field.Index)
				if fieldValue.Type().Kind() == reflect.Pointer {
					if fieldValue.IsNil() {
						if fieldValue.CanSet() {
							fieldValue.Set(reflect.New(fieldValue.Type().Elem()))
						} else {
							continue
						}
					}
					injectStruct(scope, fieldValue.Interface(), depth+1)
				} else { // Embedded by value
					if fieldValue.CanAddr() {
						injectStruct(scope, fieldValue.Addr().Interface(), depth+1)
					}
				}

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