package checker

import (
	"os"
	"testing"
	"time"
)

// TestCheckExample 验证 example 脚本的语法门禁 + 类型诊断，并记录耗时
// （flow-sensitive 类型检查可能较慢，观察其对加载的影响）。
func TestCheckExample(t *testing.T) {
	src, err := os.ReadFile("../../scripts/example/main.lua")
	if err != nil {
		t.Fatalf("read example script: %v", err)
	}
	start := time.Now()
	diags, err := Check(string(src), "example")
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
