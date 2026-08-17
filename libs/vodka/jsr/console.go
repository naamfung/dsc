package jsr

import (
	"fmt"
)

// func log(call js.FunctionCall) js.Value {
// 	str := call.Argument(0)
// 	fmt.Println(str.String())
// 	return str
// }

// RegisterConsole register a console.log to runtime
func RegisterConsole(c *Core) {
	r := c.GetRts()
	o := r.NewObject()
	o.Set("log", fmt.Println)
	r.Set("console", o)
}
