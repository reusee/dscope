package dscope

import (
	"math/rand/v2"
	"reflect"
	"sync"
	"sync/atomic"
)

// _Value is one provided type in one layer.
type _Value struct {
	initializer *_Initializer
	typeInfo    *_TypeInfo
	id          _TypeID
}

// _TypeInfo describes how one definition produces one of its values: the
// definition type, the output position of the value, and the dependencies of the
// definition that produced it.
type _TypeInfo struct {
	DefType      reflect.Type
	Dependencies []_TypeID
	DefTypeID    _TypeID
	Position     int
}

// _TypeID is a unique identifier for a reflect.Type.
type _TypeID int

// _Hash is used for scope signatures and cache keys.
type _Hash [2]uint64

var hashSeeds = [2]uint64{rand.Uint64(), rand.Uint64()}

const hashStep = 0x9e3779b97f4a7c15

func hashWord(state, word uint64) uint64 {
	x := state + word + hashStep
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ (x >> 31)
}

func hashTypeIDs(ids []_TypeID) (ret _Hash) {
	h0, h1 := hashSeeds[0], hashSeeds[1]
	h0 = hashWord(h0, uint64(len(ids)))
	h1 = hashWord(h1, uint64(len(ids)))
	for _, id := range ids {
		h0 = hashWord(h0, uint64(id))
		h1 = hashWord(h1, uint64(id))
	}
	ret[0] = h0
	ret[1] = h1
	return
}

func forkKey(signature _Hash, defs []any) _Hash {
	if len(defs) == 1 {
		if entry := forkKeyLastEntry.Load(); entry != nil && entry.sig == signature && entry.t0 == reflect.TypeOf(defs[0]) {
			return entry.key
		}
	}
	return forkKeySlow(signature, defs)
}

func forkKeySlow(signature _Hash, defs []any) (ret _Hash) {
	if len(defs) == 1 {
		t0 := reflect.TypeOf(defs[0])
		h0, h1 := hashSeeds[0]^signature[0], hashSeeds[1]^signature[1]
		id := uint64(getTypeID(t0))
		h0 = hashWord(hashWord(h0, 1), id)
		h1 = hashWord(hashWord(h1, 1), id)
		ret = _Hash{h0, h1}
		forkKeyLastEntry.Store(&_ForkKeyCacheEntry{sig: signature, t0: t0, key: ret})
		return ret
	}
	h0, h1 := hashSeeds[0]^signature[0], hashSeeds[1]^signature[1]
	h0 = hashWord(h0, uint64(len(defs)))
	h1 = hashWord(h1, uint64(len(defs)))
	for _, def := range defs {
		id := uint64(getTypeID(reflect.TypeOf(def)))
		h0 = hashWord(h0, id)
		h1 = hashWord(h1, id)
	}
	ret[0] = h0
	ret[1] = h1
	return
}

type _ForkKeyCacheEntry struct {
	sig _Hash
	t0  reflect.Type
	key _Hash
}

var forkKeyLastEntry atomic.Pointer[_ForkKeyCacheEntry]

func forkKeyFromTypes(signature _Hash, defTypes []reflect.Type) (ret _Hash) {
	h0, h1 := hashSeeds[0]^signature[0], hashSeeds[1]^signature[1]
	h0 = hashWord(h0, uint64(len(defTypes)))
	h1 = hashWord(h1, uint64(len(defTypes)))
	for _, t := range defTypes {
		id := uint64(getTypeID(t))
		h0 = hashWord(h0, id)
		h1 = hashWord(h1, id)
	}
	ret[0] = h0
	ret[1] = h1
	return
}

// TheoryOfScopeCore documents the fundamental model of dscope: an immutable,
// type-keyed container of lazily evaluated definitions.
const TheoryOfScopeCore = `
dscope core theory:
- A Scope is an immutable, type-keyed container. Every provided value is
  identified by its declared Go type — concrete or interface — and a type has
  at most one effective definition per scope.
- Resolution is by exact declared type: a provider returning an interface
  satisfies requests for that interface, not requests for the concrete value
  it holds; no implicit conversions are performed.
- A computation is a provider: fork the computing function, then get its
  result type. There is no separate invocation entry point.
- Three built-in dependencies — InjectStruct, Fork, Reset — are always
  available, bound to the current scope, and cannot be overridden: they are
  the escape hatches through which providers interact with the scope
  dynamically.
- Every public operation — Get, TryGet, Assign, InjectStruct, AllTypes —
  reflects the effective definitions of the scope it is invoked on.
`

