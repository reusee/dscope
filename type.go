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
	// Optimistically store the reverse mapping first.
	// This ensures that if another goroutine successfully loads 'id' from typeToID,
	// the corresponding 't' is guaranteed to be present in idToType.
	idToType.Store(id, t)

	v, loaded := typeToID.LoadOrStore(t, id)
	if loaded {
		// We lost the race; the type was already inserted by another goroutine.
		// Clean up our unused optimistic entry.
		idToType.Delete(id)
		return v.(_TypeID)
	} else {
		// We won the race; the mapping is now canonical.
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

func isAlwaysProvided(id _TypeID) bool {
	switch id {
	case injectStructTypeID:
		return true
	}
	return false
}
