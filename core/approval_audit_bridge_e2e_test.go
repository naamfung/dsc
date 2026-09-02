package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
)

// 本文件覆盖 「host↔agent 双进程」的「审批审计落会话」桥（对齐 DSH：审批审计写会话日志）：
// 真实 agent-react-loop 进程跑完一轮（mock LLM，session "default" 已建并落盘），宿主进程经
// agent 的 PluginHookService.OnEvent（即宿主 broadcastEventToPlugins 会调的同一条 RPC）投递
// approval/asked + decided，断言 agent 会话日志确实写入这两个审计事件（跨进程端到端）。

func TestApprovalAuditBridgeToAgentSession(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoWhole := filepath.Dir(wd) // .../dsc/core -> .../dsc
	tmpDir := t.TempDir()
	exe := filepath.Join(tmpDir, "agent-react-loop.exe")
	buildAgentBin(t, filepath.Join(repoWhole, "plugins", "agent-react-loop"), exe)

	m := NewManager(&ManagerConfig{ExecDir: tmpDir})
	const name = "agent-react-loop"
	_, gc := loadTestAgent(t, m, name, exe, tmpDir)
	defer func() {
		m.mu.Lock()
		if c := m.clients[name]; c != nil {
			c.Kill()
		}
		m.mu.Unlock()
	}()

	// 跑一轮：建立 session（loadTestAgent 已 SwitchSession "default"）并落盘。
	agent, _ := GetAgentForTest(m, name)
	runTestAgentTurn(t, agent, "ping")

	// host→agent：同一条 PluginHookService.OnEvent RPC，投递审批审计事件。
	hook := proto.NewPluginHookServiceClient(gc.Conn)
	if _, err := hook.OnEvent(context.Background(), &proto.OnEventRequest{
		Name:     string(EventApprovalAsked),
		DataJson: `{"session":"default","tool":"shell","mode":"workspace-write","reason":"need it"}`,
	}); err != nil {
		t.Fatalf("OnEvent(asked): %v", err)
	}
	if _, err := hook.OnEvent(context.Background(), &proto.OnEventRequest{
		Name:     string(EventApprovalDecided),
		DataJson: `{"session":"default","tool":"shell","mode":"workspace-write","outcome":"allowed-once"}`,
	}); err != nil {
		t.Fatalf("OnEvent(decided): %v", err)
	}

	// 读 agent 的会话 JSONL，断言两个审计事件确实落盘。
	dir := filepath.Join(tmpDir, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("sessions dir: %v", err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			sb.Write(b)
		}
	}
	all := sb.String()
	if !strings.Contains(all, "approval/asked") || !strings.Contains(all, "approval/decided") {
		t.Fatalf("agent 会话日志未写入审批审计事件，got:\n%s", all)
	}
	if !strings.Contains(all, "allowed-once") {
		t.Fatalf("会话日志缺 decided outcome，got:\n%s", all)
	}
}
