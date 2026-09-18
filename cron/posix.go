package cron

import "path/filepath"

// posixJoin 拼接路径（结果归一化为正斜杆）。cron 包与 core 存在包级 import 环
// （core 引用 cron 调度、cron 不引用 core），无法复用 core.P*，故在本包内提供
// 等价实现；本文件是 cron 包内唯一允许直接调用被禁 filepath 函数的文件
// （core/filepath_guard_test.go 白名单）。
func posixJoin(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}
