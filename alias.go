package dscope

import (
	"errors"

	"github.com/reusee/e5"
)

var (
	is    = errors.Is
	we    = e5.Wrap
	throw = e5.Throw
)
