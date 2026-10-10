package dscope

import (
	"reflect"
	"testing"
)

type (
	T1  int
	T2  int
	T3  int
	T4  int
	T5  int
	T6  int
	T7  int
	T8  int
	T9  int
	T10 int
	T11 int
	T12 int
	T13 int
	T14 int
	T15 int
	T16 int
	T17 int
	T18 int
	T19 int
	T20 int
	T21 int
	T22 int
	T23 int
	T24 int
	T25 int
	T26 int
	T27 int
	T28 int
	T29 int
	T30 int
)

func BenchmarkForkWithNewDeps(b *testing.B) {
	scope := New().Fork(
		func() T1 { return 42 },
		func(t1 T1) T2 { return T2(t1) },
		func(t2 T2) T3 { return T3(t2) },
		func(t3 T3) T4 { return T4(t3) },
		func(t4 T4) T5 { return T5(t4) },
		func(t5 T5) T6 { return T6(t5) },
		func(t6 T6) T7 { return T7(t6) },
		func(t7 T7) T8 { return T8(t7) },
		func(t8 T8) T9 { return T9(t8) },
		func(t9 T9) T10 { return T10(t9) },
		func(t10 T10) T11 { return T11(t10) },
		func(t11 T11) T12 { return T12(t11) },
		func(t12 T12) T13 { return T13(t12) },
		func(t13 T13) T14 { return T14(t13) },
		func(t14 T14) T15 { return T15(t14) },
		func(t15 T15) T16 { return T16(t15) },
		func(t16 T16) T17 { return T17(t16) },
		func(t17 T17) T18 { return T18(t17) },
		func(t18 T18) T19 { return T19(t18) },
		func(t19 T19) T20 { return T20(t19) },
		func(t20 T20) T21 { return T21(t20) },
		func(t21 T21) T22 { return T22(t21) },
		func(t22 T22) T23 { return T23(t22) },
		func(t23 T23) T24 { return T24(t23) },
		func(t24 T24) T25 { return T25(t24) },
		func(t25 T25) T26 { return T26(t25) },
		func(t26 T26) T27 { return T27(t26) },
		func(t27 T27) T28 { return T28(t27) },
		func(t28 T28) T29 { return T29(t28) },
		func(t29 T29) T30 { return T30(t29) },
	)

	type S string

	for b.Loop() {
		scope.Fork(
			func(s S) T2 {
				return 29
			},
			func() S {
				return "42"
			},
		)
	}

}

func BenchmarkForkWithoutNewDeps(b *testing.B) {
	scope := New().Fork(
		func() T1 { return 42 },
		func(t1 T1) T2 { return T2(t1) },
		func(t2 T2) T3 { return T3(t2) },
		func(t3 T3) T4 { return T4(t3) },
		func(t4 T4) T5 { return T5(t4) },
		func(t5 T5) T6 { return T6(t5) },
		func(t6 T6) T7 { return T7(t6) },
		func(t7 T7) T8 { return T8(t7) },
		func(t8 T8) T9 { return T9(t8) },
		func(t9 T9) T10 { return T10(t9) },
		func(t10 T10) T11 { return T11(t10) },
		func(t11 T11) T12 { return T12(t11) },
		func(t12 T12) T13 { return T13(t12) },
		func(t13 T13) T14 { return T14(t13) },
		func(t14 T14) T15 { return T15(t14) },
		func(t15 T15) T16 { return T16(t15) },
		func(t16 T16) T17 { return T17(t16) },
		func(t17 T17) T18 { return T18(t17) },
		func(t18 T18) T19 { return T19(t18) },
		func(t19 T19) T20 { return T20(t19) },
		func(t20 T20) T21 { return T21(t20) },
		func(t21 T21) T22 { return T22(t21) },
		func(t22 T22) T23 { return T23(t22) },
		func(t23 T23) T24 { return T24(t23) },
		func(t24 T24) T25 { return T25(t24) },
		func(t25 T25) T26 { return T26(t25) },
		func(t26 T26) T27 { return T27(t26) },
		func(t27 T27) T28 { return T28(t27) },
		func(t28 T28) T29 { return T29(t28) },
		func(t29 T29) T30 { return T30(t29) },
	)

	for b.Loop() {
		scope.Fork(
			func() T30 {
				return 42
			},
		)
	}

}

