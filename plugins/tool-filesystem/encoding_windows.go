//go:build windows

package main

// Windows：按系统 OEM/ANSI 码页把非 UTF-8 字节流严格解码为 UTF-8 文本。
// 中文 Windows 的控制台工具（tree.com、find.exe 等原生程序）以系统 OEM 码页
// （zh-CN 为 GBK/936）输出；先 OEM 后 ANSI，均以 MB_ERR_INVALID_CHARS 严格
// 校验——解不出合法文本即返回 false，交由调用方走 U+FFFD 兜底，绝不猜码页
// 猜出假文本。

import (
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

const (
	codePageACP   = 0 // 系统 ANSI 码页
	codePageOEMCP = 1 // 系统 OEM 码页（控制台工具的实际输出码页）

	mbErrInvalidChars = 0x00000008 // 遇非法字节序列即报错（严格模式）
)

// systemOEMCodePage 返回系统 OEM 码页（控制台工具的实际输出码页），供测试按
// 实际系统能力断言（如 zh-CN 的 936/GBK）。经 kernel32.GetOEMCP 惰性调用。
func systemOEMCodePage() uint16 {
	r, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP").Call()
	return uint16(r)
}

// decodeSystemCodePage 尝试以系统 OEM/ANSI 码页严格解码 s；任一码页成功即返回。
func decodeSystemCodePage(s string) (string, bool) {
	if len(s) == 0 {
		return "", false
	}
	b := []byte(s)
	for _, cp := range []uint32{codePageOEMCP, codePageACP} {
		if u16, ok := multibyteToUTF16(cp, b); ok {
			return string(utf16.Decode(u16)), true
		}
	}
	return "", false
}

// multibyteToUTF16 调 MultiByteToWideChar（严格模式）把多字节缓冲转为 UTF-16。
// 第一次调用传 nil 目标取所需长度，再分配后取实际结果（Win32 惯例两段式）。
func multibyteToUTF16(codePage uint32, b []byte) ([]uint16, bool) {
	pin := &b[0]
	n, err := windows.MultiByteToWideChar(codePage, mbErrInvalidChars, pin, int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return nil, false
	}
	u16 := make([]uint16, n)
	n, err = windows.MultiByteToWideChar(codePage, mbErrInvalidChars, pin, int32(len(b)), &u16[0], n)
	if err != nil || n <= 0 {
		return nil, false
	}
	return u16[:n], true
}
