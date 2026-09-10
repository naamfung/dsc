package core

import (
	"context"
	"fmt"
	"path/filepath"

	"dsc/cron"
)

// StartCron 启动 cron 定时任务调度器。任务执行经 RunSubagent（宿主侧 LLM + 工具
// 流水线），不占用主 agent 的交互会话；任务定义持久化于 ExecDir/cron/cron.json。
func (m *Manager) StartCron() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cronScheduler != nil {
		return fmt.Errorf("cron: scheduler already started")
	}
	store, err := cron.NewStore(filepath.Join(m.config.ExecDir, "cron"))
	if err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	sch := cron.NewScheduler(store, m.runCronJob, 0)
	if err := sch.Start(); err != nil {
		return fmt.Errorf("cron: %w", err)
	}
	m.cronScheduler = sch
	m.logger.Info("cron scheduler started", "dir", filepath.Join(m.config.ExecDir, "cron"))
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

// runCronJob 任务执行器：把任务的 prompt 交给 RunSubagent 执行。
func (m *Manager) runCronJob(ctx context.Context, job *cron.Job) (string, error) {
	return m.RunSubagent(ctx, &SubagentRequest{Prompt: job.Prompt, MaxIterations: job.MaxIterations})
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
