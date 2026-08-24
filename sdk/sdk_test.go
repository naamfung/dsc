package dsc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
)

func testSDK() *SDK {
	return New(Config{Name: "test", Version: "1.0.0", Type: TypeTool})
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		build   func() *SDK
		wantErr string
	}{
		{"tool 无工具报错", Config{Name: "x", Type: TypeTool}, func() *SDK { return New(Config{Name: "x", Type: TypeTool}) }, "至少注册一个工具"},
		{"tool 无 Handler 报错", Config{Name: "x", Type: TypeTool}, func() *SDK {
			s := New(Config{Name: "x", Type: TypeTool})
			s.Tool(Tool{Name: "t"})
			return s
		}, "未设置 Handler"},
		{"tool 空名字报错", Config{Name: "x", Type: TypeTool}, func() *SDK {
			s := New(Config{Name: "x", Type: TypeTool})
			s.Tool(Tool{Name: "", Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "", nil }})
			return s
		}, "Name 不能为空"},
		{"llm 无实现报错", Config{Name: "x", Type: TypeLLM}, func() *SDK { return New(Config{Name: "x", Type: TypeLLM}) }, "必须注册 LLMProvider"},
		{"agent 无实现报错", Config{Name: "x", Type: TypeAgent}, func() *SDK { return New(Config{Name: "x", Type: TypeAgent}) }, "必须注册 Agent"},
		{"空名字报错", Config{Name: "", Type: TypeTool}, func() *SDK { return New(Config{Name: "", Type: TypeTool}) }, "Name 必填"},
		{"未知类型报错", Config{Name: "x", Type: "weird"}, func() *SDK { return New(Config{Name: "x", Type: "weird"}) }, "不支持的插件类型"},
		{"tool 正常", Config{Name: "x", Type: TypeTool}, func() *SDK {
			s := New(Config{Name: "x", Type: TypeTool})
			s.Tool(Tool{Name: "t", Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "ok", nil }})
			return s
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.build().validate()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("validate = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("validate = %v, want contains %q", err, c.wantErr)
			}
		})
	}
}

func TestToolServiceServer(t *testing.T) {
	s := testSDK()
	s.Tool(Tool{
		Name: "echo", Description: "echo text",
		Schema:  json.RawMessage(`{"type":"object"}`),
		Context: "echo 工具约定说明",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var v struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &v); err != nil {
				return "", err
			}
			if v.Text == "boom" {
				return "", errors.New("boom")
			}
			return "echo: " + v.Text, nil
		},
	})
	srv := &toolServiceServer{sdk: s}

	// ExecuteTool
	resp, err := srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{ToolName: "echo", ArgumentsJson: `{"text":"hi"}`})
	if err != nil || resp.Error != "" || resp.Content != "echo: hi" {
		t.Fatalf("ExecuteTool = (%+v, %v), want echo: hi", resp, err)
	}
	// 错误路径：error 进 Error 字段而非 gRPC error
	resp, _ = srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{ToolName: "echo", ArgumentsJson: `{"text":"boom"}`})
	if resp.Error != "boom" {
		t.Fatalf("ExecuteTool error = %q, want boom", resp.Error)
	}
	// 未知工具
	resp, _ = srv.ExecuteTool(context.Background(), &proto.ExecuteToolRequest{ToolName: "nope", ArgumentsJson: `{}`})
	if resp.Error == "" {
		t.Fatal("未知工具应返回错误")
	}

	// ListTools
	list, err := srv.ListTools(context.Background(), &proto.ListToolsRequest{})
	if err != nil || len(list.Tools) != 1 || list.Tools[0].Name != "echo" || list.Tools[0].Description != "echo text" {
		t.Fatalf("ListTools = %+v, %v", list, err)
	}

	// ListContext
	lc, err := srv.ListContext(context.Background(), &proto.ListContextRequest{})
	if err != nil || !strings.Contains(lc.Content, "echo 工具约定说明") {
		t.Fatalf("ListContext = %+v, %v", lc, err)
	}
}

