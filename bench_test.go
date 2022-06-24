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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.Fork(
			func() T30 {
				return 42
			},
		)
	}

}

func BenchmarkCall(b *testing.B) {
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

	var t1 T1
	var t2 T2
	var t3 T3
	var t4 T4
	var t5 T5
	var t6 T6
	var t7 T7
	var t8 T8
	var t9 T9
	var t10 T10
	var t11 T11
	var t12 T12
	var t13 T13
	var t14 T14
	var t15 T15
	var t16 T16
	var t17 T17
	var t18 T18
	var t19 T19
	var t20 T20
	var t21 T21
	var t22 T22

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.Call(func(
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
		},
		).Assign(
			&t1,
			&t2,
			&t3,
			&t4,
			&t5,
			&t6,
			&t7,
			&t8,
			&t9,
			&t10,
			&t11,
			&t12,
			&t13,
			&t14,
			&t15,
			&t16,
			&t17,
			&t18,
			&t19,
			&t20,
			&t21,
			&t22,
		)
	}
}

func BenchmarkCallPointerProvider(b *testing.B) {
	t1 := T1(1)
	t2 := T2(2)
	t3 := T3(3)
	t4 := T4(4)
	t5 := T5(5)
	t6 := T6(6)
	t7 := T7(7)
	t8 := T8(8)
	t9 := T9(9)
	t10 := T10(10)
	t11 := T11(11)
	t12 := T12(12)
	t13 := T13(13)
	t14 := T14(14)
	t15 := T15(15)
	t16 := T16(16)
	t17 := T17(17)
	t18 := T18(18)
	t19 := T19(19)
	t20 := T20(20)
	t21 := T21(21)
	t22 := T22(22)
	scope := New().Fork(
		&t1,
		&t2,
		&t3,
		&t4,
		&t5,
		&t6,
		&t7,
		&t8,
		&t9,
		&t10,
		&t11,
		&t12,
		&t13,
		&t14,
		&t15,
		&t16,
		&t17,
		&t18,
		&t19,
		&t20,
		&t21,
		&t22,
	)
	var r1 T1
	var r2 T2
	var r3 T3
	var r4 T4
	var r5 T5
	var r6 T6
	var r7 T7
	var r8 T8
	var r9 T9
	var r10 T10
	var r11 T11
	var r12 T12
	var r13 T13
	var r14 T14
	var r15 T15
	var r16 T16
	var r17 T17
	var r18 T18
	var r19 T19
	var r20 T20
	var r21 T21
	var r22 T22

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.Call(func(
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
		},
		).Assign(
			&r1,
			&r2,
			&r3,
			&r4,
			&r5,
			&r6,
			&r7,
			&r8,
			&r9,
			&r10,
			&r11,
			&r12,
			&r13,
			&r14,
			&r15,
			&r16,
			&r17,
			&r18,
			&r19,
			&r20,
			&r21,
			&r22,
		)
	}
}

func BenchmarkSimpleCall(b *testing.B) {
	type Foo int
	f := Foo(42)
	scope := New(&f)
	var ret Foo
	for i := 0; i < b.N; i++ {
		scope.Call(func(
			f Foo,
		) Foo {
			return f
		}).Assign(&ret)
	}
}

func BenchmarkCallWithScope(b *testing.B) {
	scope := New()
	for i := 0; i < b.N; i++ {
		scope.Call(func(
			_ Scope,
		) {
		})
	}
}

func BenchmarkAssign(b *testing.B) {
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

	b.ResetTimer()
	var t30 T30
	for i := 0; i < b.N; i++ {
		scope.Assign(&t30)
	}

}

func BenchmarkFork(b *testing.B) {
	scope := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.Fork()
	}
}

func BenchmarkFork1(b *testing.B) {
	scope := New()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope.Fork(func() int {
			return 42
		})
	}
}

func BenchmarkGetTypeID(b *testing.B) {
	scopeType := reflect.TypeOf((*Scope)(nil)).Elem()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getTypeID(scopeType)
	}
}

func BenchmarkRecursiveFork(b *testing.B) {
	scope := New(
		func() int {
			return 42
		},
	)
	var n int
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scope = scope.Fork(
			func() string {
				return "42"
			},
		)
		scope.Assign(&n)
	}
}

type benchMultiFunc func()

var _ CustomReducer = benchMultiFunc(nil)

func (_ benchMultiFunc) Reduce(_ Scope, vs []reflect.Value) reflect.Value {
	return Reduce(vs)
}

func BenchmarkMultipleDispatch(b *testing.B) {
	s := New(
		func() benchMultiFunc {
			return func() {
			}
		},
		func() benchMultiFunc {
			return func() {
			}
		},
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Call(func(fn benchMultiFunc) {
			fn()
		})
	}
}
