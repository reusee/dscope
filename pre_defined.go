package dscope

import "reflect"

func (s Scope) getPredefined(
	t reflect.Type,
) (_ reflect.Value) {

	switch t {

	case subType:
		return reflect.ValueOf(s.Sub)

	case assignType:
		return reflect.ValueOf(s.Assign)

	case getType:
		return reflect.ValueOf(s.Get)

	case callType:
		return reflect.ValueOf(s.Call)

	case callValueType:
		return reflect.ValueOf(s.CallValue)

	}

	return

}
