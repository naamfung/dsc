package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"dsc/jobs"
	"dsc/proto"
)

// 工具执行流水线（对齐 DSH tools/* 事件管线）：
//
//      pre-execute → execute → post-execute
//
// pre/post 均为 waterfall 事件：监听器通过 next 委托给链上后续监听器，
// 不调 next 即 veto。pre 阶段可拦截（阻止执行），post 阶段可观测/改写结果。
// 流水线挂在宿主聚合 Tool 服务上，agent 的所有工具调用都经过它，
// policy 插件等策略逻辑以监听器形式参与（替代旁路）。

const (
	// EventToolPreExecute 工具执行前拦截（waterfall）：veto 返回错误即阻止执行。
	EventToolPreExecute EventName = "tools/pre-execute"
	// EventToolExecute 工具执行（waterfall）：对齐 DSH tools/execute——监听器可改写
	// 执行方式/参数/超时策略（timeout-policy）；不调 next 即 veto。DSC 现有实现把
	// 实际执行纳入 pre-execute 的 next 闭包，此处约束为显式事件以便插件订阅。
	EventToolExecute EventName = "tools/execute"
	// EventToolPostExecute 工具执行后处理（waterfall）：可观测或改写结果。
	EventToolPostExecute EventName = "tools/post-execute"
	// EventToolResult 工具结果（emit）：对齐 DSH tools/result——工具执行完成后广播，
	// 供插件订阅工具调用结果（非拦截，仅通知）。
	EventToolResult EventName = "tools/result"
	// EventToolsChange 工具集变更（emit）：对齐 DSH tools/change——工具加载/卸载时广播，
	// 供依赖工具清单的插件（如 subagent）感知工具集变化。
	EventToolsChange EventName = "tools/change"
)

