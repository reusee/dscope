// +build !js

package dscope

import (
	"fmt"
	"os"
)

func debugLog(format string, args ...any) { // NOCOVER
	fmt.Fprintf(os.Stderr, format, args...)
}
