package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
	"github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"
)

// TestHotReloadAgentReconnect 是 agent 热重载后"失联"缺陷的修复验收：
//
// 旧实现把"旧 agent 连接上挂载的聚合 serviceID"原样注入新 agent；但 go-plugin
// broker 的 (id,addr) 连接信息是 per-connection 的，新 agent 的新连接没有旧 id
// 的通告，运行时 RunStream 同步 Dial 会超时失联。
//
// 修复后 hotReloadAgent 会在「新 agent 自己的 broker」上重新挂载聚合服务，并以
// 新 id 注入。本测试驱动真实 agent-react-loop：首次 RunStream 成功 → HotReload →
// 二次 RunStream 成功，即证明热重载后新 agent 仍能连上宿主聚合 LLM/Tool。
func TestHotReloadAgentReconnect(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoWhole := filepath.Dir(wd) // .../dsc/core -> .../dsc

	tmpDir := t.TempDir()
	baseExe := filepath.Join(tmpDir, "agent-react-loop.exe")
	buildAgentBin(t, filepath.Join(repoWhole, "plugins", "agent-react-loop"), baseExe)
	// "新 agent"：复制基线产物为更高版本文件名（Windows 下运行中的 exe 被占用时
	// 不能覆盖，只能以新版本文件启动新进程，与热重载 watch 的约定一致）。
	newExe := filepath.Join(tmpDir, "agent-react-loop-v1.0.1.exe")
	if b, err := os.ReadFile(baseExe); err != nil {
		t.Fatalf("read base exe: %v", err)
	} else if err := os.WriteFile(newExe, b, 0755); err != nil {
		t.Fatalf("write new exe: %v", err)
	}
	if fi, err := os.Stat(newExe); err != nil {
		t.Fatalf("stat new exe: %v", err)
	} else {
		t.Logf("newExe size=%d", fi.Size())
	}

	m := NewManager(&ManagerConfig{
		Logger: hclog.NewNullLogger(),
		PluginLogger: hclog.New(&hclog.LoggerOptions{
			Name: "test-plugin", Level: hclog.Error,
		}),
	})
	const name = "agent-react-loop"

	// 手工加载"旧 agent"，在其 broker 上挂载聚合服务（mock LLM/Tool/UQ）
	oldImpl := loadTestAgent(t, m, name, baseExe, tmpDir)
	runTestAgentTurn(t, oldImpl, "ping") // 首次 RunStream：加载期链路正常

	// 热重载到"新 agent"（更高版本二进制）
	if err := m.HotReload(name, newExe); err != nil {
		t.Fatalf("HotReload(agent) 应成功(新agent已重挂聚合服务), got err: %v", err)
	}

	newImpl, ok := GetAgentForTest(m, name)
	if !ok {
		t.Fatalf("重载后取不到 agent %s", name)
	}
	if newImpl == oldImpl {
		t.Fatal("重载后 agent 实例应被替换为新进程实例")
	}
	runTestAgentTurn(t, newImpl, "ping")
	t.Log("修复验证: 热重载后新 agent 能连通聚合 LLM/Tool (RunStream 成功)")

	// 收尾：终止插件进程
	m.mu.Lock()
	if c := m.clients[name]; c != nil {
		c.Kill()
	}
	m.mu.Unlock()
}

// buildAgentBin 编译 agent-react-loop 插件二进制到 out。
func buildAgentBin(t *testing.T, dir, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = dir
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", dir, err, b)
	}
}

// runTestAgentTurn 驱动一次 RunStream，断言成功（产出 mock LLM 回复且无 error 帧）。
func runTestAgentTurn(t *testing.T, agent Agent, input string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := agent.RunStream(ctx, input, nil)
	if err != nil {
		t.Fatalf("RunStream(%q) err: %v", input, err)
	}
	var gotErr bool
	var gotContent string
	for it := range ch {
		if it.Status == "error" {
			gotErr = true
		}
		gotContent += it.Output
	}
	if gotErr {
		t.Fatalf("RunStream(%q) 报告 error 帧 -> 热重载后链路失联", input)
	}
	if !strings.Contains(gotContent, "stub-reply") {
		t.Fatalf("RunStream(%q) 未产出 mock 回复, got: %q", input, gotContent)
	}
}