const TheoryOfScopeDefinitions = `
dscope definition theory:
- A definition is a provider function (its parameters are dependencies resolved
  from the scope, its results are the provided values) or a pointer to a value
  (the pointed-at value is copied into the scope at construction time).
- A provider may return multiple values; each result type becomes a provided
  type of the scope, and a single evaluation feeds all of them.
- Public scope construction validates every definition before deriving type
  identity: nil definitions, nil function or pointer definitions, functions
  that return nothing, variadic functions, and non-function non-pointer
  values are rejected.
- Two definitions in the same Fork call must not produce the same type; a
  duplicate is rejected with an error naming both conflicting definitions.
- Invalid definitions produce structured dscope errors rather than leaking
  reflection, hashing, or storage implementation panics.
`

// Scope represents an immutable dependency injection container.
type Scope struct {
	values    *_StackedMap
	signature _Hash
}

type _LazyResetBundle struct {
	layer _StackedMap
	extra _StackedMapExtra
	reset _ResetState
}

// Universe is the empty root scope.
var Universe = Scope{}

// New creates a new root Scope with the given definitions.
func New(defs ...any) Scope {
	return Universe.Fork(defs...)
}

const forkerCacheSize = 1024

type _ForkerCacheEntry struct {
	key    _Hash
	forker *_Forker
}

type _ForkerTable struct {
	slots [forkerCacheSize]atomic.Pointer[_ForkerCacheEntry]
	mu    sync.RWMutex
	m     map[_Hash]*_Forker
}

func (t *_ForkerTable) Load(key _Hash) (*_Forker, bool) {
	slot := &t.slots[key[0]&(forkerCacheSize-1)]
	if entry := slot.Load(); entry != nil && entry.key == key {
		return entry.forker, true
	}
	return t.loadSlow(key, slot)
}

func (t *_ForkerTable) loadSlow(key _Hash, slot *atomic.Pointer[_ForkerCacheEntry]) (*_Forker, bool) {
	t.mu.RLock()
	f, ok := t.m[key]
	t.mu.RUnlock()
	if ok && slot.Load() == nil {
		slot.CompareAndSwap(nil, &_ForkerCacheEntry{key: key, forker: f})
	}
	return f, ok
}

func (t *_ForkerTable) LoadOrStore(key _Hash, forker *_Forker) (*_Forker, bool) {
	t.mu.Lock()
	if t.m == nil {
		t.m = make(map[_Hash]*_Forker, 64)
	}
	if existing, ok := t.m[key]; ok {
		t.mu.Unlock()
		return existing, true
	}
	t.m[key] = forker
	t.mu.Unlock()
	slot := &t.slots[key[0]&(forkerCacheSize-1)]
	if slot.Load() == nil {
		slot.CompareAndSwap(nil, &_ForkerCacheEntry{key: key, forker: forker})
	}
	return forker, false
}

var forkers _ForkerTable

type _Forker1CacheEntry struct {
	sig    _Hash
	t0     reflect.Type
	forker *_Forker
	nilPtr any
	isPtr  bool
}

type _Forker2CacheEntry struct {
	sig    _Hash
	t0     reflect.Type
	t1     reflect.Type
	forker *_Forker
	nil0   any
	nil1   any
	isPtr0 bool
	isPtr1 bool
}

var (
	forker1LastEntry atomic.Pointer[_Forker1CacheEntry]
	forker2LastEntry atomic.Pointer[_Forker2CacheEntry]
)

func (scope Scope) Fork(defs ...any) Scope {
	if len(defs) == 0 {
		return scope
	}
	return scope.forkDefs(defs)
}

