package dscope

import "reflect"

var FastCalls = map[reflect.Type]func(Scope, reflect.Value, []interface{}) []reflect.Value{
	// func()
	reflect.TypeOf((*func())(nil)).Elem(): func(_ Scope, v reflect.Value, _ []interface{}) []reflect.Value {
		v.Interface().(func())()
		return nil
	},

	// func() error
	reflect.TypeOf((*func() error)(nil)).Elem(): func(_ Scope, v reflect.Value, rets []interface{}) []reflect.Value {
		err := v.Interface().(func() error)()
		for _, ret := range rets {
			if p, ok := ret.(*error); ok {
				*p = err
			}
		}
		return []reflect.Value{
			reflect.ValueOf(err),
		}
	},

	// func() float64
	reflect.TypeOf((*func() float64)(nil)).Elem(): func(_ Scope, v reflect.Value, rets []interface{}) []reflect.Value {
		f := v.Interface().(func() float64)()
		for _, ret := range rets {
			if p, ok := ret.(*float64); ok {
				*p = f
			}
		}
		return []reflect.Value{
			reflect.ValueOf(f),
		}
	},

	// func() int
	reflect.TypeOf((*func() int)(nil)).Elem(): func(_ Scope, v reflect.Value, rets []interface{}) []reflect.Value {
		i := v.Interface().(func() int)()
		for _, ret := range rets {
			if p, ok := ret.(*int); ok {
				*p = i
			}
		}
		return []reflect.Value{
			reflect.ValueOf(i),
		}
	},

	// func() string
	reflect.TypeOf((*func() string)(nil)).Elem(): func(_ Scope, v reflect.Value, rets []interface{}) []reflect.Value {
		s := v.Interface().(func() string)()
		for _, ret := range rets {
			if p, ok := ret.(*string); ok {
				*p = s
			}
		}
		return []reflect.Value{
			reflect.ValueOf(s),
		}
	},
}
