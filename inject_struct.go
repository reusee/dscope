package dscope

import (
	"reflect"
	"sync"
)

// TheoryOfScopeInjectStruct documents struct field injection: how fields are
// selected and resolved.
const TheoryOfScopeInjectStruct = `
dscope inject theory:
- InjectStruct fills the exported fields of a struct from the scope; it wires
  objects that need many dependencies without forcing every dependency to be
  a constructor parameter.
- A field is injected when it carries the tag dscope:"." or dscope:"inject";
  a field of type Inject[T] receives a lazy function that resolves T on
  demand; an untagged embedded struct is recursed into, allocating nil pointer
  fields along the way.
- Tagged fields are resolved exactly like any other lookup. Unexported fields
  are never touched.
- The target may be a pointer chain ending in a struct; nil pointers along the
  chain are allocated when settable.
`

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
		panic(errWith(ErrBadArgument, "target must be a pointer to a struct, got nil"))
	}
	if v.Kind() != reflect.Pointer {
		panic(errWith(ErrBadArgument, "target must be a pointer to a struct, got %v", v.Type()))
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
			panic(errWith(ErrBadArgument, "too many dereferences or recursive pointer type %v", t))
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
			panic(errWith(ErrBadArgument, "target type %v is not a struct or pointer to struct", t))
		}
	}

	type FieldInfo struct {
		Field      reflect.StructField
		IsInject   bool
		IsEmbedded bool
		Type       reflect.Type
		TypeID     _TypeID
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
			injectedType := field.Type.Out(0)
			infos = append(infos, FieldInfo{
				Field:    field,
				IsInject: true,
				Type:     injectedType,
				TypeID:   getTypeID(injectedType),
			})

		} else if directive == "." || directive == "inject" {
			infos = append(infos, FieldInfo{
				Field:  field,
				Type:   field.Type,
				TypeID: getTypeID(field.Type),
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
			panic(errWith(ErrBadArgument, "recursive struct injection depth limit exceeded"))
		}

		for range numDeref {
			if value.IsNil() {
				if value.CanSet() {
					value.Set(reflect.New(value.Type().Elem()))
				} else {
					panic(errWith(ErrBadArgument, "cannot inject into a nil pointer target of type %v", value.Type()))
				}
			}
			value = value.Elem()
		}
		for _, info := range infos {

			if info.IsInject {
				// The lazy function reads the field data from plain locals: a
				// closure over the loop variable would push a copy of every
				// FieldInfo to the heap on each call.
				fieldType := info.Field.Type
				fieldIndex := info.Field.Index
				injectedType := info.Type
				typeID := info.TypeID
				value.FieldByIndex(fieldIndex).Set(
					reflect.MakeFunc(
						fieldType,
						func(_ []reflect.Value) []reflect.Value {
							v, ok := scope.get(typeID)
							if !ok {
								throwErrDependencyNotFound(injectedType)
							}
							return []reflect.Value{v}
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
				v, ok := scope.get(info.TypeID)
				if !ok {
					throwErrDependencyNotFound(info.Type)
				}
				value.FieldByIndex(info.Field.Index).Set(v)
			}

		}
	}
}