func (scope Scope) forkDefs(defs []any) Scope {
	if len(defs) == 1 {
		def := defs[0]
		if def != nil {
			t0 := reflect.TypeOf(def)
			if entry := forker1LastEntry.Load(); entry != nil && entry.sig == scope.signature && entry.t0 == t0 {
				if entry.isPtr {
					if def == entry.nilPtr {
						throwNilDef(def, true)
					}
				} else if reflect.ValueOf(def).IsNil() {
					throwNilDef(def, false)
				}
				f := entry.forker
				if f.kind == forkerKindSingleFunc1 {
					base := scope.values
					if base != nil && (base.isLazyReset() || base.Height > stackHeightLimit) {
						base = base.flatten()
					}
					if len(f.ResetIDs) == 0 {
						bundle := new(_ForkBundle)
						layer := &bundle.layer
						if base != nil {
							layer.Next = base
							layer.Height = base.Height + 1
							if hb := base.hot.Load(); hb != nil && hb.id != f.val0ID {
								layer.hot.Store(hb)
							}
						} else {
							layer.Height = 1
						}
						layer.count = 1
						bundle.init0.Def = def
						layer.val0 = _Value{
							initializer: &bundle.init0,
							typeInfo:    f.val0TypeInfo,
							id:          f.val0ID,
						}
						return Scope{values: layer, signature: f.Signature}
					}
					return f.applySingleFunc1Reset(base, def)
				}
				return f.forkNonSingle1(scope, defs)
			}
			return scope.fork1Miss(defs, def, t0)
		}
	} else if len(defs) == 2 {
		d0, d1 := defs[0], defs[1]
		if d0 != nil && d1 != nil {
			t0, t1 := reflect.TypeOf(d0), reflect.TypeOf(d1)
			if entry := forker2LastEntry.Load(); entry != nil && entry.sig == scope.signature && entry.t0 == t0 && entry.t1 == t1 {
				if entry.isPtr0 {
					if d0 == entry.nil0 {
						throwNilDef(d0, true)
					}
				} else if reflect.ValueOf(d0).IsNil() {
					throwNilDef(d0, false)
				}
				if entry.isPtr1 {
					if d1 == entry.nil1 {
						throwNilDef(d1, true)
					}
				} else if reflect.ValueOf(d1).IsNil() {
					throwNilDef(d1, false)
				}
				f := entry.forker
				if f.kind == forkerKindSingleFunc2 {
					base := scope.values
					if base != nil && (base.isLazyReset() || base.Height > stackHeightLimit) {
						base = base.flatten()
					}
					if len(f.ResetIDs) == 0 {
						bundle := new(_ForkBundle2)
						layer := &bundle.layer
						if base != nil {
							layer.Next = base
							layer.Height = base.Height + 1
							if hb := base.hot.Load(); hb != nil && hb.id != f.val0ID && hb.id != f.val1ID {
								layer.hot.Store(hb)
							}
						} else {
							layer.Height = 1
						}
						layer.count = 2
						bundle.inits[0].Def = d0
						bundle.inits[1].Def = d1
						v0 := _Value{initializer: &bundle.inits[0], typeInfo: f.val0TypeInfo, id: f.val0ID}
						v1 := _Value{initializer: &bundle.inits[1], typeInfo: f.val1TypeInfo, id: f.val1ID}
						if !f.swap2 {
							layer.val0 = v0
							bundle.val1[0] = v1
						} else {
							layer.val0 = v1
							bundle.val1[0] = v0
						}
						bundle.extra.values = bundle.val1[:]
						layer.extra = &bundle.extra
						return Scope{values: layer, signature: f.Signature}
					}
					return f.applySingleFunc2Reset(base, d0, d1)
				}
				return f.Fork(scope, defs)
			}
			return scope.fork2Miss(defs, d0, d1, t0, t1)
		}
	}
	return scope.forkMany(defs)
}

