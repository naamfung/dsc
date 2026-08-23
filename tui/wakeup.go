package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbletea/v2"
	"dsc/jobs"
)

// 后台任务完成通知唤醒（对齐 DSH tool-jobs 的 completionDelivery: wakeup）。
// Manager 侧 jobs 落定 → OnJobDone 回调 → tea.Program.Send(jobDoneMsg) 送进
// 事件循环：空闲时自动开启一轮（完成通知作为输入，模型据此 job_output 收集）；
// 忙碌或预算耗尽时排队，下一条用户消息前置注入。用户消息恢复唤醒预算。

// maxConsecutiveWakes 可由唤醒开启的连续轮数上限（对齐 DSH 默认 3），
// 防自激：被唤醒的一轮可能启动新的后台任务，其完成又唤醒同一 owner。
const maxConsecutiveWakes = 3

// jobDoneMsg 后台任务完成事件（Manager 侧 jobs 落定回调 → 事件循环）。
type jobDoneMsg struct {
	snapshot jobs.JobSnapshot
}

// renderJobDoneNotice 渲染完成通知文本（对齐 DSH：
// background job <id> (<kind>: <label>) finished [status: ...]）。
func renderJobDoneNotice(s jobs.JobSnapshot) string {
	return fmt.Sprintf("background job %s (%s: %s) finished [status: %s]. Read its output with job_output.",
		s.ID, s.Kind, s.Label, s.Status)
}

// noticeBubble 渲染完成通知行（系统提示样式，不冒充用户消息）。
func noticeBubble(text string) string {
	return dimSty.Render(text)
}

// startTurn 启动一轮 agent 运行（Enter 用户输入与完成通知唤醒共用）。
func (m *Model) startTurn(text, rendered string) tea.Cmd {
	m.appendMessage(rendered)
	m.thinking = true
	m.syncInputHeight()
	m.render()
	m.viewport.GotoBottom()
	return tea.Batch(m.submitCmd(text), m.spinner.Tick)
}

// tryWakeup 空闲且预算未耗尽时，把排队通知作为新轮输入自动发送
// （对齐 DSH：完成通知唤醒空闲 owner；忙碌/预算耗尽时通知排队等下一轮）。
// 返回 nil 表示本轮不自动唤醒。
func (m *Model) tryWakeup() tea.Cmd {
	if m.thinking || m.streaming {
		return nil
	}
	if len(m.pendingWakeups) == 0 {
		return nil
	}
	if m.wakeBudget <= 0 {
		return nil
	}
	m.wakeBudget--
	text := strings.Join(m.pendingWakeups, "\n\n")
	m.pendingWakeups = nil
	return m.startTurn(text, noticeBubble(text))
}
