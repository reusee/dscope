package dscope

import (
	"errors"
	"fmt"
)

type (
	any = interface{}
)

var (
	pt = fmt.Printf
	as = errors.As
)
