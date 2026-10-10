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

// typeIDCacheSize is the number of slots of the direct-mapped cache in front of
// the type ID table.
const typeIDCacheSize = 1024

// _TypeIDCacheEntry is one confirmed type-to-ID mapping.
type _TypeIDCacheEntry struct {
	typ reflect.Type
	id  _TypeID
}

// typeIDCache accelerates getTypeID for the types a program resolves
// repeatedly. Entries are immutable and published atomically, so a lookup takes
// no lock. The cache never decides an identifier: every hit is confirmed by
// comparing the type itself, so a slot collision costs a miss and nothing more.
var typeIDCache [typeIDCacheSize]atomic.Pointer[_TypeIDCacheEntry]

func getTypeID(t reflect.Type) _TypeID {
	if slot, ok := typeIDCacheSlot(t); ok {
		if entry := typeIDCache[slot].Load(); entry != nil && entry.typ == t {
			return entry.id
		}
		id := getTypeIDUncached(t)
		typeIDCache[slot].Store(&_TypeIDCacheEntry{typ: t, id: id})
		return id
	}
	return getTypeIDUncached(t)
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

// getTypeIDUncached resolves a type through the authoritative identifier table.
func getTypeIDUncached(t reflect.Type) _TypeID {
	if v, ok := typeToID.Load(t); ok {
		return v.(_TypeID)
	}
	return getTypeIDSlow(t)
}

// typeIDCacheSlot maps a type to a cache slot. The dynamic type of a
// reflect.Type value is a pointer to the runtime type descriptor, so the
// descriptor address spreads types over the slots. A reflect.Type value that is
// not pointer shaped is not cached.
func typeIDCacheSlot(t reflect.Type) (int, bool) {
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0, false
	}
	return int((v.Pointer() >> 4) & (typeIDCacheSize - 1)), true
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
	case forkTypeID:
		return true
	case resetTypeID:
		return true
	}
	return false
}
