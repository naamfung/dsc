package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// slowCtxTool 感知 ctx 的慢工具：声明 timeoutMs；超时经 ctx.Done 返回。
type slowCtxTool struct {
	ms int
}

func (t *slowCtxTool) Name() string                      { return "slow_ctx" }
func (t *slowCtxTool) Description() string               { return "slow cooperative tool" }
func (t *slowCtxTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *slowCtxTool) TimeoutMs() int                    { return t.ms }
func (t *slowCtxTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(500 * time.Millisecond):
		return "done", nil
	}
}

// ignoreCtxTool 忽略 ctx 的慢工具：声明 timeoutMs 但不感知取消（协作式边界）。
type ignoreCtxTool struct{}

func (t *ignoreCtxTool) Name() string                      { return "ignore_ctx" }
func (t *ignoreCtxTool) Description() string               { return "ignores ctx" }
func (t *ignoreCtxTool) ParametersSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (t *ignoreCtxTool) TimeoutMs() int                    { return 50 }
func (t *ignoreCtxTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	time.Sleep(100 * time.Millisecond) // 不转发 ctx：超时不中断
	return "done", nil
}

func TestToolTimeoutPolicy(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	_ = m.toolRegistry.Register(&slowCtxTool{ms: 50})
	_ = m.toolRegistry.Register(&ignoreCtxTool{})

	// 声明 timeoutMs 的感知工具：超时 → TOOL_TIMEOUT 文本
	start := time.Now()
	_, err := m.ExecuteTool(context.Background(), "slow_ctx", json.RawMessage(`{}`))
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out after 50ms (TOOL_TIMEOUT)") {
		t.Fatalf("timeout error = %v", err)
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("timeout should cut the call short, took %v", elapsed)
	}
	var terr *ToolTimeoutError
	if !asToolTimeout(err, &terr) {
		t.Fatalf("error should be ToolTimeoutError, got %v", err)
	}

	// 忽略 ctx 的工具（协作式边界）：超时不硬终止，返回正常结果
	out, err := m.ExecuteTool(context.Background(), "ignore_ctx", json.RawMessage(`{}`))
	if err != nil || out != "done" {
		t.Fatalf("ignore-ctx tool = %q, %v", out, err)
	}
}

// asToolTimeout 断言错误为 *ToolTimeoutError。
func asToolTimeout(err error, target **ToolTimeoutError) bool {
	te, ok := err.(*ToolTimeoutError)
	if ok {
		*target = te
	}
	return ok
}