func (scope Scope) fork1Miss(defs []any, def any, t0 reflect.Type) Scope {
	if _, ok := def.(isModule); ok {
		return scope.forkMany(defs)
	}
	typeEntry := getTypeEntry(t0)
	validateDefinitionEntry(def, typeEntry)
	id0 := uint64(typeEntry.id)
	h0 := hashWord(hashWord(hashSeeds[0]^scope.signature[0], 1), id0)
	h1 := hashWord(hashWord(hashSeeds[1]^scope.signature[1], 1), id0)
	key := _Hash{h0, h1}
	forker, ok := forkers.Load(key)
	if !ok {
		forker = newForkerWithEntries(scope, []*_TypeIDCacheEntry{typeEntry})
		forker, _ = forkers.LoadOrStore(key, forker)
	}
	meta := getOrBuildDefMeta(typeEntry)
	forker1LastEntry.Store(&_Forker1CacheEntry{
		sig:    scope.signature,
		t0:     t0,
		forker: forker,
		nilPtr: meta.nilPtr,
		isPtr:  meta.isPointer,
	})
	return forker.Fork(scope, defs)
}

func (scope Scope) fork2Miss(defs []any, d0, d1 any, t0, t1 reflect.Type) Scope {
	_, m0 := d0.(isModule)
	_, m1 := d1.(isModule)
	if m0 || m1 {
		return scope.forkMany(defs)
	}
	e0 := getTypeEntry(t0)
	validateDefinitionEntry(d0, e0)
	e1 := getTypeEntry(t1)
	validateDefinitionEntry(d1, e1)
	h0 := hashWord(hashWord(hashWord(hashSeeds[0]^scope.signature[0], 2), uint64(e0.id)), uint64(e1.id))
	h1 := hashWord(hashWord(hashWord(hashSeeds[1]^scope.signature[1], 2), uint64(e0.id)), uint64(e1.id))
	key := _Hash{h0, h1}
	forker, ok := forkers.Load(key)
	if !ok {
		forker = newForkerWithEntries(scope, []*_TypeIDCacheEntry{e0, e1})
		forker, _ = forkers.LoadOrStore(key, forker)
	}
	meta0 := getOrBuildDefMeta(e0)
	meta1 := getOrBuildDefMeta(e1)
	forker2LastEntry.Store(&_Forker2CacheEntry{
		sig:    scope.signature,
		t0:     t0,
		t1:     t1,
		forker: forker,
		nil0:   meta0.nilPtr,
		nil1:   meta1.nilPtr,
		isPtr0: meta0.isPointer,
		isPtr1: meta1.isPointer,
	})
	return forker.Fork(scope, defs)
}

func (scope Scope) forkMany(defs []any) Scope {
	var moduleObjects []any
	for _, def := range defs {
		if module, ok := def.(isModule); ok {
			moduleObjects = append(moduleObjects, module)
		}
	}
	if len(moduleObjects) > 0 {
		newDefs := make([]any, 0, len(defs))
		for _, def := range defs {
			if _, ok := def.(isModule); !ok {
				newDefs = append(newDefs, def)
			}
		}
		defs = append(newDefs, Methods(moduleObjects...)...)
		if len(defs) == 0 {
			return scope
		}
	}

	var entriesBuf [8]*_TypeIDCacheEntry
	var entries []*_TypeIDCacheEntry
	if len(defs) <= len(entriesBuf) {
		entries = entriesBuf[:len(defs)]
	} else {
		entries = make([]*_TypeIDCacheEntry, len(defs))
	}
	h0, h1 := hashSeeds[0]^scope.signature[0], hashSeeds[1]^scope.signature[1]
	h0 = hashWord(h0, uint64(len(defs)))
	h1 = hashWord(h1, uint64(len(defs)))
	for i, def := range defs {
		if def == nil {
			panic(errWith(ErrBadArgument, "nil definition"))
		}
		e := getTypeEntry(reflect.TypeOf(def))
		validateDefinitionEntry(def, e)
		entries[i] = e
		id := uint64(e.id)
		h0 = hashWord(h0, id)
		h1 = hashWord(h1, id)
	}
	key := _Hash{h0, h1}
	forker, ok := forkers.Load(key)
	if !ok {
		forker = newForkerWithEntries(scope, entries)
		forker, _ = forkers.LoadOrStore(key, forker)
	}
	return forker.Fork(scope, defs)
}

// TheoryOfScopeReset documents the semantics and typical use of Reset.
const TheoryOfScopeReset = `
dscope reset theory:
- Reset returns a new scope in which every cached provider result is
  invalidated; the original scope is unaffected.
- Reset is O(1): it installs a lazy reset layer over the existing value stack.
- Typical use: keep the definitions but drop every cached result, either to
  observe fresh provider evaluation in tests, or to re-run the graph after
  external state (files, clocks, globals) has changed.
`