// ToolInvocation 一次工具调用的流水线上下文（共享指针，监听器可直接改写）。
type ToolInvocation struct {
	ToolName      string
	ArgumentsJSON string
	CallID        string
	Result        string // post-execute 阶段：执行结果
	Err           error  // 执行错误或 pre 阶段 veto 原因
	ViewJSON      string // 工具声明的结构化视图 spec（可选，见 ViewExecutor）
	// SessionID 调用方会话标识（来自 ExecuteToolWithView 的 ctx，agent 每次调用都会带）；
	// 供 per-session 审批策略（approvalPolicyFor）与审计事件归属使用。
	SessionID string
	// ApprovalPolicy 调用方会话随调用转发的审批策略（"ask"/"never"；空 = 未提供）。
	// 审批门优先以此为本会话生效策略，实现宿主重启后 per-session 恢复。
	ApprovalPolicy string

	// Escalated / EscalatedMode 沙箱升级审批（对齐 DSH approveEscalation）字段：
	// 审批门 allowed 后置 Escalated=true，并把本次调用目标更宽档暂存，随后
	// sandboxPolicy 以 EscalatedMode 复审放行（仅作用于这一个调用）。
	Escalated     bool
	EscalatedMode SandboxPolicy
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
// 视图信息（插件 ViewJson / 宿主 ViewExecutor）不在此返回，见 ExecuteToolWithView。
func (m *Manager) ExecuteTool(ctx context.Context, toolName string, argsJSON json.RawMessage) (string, error) {
	result, _, err := m.ExecuteToolWithView(ctx, toolName, argsJSON)
	return result, err
}

// ExecuteToolWithView 与 ExecuteTool 语义相同，额外返回工具声明的结构化视图 spec
// （ViewJson）：插件工具透传 Tool.ViewFn 产物（经 RemoteTool），宿主工具按需实现
// ViewExecutor。聚合 Tool 服务（ToolGRPCServer）据此把视图一并回给调用方。
//
// run_in_background 支持（对齐 DSH bash run_in_background）：若工具参数中
// run_in_background=true，宿主在 job 注册表中登记一个后台任务，异步执行完整
// 流水线（pre-execute → execute → post-execute），立即返回 job_id。
// 模型可用 job_output/job_list/job_kill 管理后台任务。
func (m *Manager) ExecuteToolWithView(ctx context.Context, toolName string, argsJSON json.RawMessage) (string, string, error) {
	// 检测 run_in_background 参数（对齐 DSH：模型在参数中声明 run_in_background: true）
	if isBackgroundRequest(argsJSON) && m.jobs != nil {
		caller := CallerFromContext(ctx)
		return m.startBackgroundTool(ctx, toolName, argsJSON, caller)
	}

	inv := &ToolInvocation{ToolName: toolName, ArgumentsJSON: string(argsJSON), SessionID: CallerFromContext(ctx), ApprovalPolicy: ApprovalPolicyFromContext(ctx)}

	// pre-execute（waterfall）：守卫。不调 next 即 veto（阻止执行，execute 不运行）。
	// 语义对齐 DSH tools/pre-execute guards。
	runErr := m.events.Waterfall(EventToolPreExecute, EventContext{Data: inv, Context: ctx}, func(EventContext) error {
		// 互通机制 3：插件 BeforeTool 钩子（可 veto/改写参数；按加载顺序调用）
		if veto := m.runPluginBeforeTool(ctx, inv); veto != nil {
			inv.Err = veto
			return veto
		}
		return inv.Err
	})
	if inv.Err == nil && runErr != nil {
		inv.Err = runErr // pre 阶段 veto（execute 未运行）
	}

	// execute（waterfall）：真正执行 + 超时策略。对齐 DSH tools/execute dispatch body。
	if inv.Err == nil {
		execErr := m.events.Waterfall(EventToolExecute, EventContext{Data: inv}, func(EventContext) error {
			return m.executeToolBody(ctx, inv, toolName)
		})
		if inv.Err == nil && execErr != nil {
			inv.Err = execErr
		}
	}

	// post-execute（waterfall）：观测/改写。对齐 DSH tools/post-execute finalize。
	if err := m.events.Waterfall(EventToolPostExecute, EventContext{Data: inv}, func(EventContext) error {
		// 互通机制 3：插件 AfterTool 钩子（可改写结果/错误）
		m.runPluginAfterTool(ctx, inv)
		return inv.Err
	}); err != nil {
		m.emitToolResult(inv)
		return "", "", err
	}

	// result（emit）：结果广播（非拦截），对齐 DSH tools/result。
	m.emitToolResult(inv)
	return inv.Result, inv.ViewJSON, inv.Err
}

// executeToolBody 实际执行工具（可被 tools/execute 的 waterfall 监听器改写/包围）。
// 返回执行错误；调用方负责据此置 inv.Err。超时策略在此协作式应用（对齐 DSH timeout-policy）。
//
// 通用 panic recover：覆盖所有宿主侧工具（含 builtin / read_spill / run_code / agent
// 内部工具等）与 RemoteTool 调用——任何工具 panic 都被转为错误返回，避免宿主进程崩溃
// 导致 LLM 连接中断。这是「工具意外不中断会话」的最后一道防线——插件 SDK 层（sdk/tool.go）
// 已先行 recover 一次，本层兜底覆盖未用 SDK 的工具（如 Lua 脚本工具、host 内置工具）。
func (m *Manager) executeToolBody(ctx context.Context, inv *ToolInvocation, toolName string) (errRet error) {
	// run_code 是 PTC presentation transport：native 模式不对模型暴露，也不可执行
	// （对齐 DSH“native agent must not find run_code”；ptc 折叠时才允许经其组合）。
	if toolName == runCodeToolName && !m.isPTC() {
		return fmt.Errorf("tool %s is only available in PTC (programmatic tool composition) mode", runCodeToolName)
	}
	tool, ok := m.toolRegistry.Get(toolName)
	if !ok {
		return fmt.Errorf("tool not found: %s", toolName)
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
	// 通用 panic recover：任何工具执行 panic 都转为错误返回，避免宿主进程崩溃。
	// 覆盖 host 内置工具（readSpillTool / runCodeTool 等）与 RemoteTool（虽插件 SDK
	// 已 recover 一次，但若插件未用 SDK 或 RemoteTool 自身 gRPC 调用 panic 仍兜底）。
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("tool %s panicked: %v", toolName, r)
			inv.Result, inv.Err = "", err
			errRet = err
		}
	}()
	var result string
	var err error
	var viewJSON string
	if ev, ok := tool.(ViewExecutor); ok {
		result, viewJSON, err = ev.ExecuteWithView(execCtx, json.RawMessage(inv.ArgumentsJSON))
	} else {
		result, err = tool.Execute(execCtx, json.RawMessage(inv.ArgumentsJSON))
	}
	if errors.Is(err, context.DeadlineExceeded) {
		err = &ToolTimeoutError{Tool: toolName, Ms: timeoutMs}
	}
	inv.Result, inv.ViewJSON, inv.Err = result, viewJSON, err
	return err
}

// ToolResultInfo tools/result 事件的载荷（对齐 DSH tools/result）。
type ToolResultInfo struct {
	ToolName string `json:"tool_name"`
	Result   string `json:"result"`
	Error    string `json:"error,omitempty"`
	ViewJSON string `json:"view_json,omitempty"`
}

