package dscope

import (
	"cmp"
	"hash/maphash"
	"iter"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
)

const boxTableSize = 256

type _HotBox struct {
	val any
	id  _TypeID
}

type _BoxTable [boxTableSize]atomic.Pointer[_HotBox]

type _StackedMap struct {
	Next   *_StackedMap
	extra  *_StackedMapExtra
	hot    atomic.Pointer[_HotBox]
	val0   _Value
	Height int32
	count  int32
}

// _StackedMapExtra holds multi-value, index, box-cache, and reset metadata for
// a value layer. Single-value plain layers leave extra nil.
// When count == 2, values holds only the second _Value (len 1).
// When count > 2, values holds all count _Values sorted by TypeID.
type _StackedMapExtra struct {
	values []_Value
	index  *[]uint16
	hitVal atomic.Pointer[_Value]
	boxes  atomic.Pointer[_BoxTable]
	reset  *_ResetState
}

type _ResetState struct {
	base  *_StackedMap // Non-nil for a lazy reset layer.
	ids   *[]_TypeID   // Sorted types a partial reset layer refreshes.
	cache atomic.Pointer[sync.Map]
	last  atomic.Pointer[_initPair]
}

type _initPair struct {
	key   *_Initializer
	fresh *_Initializer
	id    _TypeID
	val   _Value
}

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

func (e *_StackedMapExtra) getOrCreateBoxes() *_BoxTable {
	if b := e.boxes.Load(); b != nil {
		return b
	}
	created := new(_BoxTable)
	if e.boxes.CompareAndSwap(nil, created) {
		return created
	}
	return e.boxes.Load()
}

func boxSlot(t reflect.Type) uint64 {
	return maphash.Comparable(typeHashSeed, t) & (boxTableSize - 1)
}

const (
	stackedMapIndexThreshold = 8
	stackedMapIndexLimit     = 1 << 15
)

// initStackedMapLayer makes layer an empty layer that sits on top of base.
func initStackedMapLayer(layer, base *_StackedMap, n int) {
	if base != nil {
		layer.Next = base
		layer.Height = base.Height + 1
	} else {
		layer.Height = 1
	}
	layer.count = int32(n)
	if n == 2 {
		layer.extra = &_StackedMapExtra{
			values: make([]_Value, 1),
		}
	} else if n > 2 {
		layer.extra = &_StackedMapExtra{
			values: make([]_Value, n),
		}
	}
}

// newStackedMapLayer creates an empty layer that sits on top of base.
func newStackedMapLayer(base *_StackedMap, n int) *_StackedMap {
	layer := new(_StackedMap)
	initStackedMapLayer(layer, base, n)
	return layer
}

func (s *_StackedMap) setAt(pos int, v _Value) {
	switch s.count {
	case 1:
		s.val0 = v
	case 2:
		if pos == 0 {
			s.val0 = v
		} else {
			s.extra.values[0] = v
		}
	default:
		if pos == 0 {
			s.val0 = v
		}
		s.extra.values[pos] = v
	}
}

func (s *_StackedMap) copySortedValues(values []_Value) {
	n := len(values)
	if n == 0 {
		return
	}
	s.val0 = values[0]
	if n == 2 {
		s.extra.values[0] = values[1]
	} else if n > 2 {
		copy(s.extra.values, values)
	}
}

func indexHashSlot(id _TypeID, mask uint64) uint64 {
	return ((uint64(id) * hashStep) >> 32) & mask
}

func (s *_StackedMap) buildIndex() {
	n := int(s.count)
	if n < stackedMapIndexThreshold || n > stackedMapIndexLimit {
		return
	}
	size := 1
	for size < n*2 {
		size <<= 1
	}
	mask := uint64(size - 1)
	positions := make([]uint16, size)
	values := s.extra.values
	for i := range values {
		slot := indexHashSlot(values[i].id, mask)
		for positions[slot] != 0 {
			slot = (slot + 1) & mask
		}
		positions[slot] = uint16(i + 1)
	}
	s.extra.index = &positions
}

