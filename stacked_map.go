package dscope

import (
	"cmp"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
)

type _StackedMap struct {
	Next   *_StackedMap // Previous layer in the stack.
	Values []_Value     // Values in this layer, sorted by TypeID.
	// index maps the type ID of a value to its position in Values, one plus the
	// position, so that 0 marks an empty slot. A layer whose binary search is
	// short enough leaves it nil. The index is built while the layer is created,
	// before the layer becomes visible to another goroutine, so a lookup reads
	// it without synchronization.
	index []uint16
	// reset carries the state of a layer that hands out fresh values. A plain
	// layer leaves it nil.
	reset *_ResetState
	// Height is the height of the stack from this node downwards. Fork flattens
	// a stack above an internal limit, so the height stays small.
	Height int
	// hit remembers the entry a layer resolved most recently, packed into one
	// word: the high bits carry the type ID, the low stackedMapHitBits carry the
	// position in Values, biased by one, so a zero word marks an empty memo. The
	// memo is a hint, confirmed against the entry itself, so a program that asks
	// for the same type over and over pays one load, while a program that asks
	// for many types in turn pays one comparison.
	hit atomic.Uint64
	// inline backs Values when the layer holds few values, so such a layer
	// needs no separate value slice.
	inline [stackedMapInlineValues]_Value
}

// _ResetState carries what a layer needs to hand out fresh values, together with
// the cache of the fresh initializers it created. A lazy reset layer sets base
// and delegates every lookup to it; a partial reset layer lists in ids the sorted
// inherited types it refreshes on access. The cache comes into existence on the
// first access to a refreshed type, so a layer whose refreshed types are never
// resolved never pays for it.
type _ResetState struct {
	base  *_StackedMap // Non-nil for a lazy reset layer.
	ids   []_TypeID    // Sorted types a partial reset layer refreshes; the slice belongs to the _Forker.
	cache atomic.Pointer[sync.Map]
	// last remembers the mapping the layer used most recently, so a program that
	// resolves one refreshed type over and over skips the cache. The mapping is
	// immutable and carries the key it belongs to, so a reader that sees it never
	// pairs a key with the fresh value of another key.
	last atomic.Pointer[_initPair]
}

// _initPair is one confirmed mapping from an inherited initializer to the fresh
// initializer a reset layer hands out for it. A reset layer publishes a mapping
// as one value, so a reader always reads the fresh initializer together with the
// key it belongs to.
type _initPair struct {
	key   *_Initializer
	fresh *_Initializer
	id    _TypeID
	val   _Value
}

// resetCache returns the cache of the fresh initializers of the layer and creates
// it on first use.
func (r *_ResetState) resetCache() *sync.Map {
	if cache := r.cache.Load(); cache != nil {
		return cache
	}
	created := new(sync.Map)
	if r.cache.CompareAndSwap(nil, created) {
		return created
	}
	return r.cache.Load()
}

// stackedMapInlineValues is the number of values a layer holds in itself. A Fork
// usually adds a single definition, and most layers never grow.
const stackedMapInlineValues = 1

// stackedMapIndexThreshold is the number of values from which a layer builds an
// index. A shorter layer answers with a binary search over its sorted values,
// which costs a few loads and no memory.
const stackedMapIndexThreshold = 8

// stackedMapIndexLimit is the largest layer that builds an index. An index entry
// is a uint16 position, so a layer with more values keeps its binary search.
const stackedMapIndexLimit = 1 << 15

// stackedMapHitBits is the number of low bits a memo word keeps for the position
// of the entry a layer resolved most recently. The remaining bits carry the type
// ID, so a lookup rejects the memo of another type without touching a value.
const stackedMapHitBits = 16

// stackedMapHitPosMask selects the position bits of a memo word.
const stackedMapHitPosMask = 1<<stackedMapHitBits - 1

// initStackedMapLayer makes layer an empty layer that sits on top of base. The
// caller fills Values with values sorted by TypeID.
func initStackedMapLayer(layer, base *_StackedMap, n int) {
	layer.Next = base
	layer.Height = 1
	if base != nil {
		layer.Height = base.Height + 1
	}
	if n <= stackedMapInlineValues {
		layer.Values = layer.inline[:n]
	} else {
		layer.Values = make([]_Value, n)
	}
}

// newStackedMapLayer creates an empty layer that sits on top of base. The caller
// fills Values with values sorted by TypeID.
func newStackedMapLayer(base *_StackedMap, n int) *_StackedMap {
	layer := new(_StackedMap)
	initStackedMapLayer(layer, base, n)
	return layer
}

