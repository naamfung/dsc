package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Runner 任务执行器（宿主注入：Manager.RunSubagent 执行 prompt）。
// ctx 为本次触发的执行上下文（带超时）；返回任务输出文本与错误。
type Runner func(ctx context.Context, job *Job) (string, error)

// defaultTimeout 单次任务执行的默认超时。
const defaultTimeout = 10 * time.Minute

// maxOutputLen 记录到任务状态中的输出截断长度（防止 jobs.json 膨胀）。
const maxOutputLen = 4096

// Scheduler cron 任务调度器：以标准 5 段 cron 表达式调度启用的任务；
// 同一任务上次未跑完时跳过本次触发（SkipIfStillRunning，防重入）。
type Scheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	store   *Store
	runner  Runner
	timeout time.Duration
	entries map[string]cron.EntryID // 任务 id -> 调度条目（启用且已注册时存在）
}

// NewScheduler 创建调度器。runner 为任务执行回调；timeout 为单次执行超时
// （<=0 用默认 10 分钟）。
func NewScheduler(store *Store, runner Runner, timeout time.Duration) *Scheduler {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Scheduler{
		store:   store,
		runner:  runner,
		timeout: timeout,
		entries: make(map[string]cron.EntryID),
	}
}

// Start 启动调度：注册所有启用的任务。已启动时返回错误。
// 存量任务 cron 表达式无效时跳过该任务（不影响其余任务与调度器本身）。
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		return fmt.Errorf("jobs: scheduler already started")
	}
	s.cron = cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	for _, j := range s.store.List() {
		if !j.Enabled {
			continue
		}
		if eid, err := s.cron.AddFunc(j.Cron, func() { s.fire(j.ID) }); err == nil {
			s.entries[j.ID] = eid
		}
	}
	s.cron.Start()
	return nil
}

// Stop 停止调度（不等待运行中的任务：运行任务自带超时，独立于调度器）。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron == nil {
		return
	}
	s.cron.Stop()
	s.cron = nil
	s.entries = make(map[string]cron.EntryID)
}

// Add 新增任务：校验 cron 表达式并持久化；调度器运行中且任务启用时立即注册。
func (s *Scheduler) Add(j *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := cron.ParseStandard(j.Cron); err != nil {
		return fmt.Errorf("jobs: invalid cron %q: %w", j.Cron, err)
	}
	if j.ID == "" {
		j.ID = newID()
	}
	if j.CreatedAt == 0 {
		j.CreatedAt = time.Now().UnixMilli()
	}
	if err := s.store.Save(j); err != nil {
		return err
	}
	if j.Enabled && s.cron != nil {
		eid, err := s.cron.AddFunc(j.Cron, func() { s.fire(j.ID) })
		if err != nil {
			return fmt.Errorf("jobs: schedule %q: %w", j.ID, err)
		}
		s.entries[j.ID] = eid
	}
	return nil
}

// Remove 删除任务并取消其调度；任务不存在返回错误。
func (s *Scheduler) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.store.Remove(id) {
		return fmt.Errorf("jobs: job %q not found", id)
	}
	if eid, ok := s.entries[id]; ok {
		s.cron.Remove(eid)
		delete(s.entries, id)
	}
	return nil
}

// SetEnabled 启用/停用任务并同步调度（调度器未启动时仅持久化，启动时统一注册）。
func (s *Scheduler) SetEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.store.Get(id)
	if !ok {
		return fmt.Errorf("jobs: job %q not found", id)
	}
	job.Enabled = enabled
	if err := s.store.Save(job); err != nil {
		return err
	}
	if s.cron == nil {
		return nil
	}
	if enabled {
		if _, exists := s.entries[id]; !exists {
			eid, err := s.cron.AddFunc(job.Cron, func() { s.fire(id) })
			if err != nil {
				return fmt.Errorf("jobs: schedule %q: %w", id, err)
			}
			s.entries[id] = eid
		}
	} else if eid, exists := s.entries[id]; exists {
		s.cron.Remove(eid)
		delete(s.entries, id)
	}
	return nil
}

// List 返回全部任务（含运行状态快照）。
func (s *Scheduler) List() []*Job {
	return s.store.List()
}

// Get 返回指定任务。
func (s *Scheduler) Get(id string) (*Job, bool) {
	return s.store.Get(id)
}

// fire 任务触发点：由 cron 调度线程调用，标记 running 后异步执行。
func (s *Scheduler) fire(id string) {
	s.mu.Lock()
	job, ok := s.store.Get(id)
	runner := s.runner
	timeout := s.timeout
	s.mu.Unlock()
	if !ok || !job.Enabled || runner == nil {
		return
	}
	job.LastRunAt = time.Now().UnixMilli()
	job.LastStatus = "running"
	job.LastError = ""
	if err := s.store.Save(job); err != nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		output, err := runner(ctx, job)
		job.LastRunAt = time.Now().UnixMilli()
		if err != nil {
			job.LastStatus = "error"
			job.LastError = err.Error()
			job.LastOutput = ""
		} else {
			job.LastStatus = "success"
			job.LastOutput = truncate(output)
		}
		_ = s.store.Save(job)
	}()
}

// truncate 截断任务输出到 maxOutputLen。
func truncate(s string) string {
	if len(s) <= maxOutputLen {
		return s
	}
	return s[:maxOutputLen]
}
