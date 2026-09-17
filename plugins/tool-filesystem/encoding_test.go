package main

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestEnsureUTF8ValidPassthrough 合法 UTF-8 原样返回（快速路径，不做任何改写）。
func TestEnsureUTF8ValidPassthrough(t *testing.T) {
	for _, s := range []string{"", "plain ascii", "中文 UTF-8 ✓ emoji 🎉", "line1\nline2\r\n"} {
		got := ensureUTF8(s)
		if got != s {
			t.Fatalf("ensureUTF8(%q) = %q, want unchanged", s, got)
		}
	}
}

// TestEnsureUTF8InvalidBytes 非法 UTF-8 字节必须被净化：结果合法 UTF-8，不再触发
// gRPC marshal 失败。净化形式与系统码页有关：Windows 上若 0x80 恰能被系统码页
// 解出字符（如 CP1252 的 €、CP437 的 Ç）则还原为对应字符，否则落到 U+FFFD——
// 契约是「绝不留非法字节、结果恒为合法 UTF-8」，不断言具体占位符。
func TestEnsureUTF8InvalidBytes(t *testing.T) {
	got := ensureUTF8("abc\x80def")
	if !utf8.ValidString(got) {
		t.Fatalf("ensureUTF8 output must be valid UTF-8, got %q", got)
	}
	if strings.Contains(got, "\x80") {
		t.Fatalf("非法字节不应残留, got %q", got)
	}
	if got == "abc\x80def" {
		t.Fatalf("非法输入不得原样返回, got %q", got)
	}
}

// TestEnsureUTF8GBKOnWindows 中文 Windows 控制台工具输出 GBK（OEM 936 码页）字节，
// 在 GBK 系统上应被还原为正确中文（而非 U+FFFD 乱码）；系统码页非 936 时跳过
// （本机非 zh-CN 时 GBK 字节本就不属于系统码页，U+FFFD 才是正确行为）。
// 夹具须为纯 GBK 字节流（真实工具输出整体按 OEM 码页编码，不会混入 UTF-8）：
// "目录列表：你好.txt" 的 GBK 编码。
func TestEnsureUTF8GBKOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("GBK 码页还原仅 Windows 有意义")
	}
	if systemOEMCodePage() != 936 {
		t.Skipf("系统 OEM 码页 %d ≠ 936（GBK），本机不解 GBK，跳过", systemOEMCodePage())
	}
	got := ensureUTF8("\xc4\xbf\xc2\xbc\xc1\xd0\xb1\xed\xa3\xba\xc4\xe3\xba\xc3.txt")
	if !utf8.ValidString(got) {
		t.Fatalf("输出必须为合法 UTF-8, got %q", got)
	}
	if !strings.Contains(got, "你好") {
		t.Fatalf("GBK 字节应还原为「你好」, got %q", got)
	}
}