func BenchmarkGetMany(b *testing.B) {
	scope := New().Fork(
		func() T1 { return 1 },
		func() T2 { return 2 },
		func() T3 { return 3 },
		func() T4 { return 4 },
		func() T5 { return 5 },
		func() T6 { return 6 },
		func() T7 { return 7 },
		func() T8 { return 8 },
		func() T9 { return 9 },
		func() T10 { return 10 },
		func() T11 { return 11 },
		func() T12 { return 12 },
		func() T13 { return 13 },
		func() T14 { return 14 },
		func() T15 { return 15 },
		func() T16 { return 16 },
		func() T17 { return 17 },
		func() T18 { return 18 },
		func() T19 { return 19 },
		func() T20 { return 20 },
		func() T21 { return 21 },
		func() T22 { return 22 },
	)

	for b.Loop() {
		func(
			t1 T1,
			t2 T2,
			t3 T3,
			t4 T4,
			t5 T5,
			t6 T6,
			t7 T7,
			t8 T8,
			t9 T9,
			t10 T10,
			t11 T11,
			t12 T12,
			t13 T13,
			t14 T14,
			t15 T15,
			t16 T16,
			t17 T17,
			t18 T18,
			t19 T19,
			t20 T20,
			t21 T21,
			t22 T22,
		) (
			T1,
			T2,
			T3,
			T4,
			T5,
			T6,
			T7,
			T8,
			T9,
			T10,
			T11,
			T12,
			T13,
			T14,
			T15,
			T16,
			T17,
			T18,
			T19,
			T20,
			T21,
			T22,
		) {
			return t1,
				t2,
				t3,
				t4,
				t5,
				t6,
				t7,
				t8,
				t9,
				t10,
				t11,
				t12,
				t13,
				t14,
				t15,
				t16,
				t17,
				t18,
				t19,
				t20,

				t21,
				t22
		}(
			scope.Get[T1](),
			scope.Get[T2](),
			scope.Get[T3](),
			scope.Get[T4](),
			scope.Get[T5](),
			scope.Get[T6](),
			scope.Get[T7](),
			scope.Get[T8](),
			scope.Get[T9](),
			scope.Get[T10](),
			scope.Get[T11](),
			scope.Get[T12](),
			scope.Get[T13](),
			scope.Get[T14](),
			scope.Get[T15](),
			scope.Get[T16](),
			scope.Get[T17](),
			scope.Get[T18](),
			scope.Get[T19](),
			scope.Get[T20](),
			scope.Get[T21](),
			scope.Get[T22](),
		)

	}
}

var assignBenchDefs = []any{
	func() T1 { return 42 },
	func(t1 T1) T2 { return T2(t1) },
	func(t2 T2) T3 { return T3(t2) },
	func(t3 T3) T4 { return T4(t3) },
	func(t4 T4) T5 { return T5(t4) },
	func(t5 T5) T6 { return T6(t5) },
	func(t6 T6) T7 { return T7(t6) },
	func(t7 T7) T8 { return T8(t7) },
	func(t8 T8) T9 { return T9(t8) },
	func(t9 T9) T10 { return T10(t9) },
	func(t10 T10) T11 { return T11(t10) },
	func(t11 T11) T12 { return T12(t11) },
	func(t12 T12) T13 { return T13(t12) },
	func(t13 T13) T14 { return T14(t13) },
	func(t14 T14) T15 { return T15(t14) },
	func(t15 T15) T16 { return T16(t15) },
	func(t16 T16) T17 { return T17(t16) },
	func(t17 T17) T18 { return T18(t17) },
	func(t18 T18) T19 { return T19(t18) },
	func(t19 T19) T20 { return T20(t19) },
	func(t20 T20) T21 { return T21(t20) },
	func(t21 T21) T22 { return T22(t21) },
	func(t22 T22) T23 { return T23(t22) },
	func(t23 T23) T24 { return T24(t23) },
	func(t24 T24) T25 { return T25(t24) },
	func(t25 T25) T26 { return T26(t25) },
	func(t26 T26) T27 { return T27(t26) },
	func(t27 T27) T28 { return T28(t27) },
	func(t28 T28) T29 { return T29(t28) },
	func(t29 T29) T30 { return T30(t29) },
}

