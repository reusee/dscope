package dscope

import (
	"fmt"
	"reflect"
	"sync"
)

type InjectStruct func(target any) error

var injectStructTypeID = getTypeID(reflect.TypeFor[InjectStruct]())

func (scope Scope) InjectStruct(target any) error {
	return injectStruct(scope, target)
}

type _InjectStructFunc = func(scope Scope, value reflect.Value) error

// reflect.Type -> _InjectStructFunc
var injectStructFuncs sync.Map

func injectStruct(scope Scope, target any) error {
	v := reflect.ValueOf(target)
	if fn, ok := injectStructFuncs.Load(v.Type()); ok {
		return fn.(_InjectStructFunc)(scope, v)
	}
	injectFunc := makeInjectStructFunc(v.Type())
	fn, _ := injectStructFuncs.LoadOrStore(v.Type(), injectFunc)
	return fn.(_InjectStructFunc)(scope, v)
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
			panic(fmt.Errorf("not injectable: %v", t))
		}
	}

	var fields []reflect.StructField
	for i := range t.NumField() {
		field := t.Field(i)
		directive := field.Tag.Get("dscope")
		if directive == "." || directive == "inject" {
			fields = append(fields, field)
		}
	}

	return func(scope Scope, value reflect.Value) error {
		for range numDeref {
			value = value.Elem()
		}
		for _, field := range fields {
			v, err := scope.Get(field.Type)
			if err != nil {
				return err
			}
			value.FieldByIndex(field.Index).Set(v)
		}
		return nil
	}
}
