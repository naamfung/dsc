package acp

import "time"

// timeNowMillis 返回当前 Unix 毫秒时间戳。
// 单独抽出便于测试时 mock（不引入全局变量避免并发问题）。
func timeNowMillis() int64 {
	return time.Now().UnixMilli()
}