// stubLLMProvider 实现 LLMProvider，返回固定文本不做任何工具调用。
type stubLLMProvider struct{}

func (stubLLMProvider) Name(context.Context) string    { return "mock-llm" }
func (stubLLMProvider) Version(context.Context) string { return "1.0.0" }
func (stubLLMProvider) HealthCheck(context.Context) error {
	return nil
}
func (stubLLMProvider) Chat(context.Context, []Message, []Tool, int) (*ChatResponse, error) {
	return &ChatResponse{Content: "stub-reply", FinishReason: "stop"}, nil
}
func (stubLLMProvider) ChatStream(context.Context, []Message, []Tool) (<-chan *ChatStreamResponse, error) {
	ch := make(chan *ChatStreamResponse, 1)
	ch <- &ChatStreamResponse{Content: "stub-reply", FinishReason: "stop"}
	close(ch)
	return ch, nil
}

// GetAgentForTest 返回 manager 中 name 对应的 agent 实现。
func GetAgentForTest(m *Manager, name string) (Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[name]
	return a, ok
}

// loadTestAgent 手工加载 agent 进程，并在其 broker 上挂载聚合 LLM/Tool/UQ 服务
// （与 loadAgentAndGetBroker + provider 就绪后挂载流程等价），返回 agent 实现。
func loadTestAgent(t *testing.T, m *Manager, name, exe, execDir string) Agent {
	t.Helper()
	cmd := exec.Command(exe)
	cmd.Dir = execDir
	cmd.Env = append(os.Environ(),
		"DSC_WORKSPACE_ROOT="+execDir,
		"DSC_SESSION_DIR="+filepath.Join(execDir, "sessions"),
		"DSC_SINGLE_TURN=1",
	)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          map[string]plugin.Plugin{"agent": &AgentGRPCPlugin{}},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		Cmd:              cmd,
		Logger:           m.coreLogger,
	})
	rpcClient, err := client.Client()
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	grpcClient, ok := rpcClient.(*plugin.GRPCClient)
	if !ok {
		t.Fatalf("client %T is not *GRPCClient", rpcClient)
	}
	raw, err := rpcClient.Dispense("agent")
	if err != nil {
		t.Fatalf("dispense: %v", err)
	}
	impl, ok := raw.(Agent)
	if !ok {
		t.Fatalf("impl %T not Agent", raw)
	}

	m.mu.Lock()
	m.config.ExecDir = execDir
	m.config.Handshake = Handshake
	m.broker = grpcClient.Broker()
	m.mainAgentName = name
	m.clients[name] = client
	m.agents[name] = impl
	m.typeMap[name] = "agent"
	m.llms["mock-llm"] = stubLLMProvider{}
	m.llmOrder = []string{"mock-llm"}
	m.agentLLMName = "mock-llm"
	broker := m.broker
	llmID, err := m.serveAggregateLLMOnBroker(broker, "mock-llm")
	if err != nil {
		t.Fatalf("serveAggregateLLM: %v", err)
	}
	toolID, err := m.serveAggregateToolOnBroker(broker)
	if err != nil {
		t.Fatalf("serveAggregateTool: %v", err)
	}
	uqID, err := m.serveUserQuestionsOnBroker(broker)
	if err != nil {
		t.Fatalf("serveUserQuestions: %v", err)
	}
	m.agentLLMServiceID = llmID
	m.agentToolServiceID = toolID
	m.userQuestionsServiceID = uqID
	m.transitionLocked(name, StateActive, "")
	m.mu.Unlock()

	agentClient := proto.NewAgentServiceClient(grpcClient.Conn)
	if _, err := agentClient.RegisterServices(context.Background(), &proto.RegisterServicesRequest{
		LlmServiceId: llmID, ToolServiceId: toolID,
	}); err != nil {
		t.Fatalf("RegisterServices: %v", err)
	}
	if err := impl.SwitchSession(context.Background(), "default"); err != nil {
		t.Fatalf("SwitchSession: %v", err)
	}
	return impl
}
