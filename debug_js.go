package dscope

import (
	"fmt"
	"syscall/js"
)

var (
	Global  = js.Global()
	Console = Global.Get("console")
)

func debugLog(format string, args ...any) {
	Console.Call("log", fmt.Sprintf(format, args...))
}