func (scope Scope) Reset() Scope {
	if scope.values == nil {
		return scope
	}
	var bundle _LazyResetBundle
	bundle.layer.extra = &bundle.extra
	bundle.extra.reset = &bundle.reset
	bundle.reset.base = scope.values
	bundle.layer.Height = 1
	return Scope{
		values:    &bundle.layer,
		signature: scope.signature,
	}
}

const TheoryOfScopeAssignment = `
dscope assignment theory:
- Values are retrieved from a scope through typed pointers: Scope.Assign[T]
  resolves T from the scope and writes the value; Scope.Get[T] returns the
  value directly.
- A missing type panics with a structured dependency-not-found error;
  Scope.TryGet[T] is the non-panicking variant: it returns (zero, false) for
  a missing type, while provider panics still propagate.
- Reflection-based retrieval mirrors the generic forms: Scope.GetType and
  Scope.TryGetType take a reflect.Type and return a reflect.Value; GetType
  panics on a missing type, TryGetType returns (zero Value, false), and a
  nil type is a bad argument.
- Assign[T] accepts a single typed pointer; a nil pointer is a bad argument
  and must produce a structured dscope error instead of leaking a runtime or
  reflection panic.
`

// Assign retrieves the value of type T from the scope and writes it to ptr.
func (scope Scope) Assign[T any](ptr *T) {
	if ptr == nil {
		panic(errWith(ErrBadArgument, "cannot assign to a nil pointer target of type %T", ptr))
	}
	if s := scope.values; s != nil {
		if hb := s.hot.Load(); hb != nil {
			if p, ok := hb.val.(*T); ok {
				*ptr = *p
				return
			}
		}
	}
	*ptr = getSlow[T](scope)
}

func (scope Scope) get(id _TypeID) (ret reflect.Value, ok bool) {
	switch id {
	case injectStructTypeID:
		return reflect.ValueOf(InjectStruct(scope.InjectStruct)), true
	case forkTypeID:
		return reflect.ValueOf(Fork(scope.Fork)), true
	case resetTypeID:
		return reflect.ValueOf(Reset(scope.Reset)), true
	}

	value, ok := scope.values.Load(id)
	if !ok {
		return ret, false
	}
	return value.initializer.get(scope, value), true
}

func (scope Scope) getBuiltinIface(id _TypeID) any {
	switch id {
	case injectStructTypeID:
		return InjectStruct(scope.InjectStruct)
	case forkTypeID:
		return Fork(scope.Fork)
	default:
		return Reset(scope.Reset)
	}
}

func evalValueIface(scope Scope, v _Value) any {
	init := v.initializer
	state := init.state.Load()
	if state&initializerStateDone != 0 {
		if state&initializerStateMulti == 0 {
			return init.result
		}
		return init.result.(*_MultiResult).ifaces[v.typeInfo.Position]
	}
	return init.getIface(scope, v)
}

