package dsc

import (
	"fmt"
	"os"
	"runtime/debug"
)

// SafeGoroutine 启动一个受保护的 goroutine：内部 panic 被 recover 并连同调用栈
// 打到 stderr（go-plugin 惯例：插件 stderr 由宿主收集，便于定位根因），绝不
// crash 插件进程导致 LLM 连接中断——goroutine 里的 panic 无法被工具执行链路的
// recover 捕获（recover 只对当前 goroutine 有效），不在此接住会直接干掉整个
// 插件进程。
//
// 所有插件的后台 goroutine 一律经此启动，禁止裸 go func()。fn 无参无返回值，
// 需要传递参数/上下文时用闭包捕获。
func SafeGoroutine(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "dsc-sdk: goroutine panicked: %v\n%s", r, debug.Stack())
			}
		}()
		fn()
	}()
}
