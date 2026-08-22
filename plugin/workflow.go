package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"dsc/workflow"
)

// workflowTool 宿主内置 workflow 工具（对齐 DSH tool-workflow）：
// 运行模型编写的 JS 编排脚本，可扇出 subagent，返回脚本最终值。
// agent() 钩子经 RunSubagent 执行（宿主侧 LLM+工具流水线，不占用主 agent 会话）。
type workflowTool struct{ m *Manager }

func (t *workflowTool) Name() string { return "workflow" }

func (t *workflowTool) Description() string {
	return "Write and run a JavaScript orchestration script that fans work out across many " +
		"subagents with phases and structured results. Use ONLY when the user explicitly asks " +
		"for a workflow or for large multi-agent orchestration; for one or two delegations, " +
		"prefer the subagent tool.\n" +
		"Script conventions: the script is async JavaScript (await supported). " +
		"Globals: args (the JSON input object passed as `args`); agent(prompt, options?) runs " +
		"one subagent and returns a Promise resolving to its result text, or null if the subagent " +
		"failed (await it; options may include `label`); parallel(thunks) runs thunks concurrently " +
		"under the configured agent concurrency limit and resolves to an array of their results in " +
		"order (each thunk is typically () => agent(prompt)); pipeline(items, ...stages) passes " +
		"each item through the stage functions (previous, item, index) in order with items " +
		"processed concurrently — a stage failure drops that item to null, fatal errors abort the " +
		"whole run; phase(title) and log(msg) record progress (phase titles must match the declared " +
		"meta.phases). The script ends with `return <json-value>`, which becomes the workflow result. " +
		"Set background:true to start it as a background job and get its job id immediately; track " +
		"it with job_output / job_kill instead of blocking the current turn."
}

func (t *workflowTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"meta": {
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Short lower-kebab-case workflow name (required)."},
					"description": {"type": "string", "description": "One-line description of what the workflow does (required)."},
					"when_to_use": {"type": "string"},
					"phases": {"type": "array", "items": {"type": "object", "properties": {
						"title": {"type": "string"},
						"detail": {"type": "string"}
					}}}
				},
				"required": ["name", "description"]
			},
			"script": {"type": "string", "description": "The plain-JavaScript workflow script body (see tool description for conventions)."},
			"args": {"type": "object", "description": "Optional JSON input exposed to the script as the 'args' global."},
			"background": {"type": "boolean", "description": "Start as a background job and return its job id immediately; track it with job_output / job_kill."}
		},
		"required": ["meta", "script"]
	}`)
}

func (t *workflowTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Meta       workflow.Meta `json:"meta"`
		Script     string        `json:"script"`
		Args       any           `json:"args"`
		Background bool          `json:"background"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("workflow: invalid args: %w", err)
	}
	req := workflow.StartRequest{
		Meta:   p.Meta,
		Script: p.Script,
		Args:   p.Args,
		Runner: workflowAgentRunner{m: t.m},
		Events: workflowEventSink{m: t.m},
	}
	// 后台模式：经 jobs 注册表启动（job_kill 取消经 jctx 传播到 workflow run），
	// 立即返回 job id，模型后续用 job_output/job_list/job_kill 管理。
	if p.Background {
		job := t.m.jobs.Start("workflow", p.Meta.Name, func(jctx context.Context) (string, error) {
			run, err := workflow.Start(jctx, req)
			if err != nil {
				return "", err
			}
			return collectWorkflow(jctx, run)
		})
		return fmt.Sprintf("workflow %q started in background (job %s). Track it with job_output.", p.Meta.Name, job.ID), nil
	}
	run, err := workflow.Start(ctx, req)
	if err != nil {
		return "", err
	}
	return collectWorkflow(ctx, run)
}

// collectWorkflow 等待运行结算并渲染结果（前台/后台共用；result 不拒绝）。
func collectWorkflow(ctx context.Context, run *workflow.Run) (string, error) {
	r := <-run.Result
	switch r.StopReason {
	case workflow.StopCompleted:
		var sb strings.Builder
		fmt.Fprintf(&sb, "workflow %q completed (%d agent(s)).\nReturn value:\n", run.Meta.Name, r.AgentsStarted)
		if r.Value == nil {
			sb.WriteString("null")
			return sb.String(), nil
		}
		b, jerr := json.MarshalIndent(r.Value, "", "  ")
		if jerr != nil {
			return "", fmt.Errorf("workflow: marshal result: %w", jerr)
		}
		sb.Write(b)
		return sb.String(), nil
	case workflow.StopCancelled:
		return "", fmt.Errorf("workflow run was cancelled")
	default:
		return "", fmt.Errorf("workflow run failed: %s", r.Error)
	}
}

// workflowAgentRunner agent() 钩子的子 agent 执行器：走宿主 RunSubagent。
type workflowAgentRunner struct{ m *Manager }

func (r workflowAgentRunner) RunAgent(ctx context.Context, prompt string) (string, error) {
	return r.m.RunSubagent(ctx, &SubagentRequest{Prompt: prompt})
}

// workflowEventSink 把工作流观测事件投递到宿主事件总线（仅供观察）。
type workflowEventSink struct{ m *Manager }

func (s workflowEventSink) Emit(name EventName, data any) {
	s.m.events.Emit(name, EventContext{Data: data})
}

func (s workflowEventSink) OnStart(id string, meta workflow.Meta) {
	s.Emit("workflow/start", map[string]any{"id": id, "meta": meta})
}

func (s workflowEventSink) OnPhase(id, title string) {
	s.Emit("workflow/phase", map[string]any{"id": id, "title": title})
}

func (s workflowEventSink) OnLog(id, msg string) {
	s.Emit("workflow/log", map[string]any{"id": id, "msg": msg})
}

func (s workflowEventSink) OnAgentStart(id string, seq int, label string) {
	s.Emit("workflow/agent-start", map[string]any{"id": id, "seq": seq, "label": label})
}

func (s workflowEventSink) OnAgentEnd(id string, seq int, outcome string) {
	s.Emit("workflow/agent-end", map[string]any{"id": id, "seq": seq, "outcome": outcome})
}

func (s workflowEventSink) OnEnd(id string, r workflow.Result) {
	s.Emit("workflow/end", map[string]any{"id": id, "stop_reason": r.StopReason, "agents_started": r.AgentsStarted})
}