func (s *_StackedMap) isLazyReset() bool {
	return s.extra != nil && s.extra.reset != nil && s.extra.reset.base != nil
}

func (s *_StackedMap) markRefresh(ids *[]_TypeID) {
	if s.extra == nil {
		s.extra = new(_StackedMapExtra)
	}
	s.extra.reset = &_ResetState{ids: ids}
}

func (s *_StackedMap) storeHitPtr(vp *_Value) {
	if s.extra.hitVal.Load() == nil {
		s.extra.hitVal.Store(vp)
		promoteTypeID(vp.id)
	}
}

// Load finds the value with the specified TypeID.
func (s *_StackedMap) Load(id _TypeID) (ret _Value, ok bool) {
	if s != nil {
		if s.extra == nil {
			if s.count == 1 && s.val0.id == id {
				return s.val0, true
			}
			return s.Next.loadSlow(id)
		}
		if v := s.extra.hitVal.Load(); v != nil && v.id == id {
			return *v, true
		}
		return s.loadSlow(id)
	}
	return ret, false
}

// loadMeta finds the value metadata with the specified TypeID without
// materialising reset initializers or updating lookup memos.
func (s *_StackedMap) loadMeta(id _TypeID) (ret _Value, ok bool) {
	for cur := s; cur != nil; {
		if cur.isLazyReset() {
			cur = cur.extra.reset.base
			continue
		}
		if v, found := cur.searchLayerMeta(id); found {
			return v, true
		}
		cur = cur.Next
	}
	return ret, false
}

func (s *_StackedMap) searchLayerMeta(id _TypeID) (ret _Value, ok bool) {
	c := s.count
	if c == 0 || id < s.val0.id {
		return ret, false
	}
	if id == s.val0.id {
		return s.val0, true
	}
	if c == 1 {
		return ret, false
	}
	if c == 2 {
		v1 := s.extra.values[0]
		if v1.id == id {
			return v1, true
		}
		return ret, false
	}
	values := s.extra.values
	left, right := uint(1), uint(len(values))
	for left < right {
		mid := (left + right) >> 1
		midID := values[mid].id
		if midID > id {
			right = mid
		} else if midID < id {
			left = mid + 1
		} else {
			return values[mid], true
		}
	}
	return ret, false
}

func (s *_StackedMap) loadSlow(id _TypeID) (ret _Value, ok bool) {
	if s == nil {
		return ret, false
	}
	if extra := s.extra; extra != nil {
		if reset := extra.reset; reset != nil && reset.base != nil {
			if pair := reset.last.Load(); pair != nil && pair.id == id {
				return pair.val, true
			}
			v, found := reset.base.Load(id)
			if !found {
				return ret, false
			}
			return s.refreshValue(v), true
		}
	}

	for cur := s; cur != nil; cur = cur.Next {
		extra := cur.extra
		if extra == nil {
			if cur.count == 1 && cur.val0.id == id {
				return cur.val0, true
			}
			continue
		}
		if reset := extra.reset; reset != nil && reset.base == nil && containsID(*reset.ids, id) {
			v, found := cur.Next.Load(id)
			if !found {
				panic("impossible: refreshed type not found below its layer")
			}
			return cur.refreshValue(v), true
		}
		c := cur.count
		if c == 0 || id < cur.val0.id {
			continue
		}
		if c == 1 {
			if cur.val0.id == id {
				if cur == s && extra.hitVal.Load() == nil {
					s.storeHitPtr(&cur.val0)
				}
				return cur.val0, true
			}
			continue
		}
		if c == 2 {
			if id == cur.val0.id {
				if cur == s && extra.hitVal.Load() == nil {
					s.storeHitPtr(&cur.val0)
				}
				return cur.val0, true
			}
			if extra.values[0].id == id {
				if cur == s && extra.hitVal.Load() == nil {
					s.storeHitPtr(&extra.values[0])
				}
				return extra.values[0], true
			}
			continue
		}
		values := extra.values
		if idxPtr := extra.index; idxPtr != nil {
			positions := *idxPtr
			mask := uint64(len(positions) - 1)
			for slot := indexHashSlot(id, mask); ; slot = (slot + 1) & mask {
				pos := positions[slot]
				if pos == 0 {
					break
				}
				if vp := &values[pos-1]; vp.id == id {
					if cur == s && extra.hitVal.Load() == nil {
						s.storeHitPtr(vp)
					}
					return *vp, true
				}
			}
			continue
		}
		if id == cur.val0.id {
			if cur == s && extra.hitVal.Load() == nil {
				s.storeHitPtr(&values[0])
			}
			return values[0], true
		}
		left, right := uint(1), uint(len(values))
		for left < right {
			mid := (left + right) >> 1
			midID := values[mid].id
			if midID > id {
				right = mid
			} else if midID < id {
				left = mid + 1
			} else {
				if cur == s && extra.hitVal.Load() == nil {
					s.storeHitPtr(&values[mid])
				}
				return values[mid], true
			}
		}
	}
	return ret, false
}

