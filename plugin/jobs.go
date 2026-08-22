package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dsc/jobs"
)

// ctxCallerKey context 键：从工具调用链路解析出的调用方会话标识。
type ctxCallerKey struct{}

// WithCaller 把调用方会话标识注入 ctx。
func WithCaller(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, ctxCallerKey{}, sessionID)
}

// CallerFromContext 读取 ctx 中的调用方会话标识（空 = 无会话调用方）。
func CallerFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxCallerKey{}).(string); ok {
		return v
	}
	return ""
}

// jobTool 面向模型的 job 工具族（对齐 DSH tool-jobs：job_output/job_list/job_kill）。
// 后台任务由 Manager.jobs 注册表承载（owner 隔离 + 消费式游标；v1 生产方：
// workflow background）。caller（调用方会话）经工具调用链路注入 ctx。

// defaultWaitTimeout 与 maxWaitTimeout job_output 的 wait 默认/上限等待时间（对齐 DSH）。
const (
	defaultWaitTimeout = 30 * time.Second
	maxWaitTimeout     = 600 * time.Second
)

type jobTool struct {
	m    *Manager
	name string
}

func (t *jobTool) Name() string { return t.name }

func (t *jobTool) Description() string {
	switch t.name {
	case "job_output":
		return "Read a background job's output and status. Non-blocking by default; set wait:true to block until the job settles (bounded by timeout_ms, capped at 600000). Stream jobs return the next output delta; final-output jobs return the terminal output once settled. Responses end with [status: ...]."
	case "job_list":
		return "List background jobs visible to this caller as '<id> [<kind>] <status> — <label>', one per line."
	case "job_kill":
		return "Request cancellation of a background job, forwarding an optional reason. A finished job returns its terminal status instead."
	}
	return ""
}

func (t *jobTool) ParametersSchema() json.RawMessage {
	switch t.name {
	case "job_output":
		return json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string"},"wait":{"type":"boolean"},"timeout_ms":{"type":"integer"}},"required":["job_id"],"additionalProperties":false}`)
	case "job_list":
		return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	case "job_kill":
		return json.RawMessage(`{"type":"object","properties":{"job_id":{"type":"string"},"reason":{"type":"string"}},"required":["job_id"],"additionalProperties":false}`)
	}
	return json.RawMessage(`{}`)
}

func (t *jobTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	caller := CallerFromContext(ctx)
	switch t.name {
	case "job_output":
		return t.execOutput(ctx, args, caller)
	case "job_list":
		return t.execList(caller)
	case "job_kill":
		return t.execKill(args, caller)
	}
	return "", fmt.Errorf("job: unknown tool %q", t.name)
}

// execOutput 读取任务输出与状态（对齐 DSH job_output）：非阻塞默认；
// wait=true 时先有界等待落定，再消费读取。
func (t *jobTool) execOutput(ctx context.Context, args json.RawMessage, caller string) (string, error) {
	var p struct {
		JobID     string `json:"job_id"`
		Wait      bool   `json:"wait"`
		TimeoutMS int    `json:"timeout_ms"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("job_output: invalid args: %w", err)
	}
	deadline := time.Duration(p.TimeoutMS) * time.Millisecond
	if deadline <= 0 {
		deadline = defaultWaitTimeout
	}
	if deadline > maxWaitTimeout {
		deadline = maxWaitTimeout
	}
	if p.Wait {
		if _, err := t.m.jobs.Wait(p.JobID, deadline, caller); err != nil {
			return "", fmt.Errorf("job_output: %w", err)
		}
	}
	rd, err := t.m.jobs.Read(p.JobID, caller)
	if err != nil {
		return "", fmt.Errorf("job_output: %w", err)
	}
	return renderJobOutput(rd), nil
}

// renderJobOutput 渲染 job_output 结果（对齐 DSH：输出 + [status: ...] 结尾；
// 空输出用 (no output yet)）。
func renderJobOutput(rd jobs.Read) string {
	var b strings.Builder
	if rd.Text != "" {
		b.WriteString(rd.Text)
	} else {
		b.WriteString("(no output yet)")
	}
	b.WriteString("\n[status: " + string(rd.Snapshot.Status) + "]")
	if rd.Snapshot.Detail != "" {
		b.WriteString(" " + rd.Snapshot.Detail)
	}
	return b.String()
}

// execList 列出调用方可见任务（对齐 DSH job_list：<id> [<kind>] <status> — <label>）。
func (t *jobTool) execList(caller string) (string, error) {
	all := t.m.jobs.List(caller)
	if len(all) == 0 {
		return "(no background jobs)", nil
	}
	var b strings.Builder
	for _, j := range all {
		fmt.Fprintf(&b, "%s [%s] %s — %s\n", j.ID, j.Kind, j.Status, j.Label)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// execKill 请求取消任务（对齐 DSH job_kill：请求取消 / 已结束返回终态）。
func (t *jobTool) execKill(args json.RawMessage, caller string) (string, error) {
	var p struct {
		JobID  string `json:"job_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("job_kill: invalid args: %w", err)
	}
	res, err := t.m.jobs.Kill(p.JobID, caller, p.Reason)
	if err != nil {
		return "", fmt.Errorf("job_kill: %w", err)
	}
	switch res {
	case jobs.KillRequested:
		return fmt.Sprintf("requested cancellation of job %s", p.JobID), nil
	default: // already-finished
		snap, gerr := t.m.jobs.Get(p.JobID, caller)
		if gerr != nil {
			return "", fmt.Errorf("job_kill: %w", gerr)
		}
		return fmt.Sprintf("job %s already finished [status: %s]", p.JobID, snap.Status), nil
	}
}
