package dscope

import (
	"hash/maphash"
	"reflect"
	"sync"
	"sync/atomic"
)

var (
	typeToID sync.Map // reflect.Type -> _TypeID
	idToType sync.Map // _TypeID -> reflect.Type
)

var (
	nextTypeID   atomic.Int64
	typeHashSeed = maphash.MakeSeed()
)

// typeIDCacheSize is the number of slots of the direct-mapped cache in front of
// the type ID table.
const typeIDCacheSize = 2048

const (
	defTypeUnknown uint8 = iota
	defTypeValidFunc
	defTypeValidPtr
	defTypeInvalid
)

type _DefTypeMeta struct {
	values      []_Value
	emptySig    _Hash
	singleFunc1 bool
	isPointer   bool
	nilPtr      any
}

// _TypeIDCacheEntry is one confirmed type-to-ID mapping and per-type metadata
// record.
type _TypeIDCacheEntry struct {
	typ         reflect.Type
	id          _TypeID
	slot        int
	defMeta     atomic.Pointer[_DefTypeMeta]
	getArgsFn   atomic.Pointer[func(Scope, []reflect.Value) int]
	injectFn    atomic.Pointer[_InjectStructFunc]
	defTypeFlag atomic.Uint32
}

var typeIDCache [typeIDCacheSize]atomic.Pointer[_TypeIDCacheEntry]
var typeIDLastEntry atomic.Pointer[_TypeIDCacheEntry]

var (
	typeIDPrevEntry   atomic.Pointer[_TypeIDCacheEntry]
	typeSlotLastEntry atomic.Pointer[_TypeIDCacheEntry]
	idToTypeMu        sync.Mutex
	idToTypeSlice     atomic.Pointer[[]reflect.Type]
	idToEntrySlice    atomic.Pointer[[]*_TypeIDCacheEntry]
)

var (
	forkReflectType         = reflect.TypeFor[Fork]()
	injectStructReflectType = reflect.TypeFor[InjectStruct]()
	resetReflectType        = reflect.TypeFor[Reset]()
)

func init() {
	nextTypeID.Store(3)
	registerBuiltinType(forkTypeID, forkReflectType)
	registerBuiltinType(injectStructTypeID, injectStructReflectType)
	registerBuiltinType(resetTypeID, resetReflectType)
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

func getTypeEntry(t reflect.Type) *_TypeIDCacheEntry {
	if entry := typeIDLastEntry.Load(); entry != nil && entry.typ == t {
		return entry
	}
	return getTypeEntrySlow(t)
}

func getTypeEntrySlow(t reflect.Type) *_TypeIDCacheEntry {
	if entry := typeIDPrevEntry.Load(); entry != nil && entry.typ == t {
		return entry
	}
	slot, ok := typeIDSlot(t)
	if !ok {
		id := getTypeIDUncached(t)
		return &_TypeIDCacheEntry{typ: t, id: id}
	}
	if entry := typeIDCache[slot].Load(); entry != nil && entry.typ == t {
		if prev := typeIDLastEntry.Load(); prev != nil && prev.typ != t {
			typeIDPrevEntry.Store(prev)
		}
		typeIDLastEntry.Store(entry)
		return entry
	}
	id := getTypeIDUncached(t)
	if p := idToEntrySlice.Load(); p != nil {
		if s := *p; uint(id) < uint(len(s)) {
			if existing := s[id]; existing != nil {
				typeIDCache[slot].Store(existing)
				if prev := typeIDLastEntry.Load(); prev != nil && prev.typ != t {
					typeIDPrevEntry.Store(prev)
				}
				typeIDLastEntry.Store(existing)
				return existing
			}
		}
	}
	entry := &_TypeIDCacheEntry{typ: t, id: id, slot: slot}
	entry = recordTypeIDEntry(id, entry)
	typeIDCache[slot].Store(entry)
	if prev := typeIDLastEntry.Load(); prev != nil && prev.typ != t {
		typeIDPrevEntry.Store(prev)
	}
	typeIDLastEntry.Store(entry)
	return entry
}

func getTypeIDSlow(t reflect.Type) _TypeID {
	id := _TypeID(nextTypeID.Add(1))
	idToType.Store(id, t)

	v, loaded := typeToID.LoadOrStore(t, id)
	if loaded {
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

func getTypeIDCached(t reflect.Type) _TypeID {
	return getTypeEntrySlow(t).id
}

func getTypeIDForGet(t reflect.Type) _TypeID {
	if entry := typeIDLastEntry.Load(); entry != nil && entry.typ == t {
		return entry.id
	}
	return getTypeIDForGetSlow(t)
}

func getTypeIDForGetSlow(t reflect.Type) _TypeID {
	if t == nil {
		return getTypeIDUncached(t)
	}
	slot := int(maphash.Comparable(typeHashSeed, t) & (typeIDCacheSize - 1))
	if entry := typeIDCache[slot].Load(); entry != nil && entry.typ == t {
		return entry.id
	}
	return getTypeIDCached(t)
}

func recordTypeIDEntry(id _TypeID, entry *_TypeIDCacheEntry) *_TypeIDCacheEntry {
	idToTypeMu.Lock()
	var oldSlice []*_TypeIDCacheEntry
	if p := idToEntrySlice.Load(); p != nil {
		oldSlice = *p
	}
	if int(id) < len(oldSlice) && oldSlice[id] != nil {
		existing := oldSlice[id]
		idToTypeMu.Unlock()
		return existing
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
	return entry
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
	if t == nil {
		return 0, false
	}
	return int(maphash.Comparable(typeHashSeed, t) & (typeIDCacheSize - 1)), true
}

func getTypeIDUncached(t reflect.Type) _TypeID {
	if v, ok := typeToID.Load(t); ok {
		return v.(_TypeID)
	}
	return getTypeIDSlow(t)
}

// typeIDCacheSlot maps a type to a cache slot.
func typeIDCacheSlot(t reflect.Type) (int, bool) {
	if entry := typeSlotLastEntry.Load(); entry != nil && entry.typ == t {
		return entry.slot, true
	}
	return typeIDCacheSlotSlow(t)
}

func typeIDCacheSlotSlow(t reflect.Type) (int, bool) {
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
