package plugin

import (
	"context"
	"fmt"
	"path/filepath"

	"dsc/jobs"
)

// StartJobs 启动 cron 定时任务调度器。任务执行经 RunSubagent（宿主侧 LLM + 工具
// 流水线），不占用主 agent 的交互会话；任务定义持久化于 ExecDir/jobs/jobs.json。
func (m *Manager) StartJobs() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobScheduler != nil {
		return fmt.Errorf("jobs: scheduler already started")
	}
	store, err := jobs.NewStore(filepath.Join(m.config.ExecDir, "jobs"))
	if err != nil {
		return fmt.Errorf("jobs: %w", err)
	}
	sch := jobs.NewScheduler(store, m.runJob, 0)
	if err := sch.Start(); err != nil {
		return fmt.Errorf("jobs: %w", err)
	}
	m.jobScheduler = sch
	m.logger.Info("jobs scheduler started", "dir", filepath.Join(m.config.ExecDir, "jobs"))
	return nil
}

// StopJobs 停止任务调度器（运行中的任务自带超时，独立于调度器）。
// 调用方需自行持有/不持有 m.mu——Shutdown 内部直接操作，避免重复加锁。
func (m *Manager) StopJobs() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopJobsLocked()
}

func (m *Manager) stopJobsLocked() {
	if m.jobScheduler != nil {
		m.jobScheduler.Stop()
		m.jobScheduler = nil
	}
}

// runJob 任务执行器：把任务的 prompt 交给 RunSubagent 执行。
func (m *Manager) runJob(ctx context.Context, job *jobs.Job) (string, error) {
	return m.RunSubagent(ctx, &SubagentRequest{Prompt: job.Prompt, MaxIterations: job.MaxIterations})
}

// AddJob 新增定时任务。
func (m *Manager) AddJob(j *jobs.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobScheduler == nil {
		return fmt.Errorf("jobs: scheduler not started")
	}
	return m.jobScheduler.Add(j)
}

// RemoveJob 删除定时任务。
func (m *Manager) RemoveJob(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobScheduler == nil {
		return fmt.Errorf("jobs: scheduler not started")
	}
	return m.jobScheduler.Remove(id)
}

// ListJobs 列出全部定时任务。
func (m *Manager) ListJobs() []*jobs.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobScheduler == nil {
		return nil
	}
	return m.jobScheduler.List()
}

// SetJobEnabled 启用/停用定时任务。
func (m *Manager) SetJobEnabled(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobScheduler == nil {
		return fmt.Errorf("jobs: scheduler not started")
	}
	return m.jobScheduler.SetEnabled(id, enabled)
}
