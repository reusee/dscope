package dscope

import "reflect"

type DebugInfo struct {
	Values map[reflect.Type]ValueDebugInfo
}

type ValueDebugInfo struct {
	DefTypes []reflect.Type
}

func (s Scope) GetDebugInfo() (info DebugInfo) {
	info.Values = make(map[reflect.Type]ValueDebugInfo)
	if err := s.values.Range(func(values []_Value) error {

		var valueInfo ValueDebugInfo
		for _, value := range values {
			valueInfo.DefTypes = append(valueInfo.DefTypes, value.typeInfo.DefType)
		}

		t := typeIDToType(values[0].typeInfo.TypeID)
		info.Values[t] = valueInfo

		return nil
	}); err != nil {
		panic(err)
	}

	return
}
