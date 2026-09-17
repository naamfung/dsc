package core

import (
	"context"
	"encoding/json"
	"fmt"

	"dsc/cron"
)

// StartCron 启动 cron 定时任务调度器。任务执行经 subagent 工具走宿主工具流水线
// （沙箱/审批门/策略插件裁决含 timeout-policy 的活跃续命超时），不占用主 agent
// 的交互会话；任务定义持久化于 ExecDir/cron/cron.json。
func (m *Manager) StartCron() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cronScheduler != nil {
		return fmt.Errorf("cron: scheduler already started")
	}
	store, err := cron.NewStore(PJoin(m.config.ExecDir, "cron"))
	if err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	sch := cron.NewScheduler(store, m.runCronJob, 0)
	if err := sch.Start(); err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	m.cronScheduler = sch
	m.logger.Info("cron scheduler started", "dir", PJoin(m.config.ExecDir, "cron"))
	return nil
}

// StopCron 停止任务调度器（运行中的任务自带超时，独立于调度器）。
// 调用方需自行持有/不持有 m.mu——Shutdown 内部直接操作，避免重复加锁。
func (m *Manager) StopCron() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCronLocked()
}

func (m *Manager) stopCronLocked() {
	if m.cronScheduler != nil {
		m.cronScheduler.Stop()
		m.cronScheduler = nil
	}
}

// runCronJob 任务执行器：把任务的 prompt 经 subagent 工具走宿主工具流水线执行——
// 与模型发起的 subagent 调用同一裁决路径（含 timeout-policy 的活跃续命超时），
// 无旁路。子代理内部工具调用照旧携带 ApprovalPolicy=never。
func (m *Manager) runCronJob(ctx context.Context, job *cron.Job) (string, error) {
	args, err := json.Marshal(struct {
		Prompt        string `json:"prompt"`
		MaxIterations int    `json:"max_iterations,omitempty"`
	}{Prompt: job.Prompt, MaxIterations: job.MaxIterations})
	if err != nil {
		return "", fmt.Errorf("cron: marshal subagent args: %w", err)
	}
	return m.ExecuteTool(ctx, "subagent", args)
}

// AddCronJob 新增定时任务。
func (m *Manager) AddCronJob(j *cron.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cronScheduler == nil {
		return fmt.Errorf("cron: scheduler not started")
	}
	return m.cronScheduler.Add(j)
}

// RemoveCronJob 删除定时任务。
func (m *Manager) RemoveCronJob(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cronScheduler == nil {
		return fmt.Errorf("cron: scheduler not started")
	}
	return m.cronScheduler.Remove(id)
}

// ListCronJobs 列出全部定时任务。
func (m *Manager) ListCronJobs() []*cron.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cronScheduler == nil {
		return nil
	}
	return m.cronScheduler.List()
}

// SetCronJobEnabled 启用/停用定时任务。
func (m *Manager) SetCronJobEnabled(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cronScheduler == nil {
		return fmt.Errorf("cron: scheduler not started")
	}
	return m.cronScheduler.SetEnabled(id, enabled)
}
