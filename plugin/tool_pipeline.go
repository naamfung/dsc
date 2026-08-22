package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"dsc/proto"
)

// 工具执行流水线（对齐 DSH tools/* 事件管线）：
//
//	pre-execute → execute → post-execute
//
// pre/post 均为 waterfall 事件：监听器通过 next 委托给链上后续监听器，
// 不调 next 即 veto。pre 阶段可拦截（阻止执行），post 阶段可观测/改写结果。
// 流水线挂在宿主聚合 Tool 服务上，agent 的所有工具调用都经过它，
// policy 插件等策略逻辑以监听器形式参与（替代旁路）。

const (
	// EventToolPreExecute 工具执行前拦截（waterfall）：veto 返回错误即阻止执行。
	EventToolPreExecute EventName = "tools/pre-execute"
	// EventToolPostExecute 工具执行后处理（waterfall）：可观测或改写结果。
	EventToolPostExecute EventName = "tools/post-execute"
)

// ToolInvocation 一次工具调用的流水线上下文（共享指针，监听器可直接改写）。
type ToolInvocation struct {
	ToolName      string
	ArgumentsJSON string
	CallID        string
	Result        string // post-execute 阶段：执行结果
	Err           error  // 执行错误或 pre 阶段 veto 原因
}

// filePathFromArgs 从工具参数 JSON 中提取 file_path 字段（观测策略用）。
func filePathFromArgs(argsJSON string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return ""
	}
	if p, ok := m["file_path"].(string); ok {
		return p
	}
	return ""
}

// ToolTimeoutError 工具调用超时（对齐 DSH TOOL_TIMEOUT 结构化结果）。
type ToolTimeoutError struct {
	Tool string
	Ms   int
}

func (e *ToolTimeoutError) Error() string {
	return fmt.Sprintf("Error: tool call timed out after %dms (TOOL_TIMEOUT)", e.Ms)
}

// ExecuteTool 以流水线方式执行工具：pre-execute(waterfall) → execute → post-execute(waterfall)。
// 任何阶段返回错误即中止；post 阶段的监听器可改写 inv.Result。
// 声明 TimeoutProvider 的工具在 execute 阶段获得协作式单次调用截止时间（timeout-policy）。
func (m *Manager) ExecuteTool(ctx context.Context, toolName string, argsJSON json.RawMessage) (string, error) {
	inv := &ToolInvocation{ToolName: toolName, ArgumentsJSON: string(argsJSON)}
	// pre-execute + execute：next 为实际执行；pre 监听器不调 next 即 veto
	runErr := m.events.Waterfall(EventToolPreExecute, EventContext{Data: inv}, func(EventContext) error {
		// 互通机制 3：插件 BeforeTool 钩子（可 veto/改写参数；按加载顺序调用）
		if veto := m.runPluginBeforeTool(ctx, inv); veto != nil {
			inv.Err = veto
			return veto
		}
		tool, ok := m.toolRegistry.Get(toolName)
		if !ok {
			inv.Err = fmt.Errorf("tool not found: %s", toolName)
			return inv.Err
		}
		// timeout-policy：声明 timeoutMs 的工具设置协作式截止时间（对齐 DSH）
		execCtx, timeoutMs := ctx, 0
		if tp, ok := tool.(TimeoutProvider); ok {
			if ms := tp.TimeoutMs(); ms > 0 {
				timeoutMs = ms
				var cancel context.CancelFunc
				execCtx, cancel = context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
				defer cancel()
			}
		}
		result, err := tool.Execute(execCtx, json.RawMessage(inv.ArgumentsJSON))
		if errors.Is(err, context.DeadlineExceeded) {
			err = &ToolTimeoutError{Tool: toolName, Ms: timeoutMs}
		}
		inv.Result, inv.Err = result, err
		return err
	})
	if inv.Err == nil && runErr != nil {
		inv.Err = runErr // pre 阶段 veto（execute 未运行）
	}
	// post-execute：观测/改写，next 原样透传 inv.Err
	if err := m.events.Waterfall(EventToolPostExecute, EventContext{Data: inv}, func(EventContext) error {
		// 互通机制 3：插件 AfterTool 钩子（可改写结果/错误）
		m.runPluginAfterTool(ctx, inv)
		return inv.Err
	}); err != nil {
		return "", err
	}
	return inv.Result, inv.Err
}

// bridgePolicyToPipeline 把已加载的 policy 插件观测服务桥接为工具流水线监听器：
// post-execute 记录文件观测，pre-execute 执行读前检查（写操作要求已有观测）。
// 返回监听器的移除函数（卸载 policy 时调用）。
func (m *Manager) bridgePolicyToPipeline(name string, pc proto.FsObservationPolicyServiceClient) []func() {
	var off []func()
	// post-execute：工具执行后记录文件观测（无 file_path 的工具跳过）
	off = append(off, m.events.OnWaterfall(EventToolPostExecute, func(ctx EventContext, next func(EventContext) error) error {
		inv, _ := ctx.Data.(*ToolInvocation)
		path := ""
		if inv != nil {
			path = filePathFromArgs(inv.ArgumentsJSON)
		}
		if err := next(ctx); err != nil {
			return err
		}
		if path == "" || inv == nil {
			return nil
		}
		content := inv.Result
		if inv.Err != nil {
			content = "error: " + inv.Err.Error()
		}
		_, err := pc.UpdateObservation(context.Background(), &proto.UpdateObservationRequest{
			FilePath: path,
			Observation: &proto.FsObservation{
				State:       "observed",
				Version:     "1",
				LastContent: content,
			},
		})
		if err != nil {
			return fmt.Errorf("policy %s update observation: %w", name, err)
		}
		return nil
	}))
	// pre-execute：读前检查——写类工具要求目标文件已有观测记录
	off = append(off, m.events.OnWaterfall(EventToolPreExecute, func(ctx EventContext, next func(EventContext) error) error {
		inv, _ := ctx.Data.(*ToolInvocation)
		if inv == nil {
			return next(ctx)
		}
		path := filePathFromArgs(inv.ArgumentsJSON)
		// 仅对写类工具（当前为读写合一编辑器）做读前检查
		if path == "" || !isWriteTool(inv.ToolName) {
			return next(ctx)
		}
		resp, err := pc.GetObservation(context.Background(), &proto.GetObservationRequest{FilePath: path})
		if err != nil {
			return next(ctx) // 策略服务不可用不阻塞执行
		}
		if resp.GetFound() {
			return next(ctx)
		}
		return fmt.Errorf("policy %s: file %q has not been read yet; read it before editing", name, path)
	}))
	return off
}

// isWriteTool 判断工具是否属于写类（需要读前检查）。
func isWriteTool(toolName string) bool {
	switch toolName {
	case "str_replace_editor":
		return true
	default:
		return false
	}
}
