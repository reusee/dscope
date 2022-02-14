package dscope

import (
	"reflect"
	"sync"
	"sync/atomic"
)

type _TypeIDInfos struct {
	TypeToID map[reflect.Type]_TypeID
	IDToType map[_TypeID]reflect.Type
}

var (
	typeIDInfos = func() atomic.Value {
		infos := &_TypeIDInfos{
			TypeToID: make(map[reflect.Type]_TypeID),
			IDToType: make(map[_TypeID]reflect.Type),
		}
		var v atomic.Value
		v.Store(infos)
		return v
	}()
	typeIDLock sync.Mutex
	nextTypeID _TypeID
)

func getTypeID(t reflect.Type) _TypeID {
	if i, ok := typeIDInfos.Load().(*_TypeIDInfos).TypeToID[t]; ok {
		return i
	}
	return getTypeIDSlow(t)
}

func getTypeIDSlow(t reflect.Type) _TypeID {
	typeIDLock.Lock()
	infos := typeIDInfos.Load().(*_TypeIDInfos)
	if i, ok := infos.TypeToID[t]; ok { // NOCOVER
		typeIDLock.Unlock()
		return i
	}
	newTypeToID := make(map[reflect.Type]_TypeID, len(infos.TypeToID)+1)
	for k, v := range infos.TypeToID {
		newTypeToID[k] = v
	}
	newIDToType := make(map[_TypeID]reflect.Type, len(infos.IDToType)+1)
	for k, v := range infos.IDToType {
		newIDToType[k] = v
	}
	nextTypeID++
	id := _TypeID(nextTypeID)
	newTypeToID[t] = id
	newIDToType[id] = t
	typeIDInfos.Store(&_TypeIDInfos{
		TypeToID: newTypeToID,
		IDToType: newIDToType,
	})
	typeIDLock.Unlock()
	return id
}
