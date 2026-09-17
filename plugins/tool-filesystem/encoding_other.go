//go:build !windows

package main

// 非 Windows 平台：POSIX 系统的命令输出默认 UTF-8（locale 决定），不做码页
// 还原；非法字节统一由 ensureUTF8 的 U+FFFD 兜底处理。

func decodeSystemCodePage(string) (string, bool) {
	return "", false
}

// systemOEMCodePage 非 Windows 平台无码页概念，返回 0。
func systemOEMCodePage() uint16 {
	return 0
}
