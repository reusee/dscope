package dscope

import (
	"cmp"
	"iter"
	"slices"
	"sync"
)

// _StackedMap implements an immutable, singly-linked list acting as a stack of
// key-value maps. Each node holds a batch of sorted _Value entries. Searches
// start from the head.
//
// When ResetBase is non-nil the node acts as a lazy reset layer: all lookups
// delegate to ResetBase but every _Initializer is replaced with a fresh copy so
// that providers are re-evaluated on first access. Fresh initializers are cached
// in ResetCache, preserving the at-most-once evaluation guarantee per scope.
//
// When Refresh is set the node acts as a partial reset layer: RefreshIDs lists
// the sorted inherited types whose recomputation the node invalidates, and the
// node also holds the values a Fork adds. Resolving a marked type searches below
// the node and replaces the initializer with a fresh copy from ResetCache, so a
// Fork re-evaluates the affected types on first access and only those. A type
// the node provides itself is never marked.
//
// A layer with few values keeps them in itself instead of a separate slice, so
// the layer of a typical Fork costs one allocation.
type _StackedMap struct {
	Next       *_StackedMap // Previous layer in the stack.
	Values     []_Value     // Values in this layer, sorted by TypeID.
	Height     int          // Height of the stack from this node downwards.
	ResetBase  *_StackedMap // Non-nil for lazy reset layers; delegates to this base.
	Refresh    bool         // Set for a partial reset layer; the types in RefreshIDs are refreshed on access.
	RefreshIDs []_TypeID    // Sorted TypeIDs a partial reset layer refreshes; the slice belongs to the _Forker.
	ResetCache *sync.Map    // Caches the fresh initializers of a reset layer, keyed by the inherited initializer.
	// inline backs Values when the layer holds few values, so such a layer
	// needs no separate value slice.
	inline [stackedMapInlineValues]_Value
}

// stackedMapInlineValues is the number of values a layer holds in itself. A Fork
// usually adds a few definitions, and most layers never grow.
const stackedMapInlineValues = 2

// newStackedMapLayer creates an empty layer that sits on top of base. The caller
// fills Values with values sorted by TypeID.
func newStackedMapLayer(base *_StackedMap, n int) *_StackedMap {
	layer := &_StackedMap{
		Next:   base,
		Height: 1,
	}
	if base != nil {
		layer.Height = base.Height + 1
	}
	if n <= stackedMapInlineValues {
		layer.Values = layer.inline[:n]
	} else {
		layer.Values = make([]_Value, n)
	}
	return layer
}

// markRefresh turns the layer into a partial reset layer: the inherited types in
// ids are refreshed on access. The layer keeps no initializer for a marked type,
// so a fresh initializer comes into existence in the layer's cache only when the
// type is actually resolved. ids must be sorted, belong to the scope below the
// layer, and be disjoint from the types of the layer's own values.
func (s *_StackedMap) markRefresh(ids []_TypeID) {
	s.Refresh = true
	s.RefreshIDs = ids
	s.ResetCache = new(sync.Map)
}

// Load finds the value with the specified TypeID.
func (s *_StackedMap) Load(id _TypeID) (ret _Value, ok bool) {
	if s != nil && s.ResetBase != nil {
		v, found := s.ResetBase.Load(id)
		if !found {
			return ret, false
		}
		return s.refreshValue(v), true
	}

	for cur := s; cur != nil; cur = cur.Next {
		if cur.Refresh && containsID(cur.RefreshIDs, id) {
			// A partial reset layer resolves the inherited value below itself
			// and hands out a fresh initializer, so the provider re-evaluates
			// in this scope.
			v, found := cur.Next.Load(id)
			if !found {
				panic("impossible: refreshed type not found below its layer")
			}
			return cur.refreshValue(v), true
		}

		values := cur.Values
		l := uint(len(values))
		if l == 0 {
			continue
		}

		// Binary search
		left, right := uint(0), l
		for left < right {
			mid := (left + right) >> 1
			midID := values[mid].typeInfo.TypeID
			if midID > id {
				right = mid
			} else if midID < id {
				left = mid + 1
			} else {
				return values[mid], true // Found
			}
		}
	}
	return // Not found
}

// containsID reports whether the sorted ID slice holds id.
func containsID(ids []_TypeID, id _TypeID) bool {
	_, ok := slices.BinarySearch(ids, id)
	return ok
}

// refreshValue returns v with a fresh initializer, cached per reset layer.
// Pointer initializers are returned as-is because they never need re-evaluation.
func (s *_StackedMap) refreshValue(v _Value) _Value {
	if v.initializer.DefIsPointer {
		return v
	}
	if cached, ok := s.ResetCache.Load(v.initializer); ok {
		return _Value{
			typeInfo:    v.typeInfo,
			initializer: cached.(*_Initializer),
		}
	}
	actual, _ := s.ResetCache.LoadOrStore(v.initializer, v.initializer.reset())
	return _Value{
		typeInfo:    v.typeInfo,
		initializer: actual.(*_Initializer),
	}
}

func (s *_StackedMap) IterValues() iter.Seq[_Value] {
	if s != nil && s.ResetBase != nil {
		resetLayer := s
		return func(yield func(_Value) bool) {
			for v := range resetLayer.ResetBase.IterValues() {
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
				id := d.typeInfo.TypeID
				if _, ok := keys[id]; ok {
					continue
				}
				keys[id] = struct{}{}
				for i := len(refreshers) - 1; i >= 0; i-- {
					if r := refreshers[i]; containsID(r.RefreshIDs, id) {
						d = r.refreshValue(d)
					}
				}
				if !yield(d) {
					return
				}
			}
			if cur.Refresh {
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
	if s != nil && s.ResetBase != nil {
		s = s.flatten()
	}
	layer := newStackedMapLayer(s, len(values))
	copy(layer.Values, values)
	return layer
}

// AppendRefresh appends the layer of a partial reset: it holds the values a Fork
// adds and marks the inherited types in ids for re-evaluation. values must be
// sorted by TypeID; ids must be sorted, belong to the scope below, and be
// disjoint from the types of values.
func (s *_StackedMap) AppendRefresh(values []_Value, ids []_TypeID) *_StackedMap {
	layer := s.Append(values)
	layer.markRefresh(ids)
	return layer
}

// Len returns the number of value entries the stack tracks: the values of each
// layer plus the inherited types a partial reset layer marks for refresh.
func (s *_StackedMap) Len() int {
	if s == nil {
		return 0
	}
	if s.ResetBase != nil {
		return s.ResetBase.Len()
	}
	ret := 0
	for s != nil {
		ret += len(s.Values) + len(s.RefreshIDs)
		s = s.Next
	}
	return ret
}

// flatten materialises the effective values of the stack into a single sorted
// layer. Effective values and override semantics are preserved.
func (s *_StackedMap) flatten() *_StackedMap {
	var flatValues []_Value
	for value := range s.IterValues() {
		flatValues = append(flatValues, value)
	}
	slices.SortFunc(flatValues, func(a, b _Value) int {
		return cmp.Compare(a.typeInfo.TypeID, b.typeInfo.TypeID)
	})
	layer := newStackedMapLayer(nil, len(flatValues))
	copy(layer.Values, flatValues)
	return layer
}
