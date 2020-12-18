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
	pt = fmt.Printf
	as = errors.As
	is = errors.Is
	we = e4.DefaultWrap
)
