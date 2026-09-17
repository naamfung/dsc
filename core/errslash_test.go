package core

import (
	"errors"
	"os"
	"testing"
)

// TestSlashErrText 断言文本级归一：Windows 原生路径（反斜杆）统一为正斜杆，
// 含 UNC 双反斜杆前缀；无反斜杆/已是正斜杆的文本零改动透传。
func TestSlashErrText(t *testing.T) {
	cases := []struct{ in, want string }{
		// 单反斜杆路径（os.* 错误内嵌形式）
		{`GetFileAttributesEx D:\Agents\pkg\src: The system cannot find the file specified.`,
			"GetFileAttributesEx D:/Agents/pkg/src: The system cannot find the file specified."},
		// UNC 双反斜杆前缀
		{`\\server\share\file.txt`, "//server/share/file.txt"},
		// 混合分隔符
		{`D:\mixed/slash\path`, "D:/mixed/slash/path"},
		// 无反斜杆：原样透传（含正斜杆路径与普通消息）
		{"GetFileAttributesEx D:/Agents/pkg/src: The system cannot find the file specified.",
			"GetFileAttributesEx D:/Agents/pkg/src: The system cannot find the file specified."},
		{"tool call timed out after 5000ms (TOOL_TIMEOUT)", "tool call timed out after 5000ms (TOOL_TIMEOUT)"},
		{"", ""},
	}
	for _, c := range cases {
		if got := SlashErrText(c.in); got != c.want {
			t.Errorf("SlashErrText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestSlashErrPathError 断言 *os.PathError 结构化归一：仅 Path 字段转正斜杆，
// Op/Err 原样保留，errors.Is 判断不中断。
func TestSlashErrPathError(t *testing.T) {
	inner := os.ErrNotExist
	err := &os.PathError{Op: "GetFileAttributesEx", Path: `D:\a\b.txt`, Err: inner}
	got := SlashErr(err)
	if got.Error() != "GetFileAttributesEx D:/a/b.txt: file does not exist" {
		t.Fatalf("SlashErr = %q, want %q", got.Error(), "GetFileAttributesEx D:/a/b.txt: file does not exist")
	}
	var pe *os.PathError
	if !errors.As(got, &pe) {
		t.Fatal("SlashErr should preserve *os.PathError type")
	}
	if pe.Op != "GetFileAttributesEx" || pe.Path != "D:/a/b.txt" || pe.Err != inner {
		t.Fatalf("fields changed: op=%q path=%q err=%v", pe.Op, pe.Path, pe.Err)
	}
	if !errors.Is(got, os.ErrNotExist) {
		t.Fatal("errors.Is chain broken after SlashErr")
	}
}

// TestSlashErrPassthrough 断言非归一场景原样返回同一实例：正斜杆 PathError、
// 非 PathError 错误、nil（错误包装链不被 SlashErr 破坏）。
func TestSlashErrPassthrough(t *testing.T) {
	if SlashErr(nil) != nil {
		t.Fatal("SlashErr(nil) should be nil")
	}
	unixErr := &os.PathError{Op: "stat", Path: "/a/b.txt", Err: os.ErrNotExist}
	if SlashErr(unixErr) != unixErr {
		t.Fatal("already-slashed PathError should return unchanged")
	}
	plain := errors.New("tool call denied by policy")
	if SlashErr(plain) != plain {
		t.Fatal("non-PathError should return unchanged")
	}
	wrapped := errors.Join(os.ErrPermission, errors.New(`denied D:\x`))
	if SlashErr(wrapped) != wrapped {
		t.Fatal("wrapped non-PathError should return unchanged (text-level slash is host exit)")
	}
}