// buildIndex gives the layer an index over its values. It must run while the
// layer is built, before another goroutine can reach it. A layer with few values
// keeps its binary search, and so does a layer whose positions do not fit an
// index entry.
func (s *_StackedMap) buildIndex() {
	n := len(s.Values)
	if n < stackedMapIndexThreshold || n > stackedMapIndexLimit {
		return
	}
	size := 1
	for size < n*2 {
		size <<= 1
	}
	mask := uint64(size - 1)
	positions := make([]uint16, size)
	for i := range s.Values {
		slot := uint64(s.Values[i].id) & mask
		for positions[slot] != 0 {
			slot = (slot + 1) & mask
		}
		positions[slot] = uint16(i + 1)
	}
	s.index = positions
}

// isLazyReset reports whether the layer delegates every lookup to its base.
func (s *_StackedMap) isLazyReset() bool {
	return s.reset != nil && s.reset.base != nil
}

// markRefresh turns the layer into a partial reset layer: the inherited types in
// ids are refreshed on access. The layer keeps no initializer for a marked type,
// so a fresh initializer comes into existence in the layer's cache only when the
// type is actually resolved. ids must be sorted, belong to the scope below the
// layer, and be disjoint from the types of the layer's own values.
func (s *_StackedMap) markRefresh(ids []_TypeID) {
	s.reset = &_ResetState{ids: ids}
}

// Load finds the value with the specified TypeID.
func (s *_StackedMap) Load(id _TypeID) (ret _Value, ok bool) {
	if s == nil {
		return ret, false
	}
	word := s.hit.Load()
	if _TypeID(word>>stackedMapHitBits) == id {
		if position := uint(word&stackedMapHitPosMask) - 1; position < uint(len(s.Values)) {
			return s.Values[position], true
		}
	}
	return s.loadSlow(id, word)
}

func (s *_StackedMap) loadSlow(id _TypeID, word uint64) (ret _Value, ok bool) {
	if s.reset != nil && s.reset.base != nil {
		if pair := s.reset.last.Load(); pair != nil && pair.id == id {
			return pair.val, true
		}
		v, found := s.reset.base.Load(id)
		if !found {
			return ret, false
		}
		return s.refreshValue(v), true
	}

	for cur := s; cur != nil; cur = cur.Next {
		if rs := cur.reset; rs != nil && rs.base == nil && containsID(rs.ids, id) {
			v, found := cur.Next.Load(id)
			if !found {
				panic("impossible: refreshed type not found below its layer")
			}
			return cur.refreshValue(v), true
		}

		values := cur.Values
		if len(values) == 1 {
			if values[0].id == id {
				if cur == s && word == 0 {
					s.storeHit(id, 0)
				} else if cur != s && cur.hit.Load() == 0 {
					cur.storeHit(id, 0)
				}
				return values[0], true
			}
			continue
		}
		if positions := cur.index; positions != nil {
			mask := uint64(len(positions) - 1)
			for slot := uint64(id) & mask; ; slot = (slot + 1) & mask {
				position := positions[slot]
				if position == 0 {
					break
				}
				if v := &values[position-1]; v.id == id {
					if cur == s && word == 0 {
						s.storeHit(id, int(position)-1)
					}
					return *v, true
				}
			}
			continue
		}

		left, right := uint(0), uint(len(values))
		for left < right {
			mid := (left + right) >> 1
			midID := values[mid].id
			if midID > id {
				right = mid
			} else if midID < id {
				left = mid + 1
			} else {
				if cur == s && word == 0 {
					s.storeHit(id, int(mid))
				}
				return values[mid], true
			}
		}
	}
	return
}

// containsID reports whether the sorted ID slice holds id.
func containsID(ids []_TypeID, id _TypeID) bool {
	_, ok := slices.BinarySearch(ids, id)
	return ok
}

// refreshValue returns the value this layer hands out for v. A reset layer keys
// the fresh initializer by the inherited initializer, so every output of one
// definition shares one fresh provider: the provider still runs once, and each
// output keeps its own identity and position. Pointer initializers are returned
// as they are because they never need re-evaluation.
func (s *_StackedMap) refreshValue(v _Value) _Value {
	if v.initializer.DefIsPointer {
		return v
	}
	state := s.reset
	prevPair := state.last.Load()
	if prevPair != nil && prevPair.key == v.initializer {
		v.initializer = prevPair.fresh
		return v
	}
	cache := state.resetCache()
	origInit := v.initializer
	loaded, ok := cache.Load(origInit)
	if !ok {
		fresh := origInit.reset()
		refreshed := v
		refreshed.initializer = fresh
		actual, _ := cache.LoadOrStore(origInit, &_initPair{
			key:   origInit,
			fresh: fresh,
			id:    v.id,
			val:   refreshed,
		})
		loaded = actual
	}
	pair := loaded.(*_initPair)
	state.last.Store(pair)
	if prevPair == nil {
		promoteTypeID(v.id)
	}
	v.initializer = pair.fresh
	return v
}