// containsID reports whether the sorted ID slice holds id.
func containsID(ids []_TypeID, id _TypeID) bool {
	_, ok := slices.BinarySearch(ids, id)
	return ok
}

func (s *_StackedMap) refreshValue(v _Value) _Value {
	if v.initializer.DefIsPointer {
		return v
	}
	state := s.extra.reset
	prevPair := state.last.Load()
	if prevPair != nil && prevPair.key == v.initializer {
		if prevPair.id == v.id {
			return prevPair.val
		}
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

func (s *_StackedMap) forEachLayerValue(fn func(v _Value) bool) bool {
	switch s.count {
	case 0:
		return true
	case 1:
		return fn(s.val0)
	case 2:
		return fn(s.val0) && fn(s.extra.values[0])
	default:
		for _, v := range s.extra.values {
			if !fn(v) {
				return false
			}
		}
		return true
	}
}

func (s *_StackedMap) IterValues() iter.Seq[_Value] {
	if s != nil && s.isLazyReset() {
		resetLayer := s
		return func(yield func(_Value) bool) {
			for v := range resetLayer.extra.reset.base.IterValues() {
				if !yield(resetLayer.refreshValue(v)) {
					return
				}
			}
		}
	}
	return func(yield func(_Value) bool) {
		keys := make(map[_TypeID]struct{})
		var refreshers []*_StackedMap
		for cur := s; cur != nil; cur = cur.Next {
			stopped := !cur.forEachLayerValue(func(d _Value) bool {
				id := d.id
				if _, ok := keys[id]; ok {
					return true
				}
				keys[id] = struct{}{}
				for i := len(refreshers) - 1; i >= 0; i-- {
					if r := refreshers[i]; containsID(*r.extra.reset.ids, id) {
						d = r.refreshValue(d)
					}
				}
				return yield(d)
			})
			if stopped {
				return
			}
			if cur.extra != nil {
				if reset := cur.extra.reset; reset != nil && reset.base == nil {
					refreshers = append(refreshers, cur)
				}
			}
		}
	}
}

// iterMeta yields the effective _Value metadata entries of the stack without
// materialising reset initializers.
func (s *_StackedMap) iterMeta() iter.Seq[_Value] {
	return func(yield func(_Value) bool) {
		keys := make(map[_TypeID]struct{})
		for cur := s; cur != nil; {
			if cur.isLazyReset() {
				cur = cur.extra.reset.base
				continue
			}
			stopped := !cur.forEachLayerValue(func(d _Value) bool {
				id := d.id
				if _, ok := keys[id]; ok {
					return true
				}
				keys[id] = struct{}{}
				return yield(d)
			})
			if stopped {
				return
			}
			cur = cur.Next
		}
	}
}

// Append creates a new _StackedMap layer on top of the current one.
func (s *_StackedMap) Append(values []_Value) *_StackedMap {
	if s != nil && s.isLazyReset() {
		s = s.flatten()
	}
	layer := newStackedMapLayer(s, len(values))
	layer.copySortedValues(values)
	layer.buildIndex()
	return layer
}

func (s *_StackedMap) Len() int {
	if s == nil {
		return 0
	}
	if s.isLazyReset() {
		return s.extra.reset.base.Len()
	}
	ret := 0
	for s != nil {
		if s.extra != nil && s.extra.reset != nil {
			ret += len(*s.extra.reset.ids)
		}
		ret += int(s.count)
		s = s.Next
	}
	return ret
}

type _FlatLayer2 struct {
	layer _StackedMap
	extra _StackedMapExtra
	val1  [1]_Value
}

func (s *_StackedMap) flatten() *_StackedMap {
	if s == nil {
		return nil
	}
	if s.isLazyReset() {
		baseFlat := s.extra.reset.base.flatten()
		if baseFlat == nil || baseFlat.count == 0 {
			return newStackedMapLayer(nil, 0)
		}
		n := int(baseFlat.count)
		layer := newStackedMapLayer(nil, n)
		switch n {
		case 1:
			layer.val0 = s.refreshValue(baseFlat.val0)
		case 2:
			layer.val0 = s.refreshValue(baseFlat.val0)
			layer.extra.values[0] = s.refreshValue(baseFlat.extra.values[0])
		default:
			for i, v := range baseFlat.extra.values {
				rv := s.refreshValue(v)
				if i == 0 {
					layer.val0 = rv
				}
				layer.extra.values[i] = rv
			}
		}
		layer.buildIndex()
		return layer
	}

	var buf [32]_Value
	flatValues := buf[:0]
	var refreshersBuf [stackHeightLimit + 2]*_StackedMap
	refreshers := refreshersBuf[:0]

	for cur := s; cur != nil; cur = cur.Next {
		switch cur.count {
		case 0:
		case 1:
			flatValues = appendFlattenedLayerValue(flatValues, cur.val0, refreshers)
		case 2:
			flatValues = appendFlattenedLayerValue(flatValues, cur.val0, refreshers)
			flatValues = appendFlattenedLayerValue(flatValues, cur.extra.values[0], refreshers)
		default:
			for _, d := range cur.extra.values {
				flatValues = appendFlattenedLayerValue(flatValues, d, refreshers)
			}
		}
		if cur.extra != nil {
			if reset := cur.extra.reset; reset != nil && reset.base == nil {
				refreshers = append(refreshers, cur)
			}
		}
	}

	hb := s.hot.Load()
	if len(flatValues) == 2 {
		if flatValues[0].id > flatValues[1].id {
			flatValues[0], flatValues[1] = flatValues[1], flatValues[0]
		}
		b := new(_FlatLayer2)
		b.layer.Height = 1
		b.layer.count = 2
		b.layer.val0 = flatValues[0]
		b.val1[0] = flatValues[1]
		b.extra.values = b.val1[:]
		b.layer.extra = &b.extra
		if hb != nil && len(refreshers) == 0 {
			b.layer.hot.Store(hb)
		}
		return &b.layer
	}
	if len(flatValues) > 2 {
		slices.SortFunc(flatValues, func(a, b _Value) int {
			return cmp.Compare(a.id, b.id)
		})
	}
	layer := newStackedMapLayer(nil, len(flatValues))
	layer.copySortedValues(flatValues)
	layer.buildIndex()
	if hb != nil && len(refreshers) == 0 {
		layer.hot.Store(hb)
	}
	return layer
}

func appendFlattenedLayerValue(flatValues []_Value, d _Value, refreshers []*_StackedMap) []_Value {
	id := d.id
	for i := range flatValues {
		if flatValues[i].id == id {
			return flatValues
		}
	}
	for i := len(refreshers) - 1; i >= 0; i-- {
		if r := refreshers[i]; containsID(*r.extra.reset.ids, id) {
			d = r.refreshValue(d)
		}
	}
	return append(flatValues, d)
}
