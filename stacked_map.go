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
// When Refresh is set the node acts as a partial reset layer: its values carry
// the initializers inherited from lower layers, and only a value that is
// actually resolved is replaced with a fresh copy from ResetCache. Fork uses
// this mode so that a new scope re-evaluates the overridden types and their
// dependents, and only those values the scope touches.
type _StackedMap struct {
	Next       *_StackedMap // Previous layer in the stack.
	Values     []_Value     // Values in this layer, sorted by TypeID.
	Height     int          // Height of the stack from this node downwards.
	ResetBase  *_StackedMap // Non-nil for lazy reset layers; delegates to this base.
	Refresh    bool         // Set for a partial reset layer; refresh values on access.
	ResetCache *sync.Map    // Caches fresh initializers (initializer ID -> *_Initializer).
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

	for s != nil {
		values := s.Values
		l := uint(len(values))
		if l == 0 {
			s = s.Next
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
				if s.Refresh {
					// A partial reset layer hands out a fresh initializer for
					// the value, so the provider re-evaluates in this scope.
					return s.refreshValue(values[mid]), true
				}
				return values[mid], true // Found
			}
		}
		s = s.Next
	}
	return // Not found
}

// refreshValue returns v with a fresh initializer, cached per reset layer.
// Pointer initializers are returned as-is because they never need re-evaluation.
func (s *_StackedMap) refreshValue(v _Value) _Value {
	if v.initializer.DefIsPointer {
		return v
	}
	if cached, ok := s.ResetCache.Load(v.initializer.ID); ok {
		return _Value{
			typeInfo:    v.typeInfo,
			initializer: cached.(*_Initializer),
		}
	}
	actual, _ := s.ResetCache.LoadOrStore(v.initializer.ID, v.initializer.reset())
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
		for cur := s; cur != nil; cur = cur.Next {
			for _, d := range cur.Values {
				if _, ok := keys[d.typeInfo.TypeID]; ok {
					continue
				}
				keys[d.typeInfo.TypeID] = struct{}{}
				if cur.Refresh {
					// A partial reset layer yields the refreshed value, so
					// flattening keeps the reset semantics.
					d = cur.refreshValue(d)
				}
				if !yield(d) {
					return
				}
			}
		}
	}
}

// Append creates a new _StackedMap layer on top of the current one.
// The provided values must be pre-sorted by TypeID.
//
// If the receiver is a lazy reset layer it is first materialised into a flat
// sorted stack so that subsequent binary searches remain correct.
func (s *_StackedMap) Append(values []_Value) *_StackedMap {
	if s != nil && s.ResetBase != nil {
		return s.flatten().Append(values)
	}
	var height int = 1
	if s != nil {
		height = s.Height + 1
	}
	return &_StackedMap{
		Values: values,
		Next:   s,
		Height: height,
	}
}

// AppendRefresh appends a layer that replaces the initializers of inherited
// values with fresh ones. The values in the layer keep their inherited
// initializers: the fresh initializer of a value comes into existence in the
// layer's cache only when the value is actually resolved, so a Fork pays for the
// values its scope touches and no more. The provided values must be pre-sorted
// by TypeID.
func (s *_StackedMap) AppendRefresh(values []_Value) *_StackedMap {
	ret := s.Append(values)
	ret.Refresh = true
	ret.ResetCache = new(sync.Map)
	return ret
}

// Len returns the total number of individual _Value entries across all layers.
func (s *_StackedMap) Len() int {
	if s == nil {
		return 0
	}
	if s.ResetBase != nil {
		return s.ResetBase.Len()
	}
	ret := 0
	for s != nil {
		ret += len(s.Values)
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
	return &_StackedMap{
		Values: flatValues,
		Height: 1,
	}
}