// TestToolServiceServerContextFn 动态上下文优先于静态 Context（每次求值）。
func TestToolServiceServerContextFn(t *testing.T) {
	calls := 0
	s := testSDK()
	s.Tool(Tool{
		Name: "t", Context: "static", ContextFn: func() string {
			calls++
			return "dynamic"
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "", nil },
	})
	srv := &toolServiceServer{sdk: s}
	lc, err := srv.ListContext(context.Background(), &proto.ListContextRequest{})
	if err != nil || !strings.Contains(lc.Content, "dynamic") || strings.Contains(lc.Content, "static") || calls != 1 {
		t.Fatalf("ContextFn ListContext = %+v, calls=%d, err %v", lc, calls, err)
	}
	// 第二次调用重新求值
	_, _ = srv.ListContext(context.Background(), &proto.ListContextRequest{})
	if calls != 2 {
		t.Fatalf("ContextFn 应每调用求值, calls=%d", calls)
	}
}

func TestHookServiceServer(t *testing.T) {
	t.Run("BeforeTool veto", func(t *testing.T) {
		srv := &hookServiceServer{hook: &Hook{
			BeforeTool: func(ctx context.Context, name, args string) (string, error) {
				return "", errors.New("blocked: " + name)
			},
		}}
		resp, err := srv.BeforeTool(context.Background(), &proto.BeforeToolRequest{ToolName: "t", ArgumentsJson: `{}`})
		if err != nil || !resp.Veto || !strings.Contains(resp.Error, "blocked") {
			t.Fatalf("veto = %+v, %v", resp, err)
		}
	})
	t.Run("BeforeTool rewrite", func(t *testing.T) {
		srv := &hookServiceServer{hook: &Hook{
			BeforeTool: func(ctx context.Context, name, args string) (string, error) {
				return `{"x":2}`, nil
			},
		}}
		resp, err := srv.BeforeTool(context.Background(), &proto.BeforeToolRequest{ToolName: "t", ArgumentsJson: `{"x":1}`})
		if err != nil || resp.Veto || resp.ArgumentsJson != `{"x":2}` {
			t.Fatalf("rewrite = %+v, %v", resp, err)
		}
	})
	t.Run("BeforeTool keep as-is", func(t *testing.T) {
		// 未设置 BeforeTool：空实现保持原样
		srv := &hookServiceServer{}
		resp, err := srv.BeforeTool(context.Background(), &proto.BeforeToolRequest{ToolName: "t", ArgumentsJson: `{"x":1}`})
		if err != nil || resp.Veto || resp.ArgumentsJson != `{"x":1}` {
			t.Fatalf("keep = %+v, %v", resp, err)
		}
	})
	t.Run("AfterTool rewrite", func(t *testing.T) {
		srv := &hookServiceServer{hook: &Hook{
			AfterTool: func(ctx context.Context, name, result, toolErr string) (string, string) {
				return result + "!", toolErr
			},
		}}
		resp, err := srv.AfterTool(context.Background(), &proto.AfterToolRequest{ToolName: "t", Result: "r", Error: ""})
		if err != nil || resp.Result != "r!" {
			t.Fatalf("AfterTool = %+v, %v", resp, err)
		}
	})
	t.Run("OnEvent delivered", func(t *testing.T) {
		got := ""
		srv := &hookServiceServer{hook: &Hook{
			OnEvent: func(ctx context.Context, eventType, dataJSON string) { got = eventType + ":" + dataJSON },
		}}
		_, err := srv.OnEvent(context.Background(), &proto.OnEventRequest{Name: "turn/start", DataJson: `{}`})
		if err != nil || got != "turn/start:{}" {
			t.Fatalf("OnEvent got %q, err %v", got, err)
		}
	})
}

func TestMetadataServer(t *testing.T) {
	srv := &metadataServer{cfg: Config{Name: "n", Version: "2.0.0", Type: TypeTool, APIVersion: "1.0"}}
	info, err := srv.GetInfo(context.Background(), &metadata.Empty{})
	if err != nil || info.Type != "tool" || info.Name != "n" || info.Version != "2.0.0" || info.ApiVersion != "1.0" {
		t.Fatalf("GetInfo = %+v, %v", info, err)
	}
	// New 会默认 APIVersion=1.0
	srv2 := &metadataServer{cfg: New(Config{Name: "n", Type: TypeTool}).cfg}
	info2, _ := srv2.GetInfo(context.Background(), &metadata.Empty{})
	if info2.ApiVersion != "1.0" {
		t.Fatalf("New 默认 APIVersion = %q, want 1.0", info2.ApiVersion)
	}
}

