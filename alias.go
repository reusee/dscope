package dscope

import (
	"errors"
	"fmt"

	"github.com/reusee/e4"
)

type (
	any = interface{}
)

var (
	pt    = fmt.Printf
	as    = errors.As
	is    = errors.Is
	he    = e4.Handle
	we    = e4.Wrap
	throw = e4.Throw
)
