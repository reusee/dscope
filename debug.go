// +build !js

package dscope

import (
	"fmt"
	"os"
)

func debugLog(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}
