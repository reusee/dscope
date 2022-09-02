package dscope

import (
	"errors"
	"fmt"

	"github.com/reusee/e5"
)

type (
	any = interface{}
)

var (
	pt    = fmt.Printf
	as    = errors.As
	is    = errors.Is
	he    = e5.Handle
	we    = e5.Wrap
	throw = e5.Throw
)
