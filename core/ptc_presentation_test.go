package core

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"

	"dsc/proto"
)

// TestPTCPresentationGate 校验 run_code 的 presentation 门控（对齐 DSH：
// native 下 run_code 不可见不可执行；ptc 下折叠直接调用为唯一 run_code）。
func TestPTCPresentationGate(t *testing.T) {
	// ---- native（默认）----
	m := NewManager(&ManagerConfig{Logger: hclog.NewNullLogger(), PluginLogger: hclog.NewNullLogger()})
	native := m.AgentDirectTools()
	if hasTool(native, runCodeToolName) {
		t.Fatalf("native 下 AgentDirectTools 不应含 run_code: %v", toolNames(native))
	}
	if len(native) == 0 {
		t.Fatal("native 下应暴露业务工具（内建 subagent/workflow/job 等）")
	}

	// native 下执行 run_code 应被拒（presentation 层禁止）
	if _, err := m.ExecuteTool(context.Background(), runCodeToolName, []byte(`{"source":"return 1"}`)); err == nil {
		t.Fatal("native 下执行 run_code 应被拒绝")
	} else if !strings.Contains(err.Error(), "only available in PTC") {
		t.Fatalf("native 拒绝信息不符, got: %v", err)
	}

	// ---- PTC ----
	mp := NewManager(&ManagerConfig{Logger: hclog.NewNullLogger(), PluginLogger: hclog.NewNullLogger(), PTC: true})
	ptc := mp.AgentDirectTools()
	if len(ptc) != 1 || ptc[0].Name != runCodeToolName {
		t.Fatalf("ptc 下仅暴露 run_code, got: %v", toolNames(ptc))
	}
	// run_code 描述应承载程序内可调的业务工具清单（来自 AllToolsProto 业务目录）
	biz := mp.AllToolsProto()
	if hasTool(biz, runCodeToolName) {
		t.Fatalf("AllToolsProto（业务目录/SDK 来源）不应含 run_code: %v", toolNames(biz))
	}
	if len(biz) > 0 && !strings.Contains(ptc[0].Description, biz[0].Name) {
		t.Fatalf("ptc run_code 描述应含业务工具名 %s, got desc: %s", biz[0].Name, ptc[0].Description)
	}

	// ptc 下执行 run_code 应放行（不触发 presentation 拒绝）
	if _, err := mp.ExecuteTool(context.Background(), runCodeToolName, []byte(`{"source":"return 1"}`)); err != nil {
		if strings.Contains(err.Error(), "only available in PTC") {
			t.Fatalf("ptc 下 run_code 不应被 presentation 拒绝: %v", err)
		}
	}

	// ---- SwitchMode 运行时同步 ----
	if err := m.SwitchMode("ptc"); err == nil && !m.isPTC() {
		t.Fatal("SwitchMode(ptc) 后应进入 PTC")
	}
}

func hasTool(tools []*proto.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func toolNames(tools []*proto.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		out = append(out, t.Name)
	}
	return out
}
