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

// _TypeIDCacheEntry is one confirmed type-to-ID mapping, padded to a 64-byte
// cache line so it never shares a cache line with unrelated 32-byte heap allocations.
type _TypeIDCacheEntry struct {
	typ  reflect.Type
	id   _TypeID
	slot int
	_    [32]byte
}

// typeIDCache accelerates getTypeID for the types a program resolves
// repeatedly. Entries are immutable and published atomically, so a lookup takes
// no lock. The cache never decides an identifier: every hit is confirmed by
// comparing the type itself, so a slot collision costs a miss and nothing more.
var typeIDCache [typeIDCacheSize]atomic.Pointer[_TypeIDCacheEntry]

// typeIDLastEntry remembers the mapping the slot cache confirmed most
// recently. The hot pattern is a program resolving the same type over
// and over, for example a loop that reads one value from a scope; the single
// entry serves that pattern without computing a slot. Like every cache entry
// it is confirmed by comparing the type itself, so a stale entry costs a miss
// and nothing more. Resolving a type through a slot republishes it, and a
// layer memo promotes a confirmed entry the same way.
var typeIDLastEntry atomic.Pointer[_TypeIDCacheEntry]

var (
	typeIDPrevEntry   atomic.Pointer[_TypeIDCacheEntry]
	typeSlotLastEntry atomic.Pointer[_TypeIDCacheEntry]
	idToTypeMu        sync.Mutex
	idToTypeSlice     atomic.Pointer[[]reflect.Type]
	idToEntrySlice    atomic.Pointer[[]*_TypeIDCacheEntry]
)

func init() {
	nextTypeID.Store(3)
	registerBuiltinType(forkTypeID, reflect.TypeFor[Fork]())
	registerBuiltinType(injectStructTypeID, reflect.TypeFor[InjectStruct]())
	registerBuiltinType(resetTypeID, reflect.TypeFor[Reset]())
}

func registerBuiltinType(id _TypeID, t reflect.Type) {
	idToType.Store(id, t)
	typeToID.Store(t, id)
	slot, _ := typeIDSlot(t)
	entry := &_TypeIDCacheEntry{typ: t, id: id, slot: slot}
	typeIDCache[slot].Store(entry)
	recordTypeIDEntry(id, entry)
	idToTypeMu.Lock()
	var oldSlice []reflect.Type
	if p := idToTypeSlice.Load(); p != nil {
		oldSlice = *p
	}
	newLen := max(int(id)+1, max(64, len(oldSlice)))
	newSlice := make([]reflect.Type, newLen)
	copy(newSlice, oldSlice)
	newSlice[id] = t
	idToTypeSlice.Store(&newSlice)
	idToTypeMu.Unlock()
}

func getTypeID(t reflect.Type) _TypeID {
	if entry := typeIDLastEntry.Load(); entry != nil && entry.typ == t {
		return entry.id
	}
	return getTypeIDCached(t)
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
	}
	idToTypeMu.Lock()
	var oldSlice []reflect.Type
	if p := idToTypeSlice.Load(); p != nil {
		oldSlice = *p
	}
	newLen := len(oldSlice)
	if int(id)+1 > newLen {
		newLen = int(id) + 1
	}
	if newLen < 64 {
		newLen = 64
	} else if int(id)+1 > len(oldSlice) {
		newLen = max(newLen, len(oldSlice)*2)
	}
	newSlice := make([]reflect.Type, newLen)
	copy(newSlice, oldSlice)
	newSlice[id] = t
	idToTypeSlice.Store(&newSlice)
	idToTypeMu.Unlock()
	return id
}

// getTypeIDCached resolves a type through the slot cache and the authoritative
// identifier table. The most recent entry did not match when this function runs.
func getTypeIDCached(t reflect.Type) _TypeID {
	if entry := typeIDPrevEntry.Load(); entry != nil && entry.typ == t {
		return entry.id
	}
	slot, ok := typeIDSlot(t)
	if !ok {
		return getTypeIDUncached(t)
	}
	if entry := typeIDCache[slot].Load(); entry != nil && entry.typ == t {
		if prev := typeIDLastEntry.Load(); prev != nil && prev.typ != t {
			typeIDPrevEntry.Store(prev)
		}
		typeIDLastEntry.Store(entry)
		return entry.id
	}
	// Both caches missed: resolve through the authoritative identifier table, then
	// publish the mapping in the slot cache and as the most recent one. The
	// identifier is still decided exactly once per type.
	id := getTypeIDUncached(t)
	entry := &_TypeIDCacheEntry{typ: t, id: id, slot: slot}
	typeIDCache[slot].Store(entry)
	recordTypeIDEntry(id, entry)
	if prev := typeIDLastEntry.Load(); prev != nil && prev.typ != t {
		typeIDPrevEntry.Store(prev)
	}
	typeIDLastEntry.Store(entry)
	return id
}

func getTypeIDForGet(t reflect.Type) _TypeID {
	if entry := typeIDLastEntry.Load(); entry != nil && entry.typ == t {
		return entry.id
	}
	return getTypeIDForGetSlow(t)
}

func getTypeIDForGetSlow(t reflect.Type) _TypeID {
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return getTypeIDUncached(t)
	}
	slot := int((v.Pointer() >> 4) & (typeIDCacheSize - 1))
	if entry := typeIDCache[slot].Load(); entry != nil && entry.typ == t {
		return entry.id
	}
	return getTypeIDCached(t)
}

func recordTypeIDEntry(id _TypeID, entry *_TypeIDCacheEntry) {
	idToTypeMu.Lock()
	var oldSlice []*_TypeIDCacheEntry
	if p := idToEntrySlice.Load(); p != nil {
		oldSlice = *p
	}
	newLen := len(oldSlice)
	if int(id)+1 > newLen {
		newLen = max(int(id)+1, max(64, len(oldSlice)*2))
	}
	newSlice := make([]*_TypeIDCacheEntry, newLen)
	copy(newSlice, oldSlice)
	newSlice[id] = entry
	idToEntrySlice.Store(&newSlice)
	idToTypeMu.Unlock()
}

func promoteTypeID(id _TypeID) {
	if p := idToEntrySlice.Load(); p != nil {
		if s := *p; uint(id) < uint(len(s)) {
			if e := s[id]; e != nil && typeIDLastEntry.Load() != e {
				typeIDLastEntry.Store(e)
			}
		}
	}
}

func typeIDSlot(t reflect.Type) (int, bool) {
	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0, false
	}
	return int((v.Pointer() >> 4) & (typeIDCacheSize - 1)), true
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
	if entry := typeSlotLastEntry.Load(); entry != nil && entry.typ == t {
		return entry.slot, true
	}
	slot, ok := typeIDSlot(t)
	if ok {
		typeSlotLastEntry.Store(&_TypeIDCacheEntry{typ: t, slot: slot})
	}
	return slot, ok
}

func typeIDToType(id _TypeID) reflect.Type {
	if p := idToTypeSlice.Load(); p != nil {
		if s := *p; uint(id) < uint(len(s)) {
			if t := s[id]; t != nil {
				return t
			}
		}
	}
	v, ok := idToType.Load(id)
	if !ok {
		panic("impossible")
	}
	return v.(reflect.Type)
}

func isAlwaysProvided(id _TypeID) bool {
	return uint(id-1) < 3
}
