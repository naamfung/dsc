// pathx 提供 plugin（本仓维护的 go-plugin fork）内部的 POSIX 路径等价实现。
//
// plugin 是独立模块（github.com/hashicorp/go-plugin），无法引入 dsc/core 的
// P* 系列，故在本模块内提供同语义的本地等价物：先以原生 filepath 计算，再统一
// 归一化为正斜杠。此文件是 plugin 模块内唯一允许直接调用被禁 filepath 函数的
// 文件（core/filepath_guard_test.go 白名单）。
package pathx

import (
	"path/filepath"
)

// Join 等价 filepath.Join，结果归一化为正斜杠。
func Join(elem ...string) string {
	return filepath.ToSlash(filepath.Join(elem...))
}

// Abs 等价 filepath.Abs，结果归一化为正斜杠。
func Abs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(abs), nil
}

// Glob 等价 filepath.Glob，匹配结果逐条归一化为正斜杠。
func Glob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	for i, m := range matches {
		matches[i] = filepath.ToSlash(m)
	}
	return matches, nil
}