func (scope Scope) getIface(id _TypeID) (any, bool) {
	if uint(id-1) < 3 {
		return scope.getBuiltinIface(id), true
	}
	s := scope.values
	if s == nil {
		return nil, false
	}
	extra := s.extra
	if extra == nil {
		if s.count == 1 && s.val0.id == id {
			init := s.val0.initializer
			state := init.state.Load()
			if state&initializerStateDone != 0 {
				if state&initializerStateMulti == 0 {
					return init.result, true
				}
				return init.result.(*_MultiResult).ifaces[s.val0.typeInfo.Position], true
			}
			return init.getIface(scope, s.val0), true
		}
		value, ok := s.Next.loadSlow(id)
		if !ok {
			return nil, false
		}
		return evalValueIface(scope, value), true
	}
	vpHit := extra.hitVal.Load()
	if vpHit != nil && vpHit.id == id {
		init := vpHit.initializer
		state := init.state.Load()
		if state&initializerStateDone != 0 {
			if state&initializerStateMulti == 0 {
				return init.result, true
			}
			return init.result.(*_MultiResult).ifaces[vpHit.typeInfo.Position], true
		}
		return init.getIface(scope, *vpHit), true
	}
	if extra.reset == nil {
		if idxPtr := extra.index; idxPtr != nil {
			if id >= s.val0.id {
				positions := *idxPtr
				values := extra.values
				mask := uint64(len(positions) - 1)
				for slot := indexHashSlot(id, mask); ; slot = (slot + 1) & mask {
					pos := positions[slot]
					if pos == 0 {
						break
					}
					vp := &values[pos-1]
					if vp.id == id {
						if vpHit == nil {
							s.storeHitPtr(vp)
						}
						init := vp.initializer
						state := init.state.Load()
						if state&initializerStateDone != 0 {
							if state&initializerStateMulti == 0 {
								return init.result, true
							}
							return init.result.(*_MultiResult).ifaces[vp.typeInfo.Position], true
						}
						return init.getIface(scope, *vp), true
					}
				}
			}
			value, ok := s.Next.Load(id)
			if !ok {
				return nil, false
			}
			return evalValueIface(scope, value), true
		}
	}
	value, ok := s.loadSlow(id)
	if !ok {
		return nil, false
	}
	return evalValueIface(scope, value), true
}

func cacheResolvedValue[T any](s *_StackedMap, id _TypeID, slot uint64, hasSlot bool, typ reflect.Type, o T) {
	if s.hot.Load() == nil {
		p := new(T)
		*p = o
		hb := &_HotBox{val: p, id: id}
		if s.hot.CompareAndSwap(nil, hb) {
			return
		}
		if s.extra != nil {
			if !hasSlot {
				slot = boxSlot(typ)
			}
			boxes := s.extra.getOrCreateBoxes()
			if boxes[slot].Load() == nil {
				boxes[slot].CompareAndSwap(nil, hb)
			}
		}
		return
	}
	if s.extra == nil {
		return
	}
	if !hasSlot {
		slot = boxSlot(typ)
	}
	boxes := s.extra.getOrCreateBoxes()
	if boxes[slot].Load() != nil {
		return
	}
	p := new(T)
	*p = o
	hb := &_HotBox{val: p, id: id}
	boxes[slot].CompareAndSwap(nil, hb)
}

func getSlow[T any](scope Scope) (o T) {
	typ := reflect.TypeFor[T]()
	s := scope.values
	var slot uint64
	var hasSlot bool
	if s != nil && s.extra != nil {
		slot = boxSlot(typ)
		hasSlot = true
		if boxes := s.extra.boxes.Load(); boxes != nil {
			if hb := boxes[slot].Load(); hb != nil {
				if p, ok := hb.val.(*T); ok {
					return *p
				}
			}
		}
	}
	id := getTypeIDForGet(typ)
	iface, ok := scope.getIface(id)
	if !ok {
		throwErrDependencyNotFound(typ)
	}
	if iface != nil {
		o = iface.(T)
	}
	if s != nil && uint(id-1) >= 3 {
		cacheResolvedValue(s, id, slot, hasSlot, typ, o)
	}
	return o
}

func tryGetSlow[T any](scope Scope) (o T, ok bool) {
	typ := reflect.TypeFor[T]()
	s := scope.values
	var slot uint64
	var hasSlot bool
	if s != nil && s.extra != nil {
		slot = boxSlot(typ)
		hasSlot = true
		if boxes := s.extra.boxes.Load(); boxes != nil {
			if hb := boxes[slot].Load(); hb != nil {
				if p, hit := hb.val.(*T); hit {
					return *p, true
				}
			}
		}
	}
	id := getTypeIDForGet(typ)
	iface, found := scope.getIface(id)
	if !found {
		return o, false
	}
	if iface != nil {
		o = iface.(T)
	}
	if s != nil && uint(id-1) >= 3 {
		cacheResolvedValue(s, id, slot, hasSlot, typ, o)
	}
	return o, true
}

// Get is a type-safe generic method to retrieve a single value of type T.
func (scope Scope) Get[T any]() (o T) {
	if s := scope.values; s != nil {
		if hb := s.hot.Load(); hb != nil {
			if p, ok := hb.val.(*T); ok {
				return *p
			}
		}
	}
	return getSlow[T](scope)
}