func TestReadEnv(t *testing.T) {
	t.Setenv("DSC_MODE", "creation")
	t.Setenv("DSC_WORKSPACE_ROOT", `D:\ws`)
	t.Setenv("DSC_CONTEXT_WINDOW", "131072")
	t.Setenv("DSC_SINGLE_TURN", "1")
	t.Setenv("DSC_TODO_ALLOW_PARALLEL", "0")
	env := ReadEnv()
	if env.Mode != "creation" || env.WorkspaceRoot != `D:\ws` || env.ContextWindow != 131072 || !env.SingleTurn || env.AllowParallelTodo {
		t.Fatalf("ReadEnv = %+v", env)
	}
}

// TestMetaWrapperCfgPrecedence 验证 LLM/Agent 元数据以 sdk.Config.Name/Version 为准
// （wrapper 覆盖实现内部 Name()/Version()，避免两处维护不一致）。
func TestMetaWrapperCfgPrecedence(t *testing.T) {
	ctx := context.Background()

	// TypeLLM：cfg 非空 → 以 cfg 为准
	llmImpl := &stubLLM{name: "impl-name", version: "0.0.1"}
	w := &llmMetaWrapper{LLMProvider: llmImpl, name: "cfg-name", version: "9.9.9"}
	if got := w.Name(ctx); got != "cfg-name" {
		t.Fatalf("LLM wrapper Name = %q, want cfg-name", got)
	}
	if got := w.Version(ctx); got != "9.9.9" {
		t.Fatalf("LLM wrapper Version = %q, want 9.9.9", got)
	}
	// cfg 为空 → 回落实现
	w2 := &llmMetaWrapper{LLMProvider: llmImpl}
	if got := w2.Name(ctx); got != "impl-name" {
		t.Fatalf("LLM wrapper fallback Name = %q, want impl-name", got)
	}

	// TypeAgent：同样规则
	agentImpl := &stubAgent{name: "a-impl", version: "0.1.0"}
	aw := &agentMetaWrapper{Agent: agentImpl, name: "a-cfg", version: "1.2.3"}
	if got := aw.Name(ctx); got != "a-cfg" {
		t.Fatalf("Agent wrapper Name = %q, want a-cfg", got)
	}
	aw2 := &agentMetaWrapper{Agent: agentImpl}
	if got := aw2.Version(ctx); got != "0.1.0" {
		t.Fatalf("Agent wrapper fallback Version = %q, want 0.1.0", got)
	}
}

type stubLLM struct {
	plugin.LLMProvider
	name, version string
}

func (s *stubLLM) Name(context.Context) string    { return s.name }
func (s *stubLLM) Version(context.Context) string { return s.version }

type stubAgent struct {
	plugin.Agent
	name, version string
}

func (s *stubAgent) Name(context.Context) string    { return s.name }
func (s *stubAgent) Version(context.Context) string { return s.version }

func TestSetInterconnectCallsHandler(t *testing.T) {
	called := false
	s := New(Config{Name: "x", Type: TypeTool})
	s.Tool(Tool{Name: "t", Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "", nil }})
	s.SetInterconnect(func(ctx context.Context, ic *Interconnect) error {
		called = true
		if ic.LLM() != nil || ic.Tool() != nil {
			t.Fatalf("未互联时客户端应为 nil")
		}
		return nil
	})
	srv := &toolServiceServer{sdk: s} // broker nil：无聚合服务可 Dial
	resp, err := srv.SetInterconnect(context.Background(), &proto.InterconnectRequest{})
	if err != nil || !called {
		t.Fatalf("SetInterconnect = (%v, %v), called=%v", resp, err, called)
	}

	// handler 错误透传（宿主仅 Warn，不影响加载）
	s.SetInterconnect(func(ctx context.Context, ic *Interconnect) error {
		return errors.New("interconnect handler failed")
	})
	_, err = srv.SetInterconnect(context.Background(), &proto.InterconnectRequest{})
	if err == nil || err.Error() != "interconnect handler failed" {
		t.Fatalf("handler 错误应透传: %v", err)
	}
}

func TestInterconnectNotifyNoopWhenDisconnected(t *testing.T) {
	ic := &Interconnect{}
	if err := ic.Notify("x", `{}`); err != nil {
		t.Fatalf("未互联时 Notify 应静默忽略: %v", err)
	}
	if err := ic.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}
	// nil 接收者安全
	var nilIC *Interconnect
	if err := nilIC.Notify("x", "{}"); err != nil {
		t.Fatalf("nil Interconnect Notify = %v", err)
	}
}
