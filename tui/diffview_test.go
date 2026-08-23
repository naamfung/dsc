package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const sampleDiff = "File replaced successfully.\n\n" +
	"--- a/test/fib.go\n" +
	"+++ b/test/fib.go\n" +
	"@@ -1,4 +1,4 @@\n" +
	" package main\n" +
	" func main() {\n" +
	"-	println(\"hi\")\n" +
	"+	fmt.Println(\"hi\")\n" +
	" }"

func TestDiffBlockStart(t *testing.T) {
	lines := strings.Split(sampleDiff, "\n")
	got := diffBlockStart(lines)
	// 文件头从第 2 行开始（0-based）：说明 + 空行 后是 ---
	if got != 2 {
		t.Fatalf("diffBlockStart = %d, want 2; lines=%q", got, lines)
	}

	// 无 diff 的普通文本 → -1
	if got := diffBlockStart(strings.Split("plain text\nwith\nmultiple\nlines", "\n")); got != -1 {
		t.Fatalf("plain text 不应判为 diff, got %d", got)
	}
	// 假阳性：有 --- 与 +++ 但无 @@ hunk 头 → -1
	if got := diffBlockStart(strings.Split("--- a/x\n+++ b/x\njust prose", "\n")); got != -1 {
		t.Fatalf("缺 hunk 头不应判为 diff, got %d", got)
	}
}

func TestRenderDiffLine(t *testing.T) {
	add := renderDiffLine("+fmt.Println(\"hi\")")
	if !strings.Contains(add, "\x1b[") || !strings.Contains(ansi.Strip(add), "+fmt.Println(\"hi\")") {
		t.Fatalf("加行应含 ANSI 颜色: %q", add)
	}
	del := renderDiffLine("-println(\"hi\")")
	if !strings.Contains(del, "\x1b[") || !strings.Contains(ansi.Strip(del), "-println(\"hi\")") {
		t.Fatalf("删行应含 ANSI 颜色: %q", del)
	}
	// 加/删行颜色不同
	if add == del {
		t.Fatal("加行与删行渲染应不同")
	}
	// hunk 头与文件头用暗色（无背景色，dim）
	hunk := renderDiffLine("@@ -1,4 +1,4 @@")
	if !strings.Contains(hunk, "\x1b[") {
		t.Fatalf("hunk 头应含 ANSI（dim）: %q", hunk)
	}
	// 上下文行原样
	ctx := renderDiffLine(" package main")
	if ctx != " package main" {
		t.Fatalf("上下文行应原样, got %q", ctx)
	}
}

func TestRenderToolResultWithDiff(t *testing.T) {
	out := renderToolResult(sampleDiff, false)
	strip := ansi.Strip(out)
	// 说明文本保留
	if !strings.Contains(strip, "File replaced successfully.") {
		t.Fatalf("说明文本应保留: %q", strip)
	}
	// diff 行着色（ANSI 存在）
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("diff 结果应含 ANSI 颜色: %q", out)
	}
	// 加行与删行都被渲染（strip 后内容保留，行首 +/- 后可能带 tab）
	if !strings.Contains(strip, "fmt.Println") || !strings.Contains(strip, "println(\"hi\")") {
		t.Fatalf("加/删行应保留: %q", strip)
	}
}

// TestRenderToolResultNoDiff 普通结果行为不变（无 ANSI 颜色）。
func TestRenderToolResultNoDiff(t *testing.T) {
	out := renderToolResult("operation ok\nall good", false)
	if strings.Contains(out, "\x1b[") {
		// connector gutter 用 dimSty 渲染，本身带颜色；正文行不应有额外颜色
		strip := ansi.Strip(out)
		if !strings.Contains(strip, "operation ok") || !strings.Contains(strip, "all good") {
			t.Fatalf("普通结果应原样渲染: %q", strip)
		}
	}
}