// TryGet is a type-safe generic method to retrieve a single value of type T.
func (scope Scope) TryGet[T any]() (o T, ok bool) {
	if s := scope.values; s != nil {
		if hb := s.hot.Load(); hb != nil {
			if p, hit := hb.val.(*T); hit {
				return *p, true
			}
		}
	}
	return tryGetSlow[T](scope)
}

// GetType retrieves the value of the given type from the scope as reflect.Value.
func (scope Scope) GetType(typ reflect.Type) reflect.Value {
	value, ok := scope.TryGetType(typ)
	if !ok {
		throwErrDependencyNotFound(typ)
	}
	return value
}

// TryGetType retrieves the value of the given type from the scope as reflect.Value.
func (scope Scope) TryGetType(typ reflect.Type) (reflect.Value, bool) {
	if typ == nil {
		panic(errWith(ErrBadArgument, "nil reflect.Type provided"))
	}
	value, found := scope.get(getTypeID(typ))
	if !found {
		return reflect.Value{}, false
	}
	return value, true
}

var getArgsFuncLastEntry atomic.Pointer[_TypeIDCacheEntry]

func (scope Scope) getArgs(fnType reflect.Type, args []reflect.Value) int {
	if entry := getArgsFuncLastEntry.Load(); entry != nil && entry.typ == fnType {
		if fn := entry.getArgsFn.Load(); fn != nil {
			return (*fn)(scope, args)
		}
	}
	return scope.getArgsSlow(fnType, args)
}

func (scope Scope) getArgsSlow(fnType reflect.Type, args []reflect.Value) int {
	entry := getTypeEntry(fnType)
	if fn := entry.getArgsFn.Load(); fn != nil {
		getArgsFuncLastEntry.Store(entry)
		return (*fn)(scope, args)
	}
	numIn := fnType.NumIn()
	ids := make([]_TypeID, numIn)
	for i := range numIn {
		ids[i] = getTypeID(fnType.In(i))
	}
	fn := func(scope Scope, args []reflect.Value) int {
		for i, id := range ids {
			var ok bool
			args[i], ok = scope.get(id)
			if !ok {
				throwErrDependencyNotFound(typeIDToType(id))
			}
		}
		return numIn
	}
	entry.getArgsFn.CompareAndSwap(nil, &fn)
	loaded := entry.getArgsFn.Load()
	getArgsFuncLastEntry.Store(entry)
	return (*loaded)(scope, args)
}

const (
	reflectValuesSmallLen   = 8
	reflectValuesPoolMaxLen = 64
)

var reflectValuesSmallPool = sync.Pool{
	New: func() any {
		return new([reflectValuesSmallLen]reflect.Value)
	},
}

var reflectValuesPool = sync.Pool{
	New: func() any {
		return new([reflectValuesPoolMaxLen]reflect.Value)
	},
}

func (scope Scope) call(fnValue reflect.Value) []reflect.Value {
	fnType := fnValue.Type()
	nArgs := fnType.NumIn()
	if nArgs == 0 {
		return fnValue.Call(nil)
	}
	if nArgs <= reflectValuesSmallLen {
		var local [reflectValuesSmallLen]reflect.Value
		n := scope.getArgs(fnType, local[:nArgs])
		ptr := reflectValuesSmallPool.Get().(*[reflectValuesSmallLen]reflect.Value)
		copy(ptr[:n], local[:n])
		rets := fnValue.Call(ptr[:n])
		clear(ptr[:n])
		reflectValuesSmallPool.Put(ptr)
		return rets
	}
	if nArgs <= reflectValuesPoolMaxLen {
		var local [reflectValuesPoolMaxLen]reflect.Value
		n := scope.getArgs(fnType, local[:nArgs])
		ptr := reflectValuesPool.Get().(*[reflectValuesPoolMaxLen]reflect.Value)
		copy(ptr[:n], local[:n])
		rets := fnValue.Call(ptr[:n])
		clear(ptr[:n])
		reflectValuesPool.Put(ptr)
		return rets
	}
	args := make([]reflect.Value, nArgs)
	n := scope.getArgs(fnType, args)
	return fnValue.Call(args[:n])
}
