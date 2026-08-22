package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dsc/jobs"
)

// jobTool 面向模型的 job 工具族（对齐 DSH tool-jobs：job_output/job_list/job_kill）。
// 后台任务由 Manager.jobs 注册表承载（v1 生产方：workflow background）。
// v1 简化：非阻塞读取最终输出（无流式游标）、kill 转发取消请求、无完成自动唤醒。

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
		return "Read a background job's output and status. Non-blocking by default; set wait:true to block until the job settles (bounded by timeout_ms, capped at 600000). Responses end with [status: ...]."
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
	switch t.name {
	case "job_output":
		return t.execOutput(ctx, args)
	case "job_list":
		return t.execList(ctx)
	case "job_kill":
		return t.execKill(ctx, args)
	}
	return "", fmt.Errorf("job: unknown tool %q", t.name)
}

// execOutput 读取任务输出与状态（对齐 DSH job_output 文本形态）。
func (t *jobTool) execOutput(ctx context.Context, args json.RawMessage) (string, error) {
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
	waitCtx := ctx
	if p.Wait {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, deadline)
		defer cancel()
	}
	// wait：轮询直到任务落定或超时（任务保持存活，超时返回存活快照）
	for {
		j, ok := t.m.jobs.Get(p.JobID)
		if !ok {
			return "", fmt.Errorf("job_output: no such job %q", p.JobID)
		}
		if !p.Wait || j.Status != jobs.StatusRunning {
			return renderJobOutput(j), nil
		}
		select {
		case <-waitCtx.Done():
			return renderJobOutput(j), nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// renderJobOutput 渲染 job_output 结果（对齐 DSH：输出 + [status: ...] 结尾；
// 空输出用 (no output yet)）。
func renderJobOutput(j *jobs.Job) string {
	var b strings.Builder
	if j.Output != "" {
		b.WriteString(j.Output)
	} else {
		b.WriteString("(no output yet)")
	}
	b.WriteString("\n[status: " + string(j.Status) + "]")
	if j.Error != "" {
		b.WriteString(" " + j.Error)
	}
	return b.String()
}

// execList 列出任务（对齐 DSH job_list：<id> [<kind>] <status> — <label>）。
func (t *jobTool) execList(ctx context.Context) (string, error) {
	jobs := t.m.jobs.List()
	if len(jobs) == 0 {
		return "(no background jobs)", nil
	}
	var b strings.Builder
	for _, j := range jobs {
		fmt.Fprintf(&b, "%s [%s] %s — %s\n", j.ID, j.Kind, j.Status, j.Label)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// execKill 请求取消任务（对齐 DSH job_kill：请求取消 / 已结束返回终态）。
func (t *jobTool) execKill(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		JobID  string `json:"job_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("job_kill: invalid args: %w", err)
	}
	j, ok := t.m.jobs.Get(p.JobID)
	if !ok {
		return "", fmt.Errorf("job_kill: no such job %q", p.JobID)
	}
	if j.Status != jobs.StatusRunning {
		return fmt.Sprintf("job %s already finished [status: %s]", p.JobID, j.Status), nil
	}
	t.m.jobs.Kill(p.JobID)
	return fmt.Sprintf("requested cancellation of job %s", p.JobID), nil
}
