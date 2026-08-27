package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dsc/core"
	"dsc/jobs"
)

// TestRunJobsCommand 校验 /jobs 子命令：list 展示后台 workflow 任务、
// output 读取输出与状态、kill 处理终态任务、非法子命令给用法提示。
func TestRunJobsCommand(t *testing.T) {
	m := newRenderCacheModel(t)
	m.manager = core.NewManager(&core.ManagerConfig{})

	// 启动一个后台 workflow（真实 job，脚本立即返回）
	out, err := m.manager.ExecuteTool(context.Background(), "workflow",
		json.RawMessage(`{"meta":{"name":"test-wf","description":"test"},"script":"return 1","background":true}`))
	if err != nil || !strings.Contains(out, "workflow-1") {
		t.Fatalf("start bg workflow = %q, %v", out, err)
	}

	// list：默认子命令（空）与 "list" 等价
	m.runJobsCommand("")
	joined := strings.Join(m.lines, "\n")
	if !strings.Contains(joined, "workflow-1") || !strings.Contains(joined, "[workflow]") {
		t.Fatalf("jobs list missing task: %s", joined)
	}

	// output
	before := len(m.lines)
	m.runJobsCommand("output workflow-1")
	if len(m.lines) <= before {
		t.Fatal("jobs output produced no message")
	}
	joined = strings.Join(m.lines, "\n")
	if !strings.Contains(joined, "job workflow-1") || !strings.Contains(joined, "[status:") {
		t.Fatalf("jobs output missing: %s", joined)
	}

	// 等待 job 落定，再 kill 验证 already-finished 路径
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rd, err := m.manager.ReadJob("workflow-1"); err == nil && rd.Snapshot.Status == jobs.StatusCompleted {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.runJobsCommand("kill workflow-1")
	joined = strings.Join(m.lines, "\n")
	if !strings.Contains(joined, "已结束") {
		t.Fatalf("jobs kill finished should report already-finished: %s", joined)
	}

	// 非法子命令 → 用法提示
	m.runJobsCommand("bogus")
	joined = strings.Join(m.lines, "\n")
	if !strings.Contains(joined, "用法") {
		t.Fatalf("jobs bogus should show usage: %s", joined)
	}
}
