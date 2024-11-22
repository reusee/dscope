package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"
)

var (
	typeToID sync.Map // reflect.Type -> _TypeID
	idToType sync.Map // _TypeID -> reflect.Type
)

var (
	nextTypeID atomic.Int64
)

func getTypeID(t reflect.Type) _TypeID {
	if v, ok := typeToID.Load(t); ok {
		return v.(_TypeID)
	}
	return getTypeIDSlow(t)
}

func getTypeIDSlow(t reflect.Type) _TypeID {
	id := _TypeID(nextTypeID.Add(1))
	v, loaded := typeToID.LoadOrStore(t, id)
	if loaded {
		// not inserted
		return v.(_TypeID)
	} else {
		// inserted
		idToType.Store(id, t)
		return id
	}
}

func typeIDToType(id _TypeID) reflect.Type {
	v, ok := idToType.Load(id)
	if !ok {
		panic("impossible")
	}
	return v.(reflect.Type)
}
