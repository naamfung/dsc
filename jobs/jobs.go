// Package jobs 最小后台任务注册表（对齐 DSH jobs 契约的 v1 种子）：
// 后台启动立即返回 job id，运行在独立 goroutine 继续，产出/状态可查询、可取消。
//
// v1 简化（相对 DSH jobs 完整契约）：
//   - 无 owner 隔离（单会话，id 仅作可预测标识，无 SessionId 校验）
//   - 非消费式读取（job_output 返回最终输出，无流式游标）
//   - 无完成自动唤醒（模型用 job_output 轮询/wait）
//   - 生产方以 kind 区分（如 workflow），kind 可扩展
package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// JobStatus 后台任务状态。
type JobStatus string

const (
	StatusRunning   JobStatus = "running"
	StatusSucceeded JobStatus = "succeeded"
	StatusFailed    JobStatus = "failed"
	StatusCancelled JobStatus = "cancelled"
)

// Job 一个后台任务快照（非消费式；Get/List 返回同一实例的读视图）。
type Job struct {
	ID         string
	Kind       string // 生产方类型（如 "workflow"）
	Label      string
	Status     JobStatus
	Output     string
	Error      string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Registry 后台任务注册表（线程安全）。
type Registry struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	cancels map[string]context.CancelFunc
	order   []string // 启动顺序（List 按此返回）
	seq     int
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{
		jobs:    make(map[string]*Job),
		cancels: make(map[string]context.CancelFunc),
	}
}

// Start 异步启动一个后台任务：登记为 running 后立即返回；fn 在独立 goroutine
// 执行，返回的文本作为最终输出。fn 返回错误且 ctx 已取消则落定为 cancelled，
// 否则落定为 failed；成功落定为 succeeded。Kill 经取消 ctx 请求停止。
func (r *Registry) Start(kind, label string, fn func(ctx context.Context) (string, error)) *Job {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.seq++
	id := fmt.Sprintf("%s-%d", kind, r.seq)
	j := &Job{ID: id, Kind: kind, Label: label, Status: StatusRunning, StartedAt: time.Now()}
	r.jobs[id] = j
	r.cancels[id] = cancel
	r.order = append(r.order, id)
	r.mu.Unlock()

	go func() {
		out, err := fn(ctx)
		r.mu.Lock()
		defer r.mu.Unlock()
		j.Output = out
		j.FinishedAt = time.Now()
		if err != nil {
			if ctx.Err() != nil {
				j.Status = StatusCancelled
				j.Error = "cancelled"
			} else {
				j.Status = StatusFailed
				j.Error = err.Error()
			}
		} else {
			j.Status = StatusSucceeded
		}
		delete(r.cancels, id)
	}()
	return j
}

// Get 查询任务快照；不存在返回 false。
func (r *Registry) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	return j, ok
}

// List 返回全部任务快照（按启动顺序）。
func (r *Registry) List() []*Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Job, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.jobs[id])
	}
	return out
}

// Kill 请求取消运行中的任务（取消其 ctx）；已结束或不存在返回 false。
func (r *Registry) Kill(id string) bool {
	r.mu.Lock()
	cancel, ok := r.cancels[id]
	r.mu.Unlock()
	if !ok {
		return false
	}
	cancel()
	return true
}
