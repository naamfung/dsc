package main

// shell 输出的 UTF-8 净化：gRPC/proto 对 string 字段强制合法 UTF-8，原样透传
// 非法字节会使整个工具结果 marshal 失败（模型只收到 "grpc: error while
// marshaling: string field contains invalid UTF-8"，输出全部丢失）。真实案例：
// 中文 Windows 上 `tree` 命中 C:\Windows\tree.com，其输出为系统 OEM 码页（GBK）
// 编码，原样进入工具结果即触发 marshal 失败。
//
// 净化分两层：
//  1. Windows 上先尝试按系统 OEM/ANSI 码页严格解码（MB_ERR_INVALID_CHARS，
//     解不出合法文本即放弃）——中文 Windows 的控制台工具输出因此可还原为正确
//     中文而非乱码；
//  2. 兜底 strings.ToValidUTF8——任何平台、任何来源的非法字节最多退化为
//     U+FFFD 占位符，绝不再让整个工具结果被 gRPC 拒发。

import (
	"runtime"
	"strings"
	"unicode/utf8"
)

// ensureUTF8 把可能含非法 UTF-8 的命令输出净化为合法 UTF-8 字符串：
// 已合法则原样返回（零开销快速路径）；Windows 上先按系统码页尽力还原；
// 仍失败才以 U+FFFD 逐字节替换。
func ensureUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	if runtime.GOOS == "windows" {
		if dec, ok := decodeSystemCodePage(s); ok && utf8.ValidString(dec) {
			return dec
		}
	}
	return strings.ToValidUTF8(s, string(utf8.RuneError))
}