func BenchmarkAssign(b *testing.B) {
	scope := New().Fork(assignBenchDefs...)

	var t30 T30
	for b.Loop() {
		scope.Assign(&t30)
	}
}

func BenchmarkFork(b *testing.B) {
	scope := New()

	for b.Loop() {
		scope.Fork()
	}
}

func BenchmarkFork1(b *testing.B) {
	scope := New()

	for b.Loop() {
		scope.Fork(func() int {
			return 42
		})
	}
}

func BenchmarkFork2(b *testing.B) {
	scope := New()

	for b.Loop() {
		scope.Fork(func() int {
			return 42
		}, func() string {
			return "foo"
		})
	}
}

func BenchmarkGenericGet(b *testing.B) {
	s := New(func() int {
		return 42
	})

	for b.Loop() {
		_ = s.Get[int]()
	}
}

func BenchmarkGetTypeID(b *testing.B) {
	scopeType := reflect.TypeFor[Scope]()

	for b.Loop() {
		getTypeID(scopeType)
	}
}

func BenchmarkGetTypeIDParallel(b *testing.B) {
	scopeType := reflect.TypeFor[Scope]()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			getTypeID(scopeType)
		}
	})
}

func BenchmarkRecursiveFork(b *testing.B) {
	scope := New(
		func() int {
			return 42
		},
	)
	var n int

	for b.Loop() {
		scope = scope.Fork(
			func() string {
				return "42"
			},
		)
		scope.Assign(&n)
	}
}

func BenchmarkForkNewType(b *testing.B) {
	for b.Loop() {
		type T int
		Universe.Fork(func() T {
			return T(0)
		})
	}
}

// BenchmarkTypeIDSlot isolates the slot computation of the type ID cache, the
// part the most-recent-entry cache skips on a hit.
func BenchmarkTypeIDSlot(b *testing.B) {
	t := reflect.TypeFor[Scope]()
	for b.Loop() {
		if _, ok := typeIDCacheSlot(t); !ok {
			b.Fatal("type is not pointer shaped")
		}
	}
}

// benchSink keeps a benchmark result reachable, so the compiler cannot drop the
// work a micro benchmark measures.
var benchSink any

// BenchmarkNewStackedMapLayer isolates the allocation of one fork layer.
func BenchmarkNewStackedMapLayer(b *testing.B) {
	for b.Loop() {
		benchSink = newStackedMapLayer(nil, 1)
	}
}

// BenchmarkNewInitializerFunc isolates the creation of an initializer for a
// function definition.
func BenchmarkNewInitializerFunc(b *testing.B) {
	def := func() int { return 42 }
	for b.Loop() {
		benchSink = newInitializer(def, false)
	}
}

// BenchmarkForkApply isolates applying a prebuilt forker to a base scope.
func BenchmarkForkApply(b *testing.B) {
	scope := New()
	forker := newForker(scope, forkBenchDefs)
	for b.Loop() {
		_ = forker.Fork(scope, forkBenchDefs)
	}
}

// BenchmarkNewForker isolates the per-shape analysis of a fork, the part the
// forker cache skips on a hit.
func BenchmarkNewForker(b *testing.B) {
	scope := New()
	for b.Loop() {
		_ = newForker(scope, forkBenchDefs)
	}
}

// BenchmarkForkLookup isolates the lookup of a cached forker.
func BenchmarkForkLookup(b *testing.B) {
	scope := New()
	key := forkKey(scope.signature, forkBenchDefs)
	for b.Loop() {
		if _, ok := forkers.Load(key); !ok {
			b.Fatal("forker is not cached")
		}
	}
}

// BenchmarkForkKey isolates the derivation of the fork cache key.
func BenchmarkForkKey(b *testing.B) {
	scope := New()
	for b.Loop() {
		_ = forkKey(scope.signature, forkBenchDefs)
	}
}

// forkBenchDefs is the single definition the fork cost microbenchmarks use.
var forkBenchDefs = []any{func() int { return 42 }}
