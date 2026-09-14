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

// TestEnsureUTF8InvalidBytes 非法 UTF-8 字节必须被净化：结果合法 UTF-8，
// 不再触发 gRPC marshal 失败；净化的形式为 U+FFFD 占位（Windows 上若字节
// 恰能按系统码页解出文本则还原，本例的 0x80 单独出现、任何码页都解不出，
// 两平台行为一致落到 U+FFFD 兜底）。
func TestEnsureUTF8InvalidBytes(t *testing.T) {
	got := ensureUTF8("abc\x80def")
	if !utf8.ValidString(got) {
		t.Fatalf("ensureUTF8 output must be valid UTF-8, got %q", got)
	}
	if !strings.Contains(got, "\uFFFD") {
		t.Fatalf("非法字节应退化为 U+FFFD, got %q", got)
	}
	if strings.Contains(got, "\x80") {
		t.Fatalf("非法字节不应残留, got %q", got)
	}
}

// TestEnsureUTF8GBKOnWindows 中文 Windows 控制台工具输出 GBK（OEM 936 码页）字节，
// 在 Windows 上应被还原为正确中文（而非 U+FFFD 乱码）；其他平台跳过。
// "你好" 的 GBK 编码为 C4 E3 BA C3。
func TestEnsureUTF8GBKOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("GBK 码页还原仅 Windows 有意义")
	}
	got := ensureUTF8("目录列表：\xc4\xe3\xba\xc3.txt")
	if !utf8.ValidString(got) {
		t.Fatalf("输出必须为合法 UTF-8, got %q", got)
	}
	if !strings.Contains(got, "你好") {
		t.Fatalf("GBK 字节应还原为「你好」, got %q", got)
	}
}
