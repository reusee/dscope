package dscope

import (
	"bytes"
	"testing"
)

func TestVisualize(t *testing.T) {

	s := New(func(
		i8 int8,
		i32 int32,
	) int {
		return int(i8) + int(i32)
	}, func(
		u8 uint8,
		u32 uint32,
		i int,
	) int64 {
		return int64(u8) + int64(u32) + int64(i)
	}, func(
		i16 int16,
	) (
		i8 int8,
		i32 int32,
		u8 uint8,
		u32 uint32,
	) {
		i8 = int8(i16)
		i32 = int32(i16)
		u8 = uint8(i16)
		u32 = uint32(i16)
		return
	}, func() int16 {
		return 42
	})

	buf := new(bytes.Buffer)
	s.Visualize(buf)

}