// storeHit records a resolved entry in the memo of the layer that holds the
// entry. A layer keeps the entry it remembers first, so a hot layer pays one
// load per lookup and never a write. A position too large for a memo word is
// not recorded; the search answers such a type as it does any other.
func (s *_StackedMap) storeHit(id _TypeID, position int) {
	if position >= stackedMapHitPosMask {
		return
	}
	if s.hit.Load() == 0 {
		s.hit.Store(uint64(id)<<stackedMapHitBits | uint64(position+1))
		promoteTypeID(id)
	}
}

func (s *_StackedMap) IterValues() iter.Seq[_Value] {
	if s != nil && s.isLazyReset() {
		resetLayer := s
		return func(yield func(_Value) bool) {
			for v := range resetLayer.reset.base.IterValues() {
				if !yield(resetLayer.refreshValue(v)) {
					return
				}
			}
		}
	}
	return func(yield func(_Value) bool) {
		keys := make(map[_TypeID]struct{})
		// refreshers collects the partial reset layers above the current layer,
		// from the top of the stack downwards. A value found below them is
		// refreshed by each of them, the way Load replaces initializers.
		var refreshers []*_StackedMap
		for cur := s; cur != nil; cur = cur.Next {
			for _, d := range cur.Values {
				id := d.id
				if _, ok := keys[id]; ok {
					continue
				}
				keys[id] = struct{}{}
				for i := len(refreshers) - 1; i >= 0; i-- {
					if r := refreshers[i]; containsID(r.reset.ids, id) {
						d = r.refreshValue(d)
					}
				}
				if !yield(d) {
					return
				}
			}
			if cur.reset != nil && cur.reset.base == nil {
				refreshers = append(refreshers, cur)
			}
		}
	}
}

// Append creates a new _StackedMap layer on top of the current one. The provided
// values must be pre-sorted by TypeID; they are copied into the layer.
//
// If the receiver is a lazy reset layer it is first materialised into a flat
// sorted stack so that subsequent binary searches remain correct.
func (s *_StackedMap) Append(values []_Value) *_StackedMap {
	if s != nil && s.isLazyReset() {
		s = s.flatten()
	}
	layer := newStackedMapLayer(s, len(values))
	copy(layer.Values, values)
	layer.buildIndex()
	return layer
}

// Len returns the number of value entries the stack tracks: the values of each
// layer plus the inherited types a partial reset layer marks for refresh.
func (s *_StackedMap) Len() int {
	if s == nil {
		return 0
	}
	if s.isLazyReset() {
		return s.reset.base.Len()
	}
	ret := 0
	for s != nil {
		if s.reset != nil {
			ret += len(s.reset.ids)
		}
		ret += len(s.Values)
		s = s.Next
	}
	return ret
}

type _FlatLayer2 struct {
	layer _StackedMap
	vals  [2]_Value
}

// flatten materialises the effective values of the stack into a single sorted
// layer. Effective values and override semantics are preserved.
func (s *_StackedMap) flatten() *_StackedMap {
	if s == nil {
		return nil
	}
	if s.isLazyReset() {
		baseFlat := s.reset.base.flatten()
		if baseFlat == nil {
			return newStackedMapLayer(nil, 0)
		}
		n := len(baseFlat.Values)
		layer := newStackedMapLayer(nil, n)
		for i, v := range baseFlat.Values {
			layer.Values[i] = s.refreshValue(v)
		}
		layer.buildIndex()
		return layer
	}

	var buf [32]_Value
	flatValues := buf[:0]
	var refreshersBuf [stackHeightLimit + 2]*_StackedMap
	refreshers := refreshersBuf[:0]

	for cur := s; cur != nil; cur = cur.Next {
	valLoop:
		for _, d := range cur.Values {
			id := d.id
			for i := range flatValues {
				if flatValues[i].id == id {
					continue valLoop
				}
			}
			for i := len(refreshers) - 1; i >= 0; i-- {
				if r := refreshers[i]; containsID(r.reset.ids, id) {
					d = r.refreshValue(d)
				}
			}
			flatValues = append(flatValues, d)
		}
		if cur.reset != nil && cur.reset.base == nil {
			refreshers = append(refreshers, cur)
		}
	}

	if len(flatValues) == 2 {
		if flatValues[0].id > flatValues[1].id {
			flatValues[0], flatValues[1] = flatValues[1], flatValues[0]
		}
		b := new(_FlatLayer2)
		b.layer.Height = 1
		b.vals[0] = flatValues[0]
		b.vals[1] = flatValues[1]
		b.layer.Values = b.vals[:]
		return &b.layer
	}
	if len(flatValues) > 2 {
		slices.SortFunc(flatValues, func(a, b _Value) int {
			return cmp.Compare(a.id, b.id)
		})
	}
	layer := newStackedMapLayer(nil, len(flatValues))
	copy(layer.Values, flatValues)
	layer.buildIndex()
	return layer
}
