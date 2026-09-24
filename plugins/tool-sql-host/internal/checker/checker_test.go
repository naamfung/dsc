package checker

import (
	"os"
	"testing"
	"time"
)

// TestCheckExample 验证示例插件的语法门禁 + 类型诊断，并记录耗时
// （flow-sensitive 类型检查可能较慢，观察其对插件加载的影响）。
func TestCheckExample(t *testing.T) {
	src, err := os.ReadFile("../../examples/hello/main.lua")
	if err != nil {
		t.Fatalf("read example plugin: %v", err)
	}
	start := time.Now()
	diags, err := Check(string(src), "hello")
	elapsed := time.Since(start)
	t.Logf("check elapsed: %v", elapsed)
	if err != nil {
		t.Fatalf("syntax gate failed: %v", err)
	}
	t.Logf("diagnostics (%d):", len(diags))
	for _, d := range diags {
		t.Logf("  %s", d)
	}
}
