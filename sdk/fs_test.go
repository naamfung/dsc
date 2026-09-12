package dsc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	data, err := ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("content = %q, want hello", data)
	}
}

func TestReadFileMissingWrapsError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing.txt")
	_, err := ReadFile(p)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	// 错误统一正斜杆呈现（对齐内部 POSIX shell 风格）——Windows 上底层 os 错误
	// 含反斜杆路径，此处断言已归一且不含反斜杆
	if !strings.Contains(err.Error(), "read ") || !strings.Contains(err.Error(), filepath.ToSlash(p)) {
		t.Fatalf("err = %q, want wrapped 'read <slashed-path>'", err)
	}
	if strings.Contains(err.Error(), "\\") {
		t.Fatalf("err = %q, should not contain backslashes (POSIX style)", err)
	}
	// Unwrap 保留底层错误：errors.Is 仍可判定系统错误类别
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want errors.Is(err, os.ErrNotExist)", err)
	}
}

func TestWriteFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	if err := WriteFile(p, []byte("content")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "content" {
		t.Fatalf("content = %q, want content", data)
	}
}

func TestWriteFileMissingParentWrapsError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "no", "such", "dir", "out.txt")
	err := WriteFile(p, []byte("x"))
	if err == nil {
		t.Fatal("expected error for missing parent dir")
	}
	if !strings.Contains(err.Error(), "write ") || !strings.Contains(err.Error(), filepath.ToSlash(p)) {
		t.Fatalf("err = %q, want wrapped 'write <slashed-path>'", err)
	}
	if strings.Contains(err.Error(), "\\") {
		t.Fatalf("err = %q, should not contain backslashes (POSIX style)", err)
	}
}

func TestWriteFilePerm(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	if err := WriteFilePerm(p, []byte("s"), 0o600); err != nil {
		t.Fatalf("WriteFilePerm: %v", err)
	}
	if runtime.GOOS == "windows" {
		return // Windows 无 Unix 权限位，os.Stat 不反映 perm
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 600", info.Mode().Perm())
	}
}

func TestMkdirAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a", "b", "c")
	if err := MkdirAll(p); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected directory")
	}
}

func TestWorkspaceRoot(t *testing.T) {
	t.Setenv("DSC_WORKSPACE_ROOT", "")
	cwd, _ := os.Getwd()
	if got := WorkspaceRoot(); got != cwd {
		t.Fatalf("fallback = %q, want cwd %q", got, cwd)
	}
	t.Setenv("DSC_WORKSPACE_ROOT", "/tmp/ws")
	if got := WorkspaceRoot(); got != "/tmp/ws" {
		t.Fatalf("env = %q, want /tmp/ws", got)
	}
}

func TestAbsPath(t *testing.T) {
	dir := t.TempDir()
	rel := filepath.Join(dir, "x")
	abs, err := AbsPath(rel)
	if err != nil {
		t.Fatalf("AbsPath: %v", err)
	}
	if !filepath.IsAbs(abs) {
		t.Fatalf("abs = %q, want absolute", abs)
	}
}
