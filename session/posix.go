package session

import "path/filepath"

// posix 路径拼接/父目录（结果归一化为正斜杠）。session 包与 core 存在包级
// import 环（core 引用 session 存储、session 不引用 core），无法复用 core.P*，
// 故在本包内提供等价实现；本文件是 session 包内唯一允许直接调用被禁 filepath
// 函数的文件（core/filepath_guard_test.go 白名单）。
func posixJoin(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

func posixDir(path string) string {
	return filepath.ToSlash(filepath.Dir(path))
}