// emitToolResult 广播工具执行结果事件（非拦截）。
func (m *Manager) emitToolResult(inv *ToolInvocation) {
	info := ToolResultInfo{ToolName: inv.ToolName, Result: inv.Result, ViewJSON: inv.ViewJSON}
	if inv.Err != nil {
		info.Error = inv.Err.Error()
	}
	m.events.Emit(EventToolResult, EventContext{Data: info})
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

// isBackgroundRequest 检测工具参数 JSON 中是否声明 run_in_background: true。
// 对齐 DSH bash 的 run_in_background 参数——任何工具都可以声明后台运行。
func isBackgroundRequest(argsJSON json.RawMessage) bool {
	var p struct {
		RunInBackground bool `json:"run_in_background"`
	}
	if err := json.Unmarshal(argsJSON, &p); err != nil {
		return false
	}
	return p.RunInBackground
}

// startBackgroundTool 在 job 注册表中登记一个后台任务，异步执行完整工具流水线，
// 立即返回 job_id（对齐 DSH bash run_in_background）。
// 模型可用 job_output/job_list/job_kill 管理后台任务。
func (m *Manager) startBackgroundTool(ctx context.Context, toolName string, argsJSON json.RawMessage, caller string) (string, string, error) {
	label := toolName
	// 尝试从参数中提取 command 作为 label 更友好的展示
	var p struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(argsJSON, &p) == nil && p.Command != "" {
		label = fmt.Sprintf("%s: %s", toolName, truncateStr(p.Command, 60))
	}

	jobID, err := m.jobs.Start(jobs.StartSpec{
		Kind:  toolName,
		Label: label,
		Owner: caller,
		Start: func() (jobs.JobHooks, error) {
			doneCh := make(chan jobs.JobOutcome, 1)
			cancelCh := make(chan string, 1)

			go func() {
				// 异步执行完整流水线（pre-execute → execute → post-execute）
				result, _, err := m.executeBackgroundPipeline(ctx, toolName, argsJSON)
				if err != nil {
					doneCh <- jobs.JobOutcome{Status: jobs.StatusFailed, Detail: err.Error(), Output: result}
				} else {
					doneCh <- jobs.JobOutcome{Status: jobs.StatusCompleted, Output: result}
				}
			}()

			return jobs.JobHooks{
				Cancel: func(reason string) {
					select {
					case cancelCh <- reason:
					default:
					}
				},
				Done:       doneCh,
				ReadOutput: nil, // final-output 模式（完成后一次性返回全部输出）
			}, nil
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("start background job: %w", err)
	}

	result := fmt.Sprintf("Background job started: %s. Track it with job_output (job_id: %s). Stop with job_kill.", toolName, jobID)
	return result, "", nil
}

// executeBackgroundPipeline 在后台执行完整工具流水线（不含 run_in_background 检测，
// 避免递归）。供 startBackgroundTool 的 goroutine 调用。
func (m *Manager) executeBackgroundPipeline(ctx context.Context, toolName string, argsJSON json.RawMessage) (string, string, error) {
	inv := &ToolInvocation{ToolName: toolName, ArgumentsJSON: string(argsJSON), SessionID: CallerFromContext(ctx), ApprovalPolicy: ApprovalPolicyFromContext(ctx)}

	runErr := m.events.Waterfall(EventToolPreExecute, EventContext{Data: inv, Context: ctx}, func(EventContext) error {
		if veto := m.runPluginBeforeTool(ctx, inv); veto != nil {
			inv.Err = veto
			return veto
		}
		return inv.Err
	})
	if inv.Err == nil && runErr != nil {
		inv.Err = runErr
	}

	if inv.Err == nil {
		execErr := m.events.Waterfall(EventToolExecute, EventContext{Data: inv}, func(EventContext) error {
			return m.executeToolBody(ctx, inv, toolName)
		})
		if inv.Err == nil && execErr != nil {
			inv.Err = execErr
		}
	}

	if err := m.events.Waterfall(EventToolPostExecute, EventContext{Data: inv}, func(EventContext) error {
		m.runPluginAfterTool(ctx, inv)
		return inv.Err
	}); err != nil {
		m.emitToolResult(inv)
		return "", "", err
	}

	m.emitToolResult(inv)
	return inv.Result, inv.ViewJSON, inv.Err
}

// truncateStr 截断字符串到 max 字节（用于 job label 展示）。
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
