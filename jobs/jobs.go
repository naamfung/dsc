// Package jobs 后台任务注册表（对齐 DSH jobs 完整契约）。
//
// 生产方声明 kind/label/owner，经 Start 预检后同步启动并登记；运行时拥有
// 身份、访问授权与生命周期状态。访问按 owner 会话隔离：有 owner 的任务仅
// 其 owner（或空调用方仅限无 owner 任务）可见/可读/可取消。
//
// 输出模型二选一（由生产方决定）：
//   - 流式任务（提供 ReadOutput）：read 消费"自上次以来的增量"，单一游标；
//   - 最终输出任务（不提供 ReadOutput）：终止后 read 幂等返回最终输出。
//
// 状态机对齐 DSH：running → (stopping) → completed/killed/failed；settlement
// 首次结果优先。reported 标志在 kill/read/wait 触及终态时置位，供完成通知
// 抑制重复（v1 无自动唤醒，字段保留）。
package jobs

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// JobStatus 任务生命周期（对齐 DSH：running、可选 stopping，随后恰好一个终态）。
type JobStatus string

const (
	StatusRunning   JobStatus = "running"
	StatusStopping  JobStatus = "stopping"
	StatusCompleted JobStatus = "completed"
	StatusKilled    JobStatus = "killed"
	StatusFailed    JobStatus = "failed"
)

// KillResult kill 的结果（对齐 DSH：requested / already-finished）。
type KillResult string

const (
	KillRequested       KillResult = "requested"
	KillAlreadyFinished KillResult = "already-finished"
)

// JobHooks 生产方钩子（对齐 DSH JobHooks）。Done 在生产方释放资源后交付终态。
type JobHooks struct {
	// Cancel 请求终止：同步、幂等，最终 settle Done；reason 原样转发。
	Cancel func(reason string)
	// Done 终态结果（completed/killed/failed）。
	Done <-chan JobOutcome
	// ReadOutput 消费自上次调用以来的输出增量；nil 标记最终输出任务。
	ReadOutput func() string
}

// JobOutcome 生产方终态结果（对齐 DSH JobOutcome）。
type JobOutcome struct {
	Status JobStatus // completed | killed | failed
	Detail string    // kind 特有事实（如 "exit code: 3"）
	Output string    // final-output 任务的最终输出；stream 任务不设
}

// StartSpec 启动声明（对齐 DSH JobStart）。
type StartSpec struct {
	Kind  string // 生产方 kind，亦为 id 前缀（如 "workflow"）
	Label string
	// OutputLimitBytes 生产方拥有的模型呈现上限（每次完整读取/通知）；<=0 不设限。
	// 注册表不重写生产方输出，由生产方自行应用。
	OutputLimitBytes int
	// Owner owner 会话标识；空 = 无 owner（开放给所有调用方，存续到服务释放）。
	Owner string
	// Start 预检后同步启动工作并返回钩子；返回错误则不登记、不启动。
	Start func() (JobHooks, error)
}

// Read 一次读取结果（对齐 DSH JobRead）。
type Read struct {
	Text     string
	Snapshot JobSnapshot
}

// JobSnapshot 任务只读快照（每次 fresh 投影，非活跃注册表状态）。
type JobSnapshot struct {
	ID               string
	Kind             string
	Label            string
	OutputLimitBytes int
	Owner            string // owner 会话标识；空 = 无 owner
	Status           JobStatus
	Detail           string
	StartedAt        time.Time
	FinishedAt       time.Time // 零值 = 未落定
	// Reported kill/read/wait 触及终态时置位，供完成通知抑制重复。
	Reported bool
}

// Registry 后台任务注册表（线程安全）。
type Registry struct {
	mu            sync.Mutex
	jobs          map[string]*Job
	order         []string // 启动顺序（List 按此返回）
	seq           int
	doneListeners map[int]func(JobSnapshot) // 完成监听器（contained，落定时通知）
	nextListener  int
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{jobs: make(map[string]*Job)}
}

// maxRetainedJobs 保留的终态任务数上限；超过则淘汰最旧的终态任务，
// 防止已完成 job 在 r.jobs/order 无限累积（P3-2）。
const maxRetainedJobs = 256

// pruneLocked 淘汰最旧的终态 job，直到终态数量不超过 maxRetainedJobs（需已持有 r.mu）。
func (r *Registry) pruneLocked() {
	terminal := 0
	for _, id := range r.order {
		if isTerminalStatus(r.jobs[id].snapshot.Status) {
			terminal++
		}
	}
	over := terminal - maxRetainedJobs
	if over <= 0 {
		return
	}
	kept := make([]string, 0, len(r.order))
	for _, id := range r.order {
		if over > 0 && isTerminalStatus(r.jobs[id].snapshot.Status) {
			delete(r.jobs, id)
			over--
			continue
		}
		kept = append(kept, id)
	}
	r.order = kept
}

// isTerminalStatus 判断是否终态（可被淘汰）。
func isTerminalStatus(s JobStatus) bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusKilled
}

// Job 注册表内部记录（公开快照经 Snapshot 投影）。
type Job struct {
	snapshot   JobSnapshot
	output     string // final-output 终态输出（幂等读）
	cancel     func(reason string)
	readOutput func() string // nil = final-output-only
	settled    chan struct{} // 落定后 close
}

// Start 预检 + 同步启动 + 登记（对齐 DSH）：预检失败或 Start 抛错都不产生
// 任务；成功返回后登记不可失败。done 在后台落定。
func (r *Registry) Start(spec StartSpec) (string, error) {
	if strings.TrimSpace(spec.Kind) == "" {
		return "", fmt.Errorf("jobs: kind is required")
	}
	if spec.Start == nil {
		return "", fmt.Errorf("jobs: starter is required")
	}
	hooks, err := spec.Start()
	if err != nil {
		return "", err
	}
	r.mu.Lock()
	r.seq++
	id := fmt.Sprintf("%s-%d", spec.Kind, r.seq)
	j := &Job{
		snapshot: JobSnapshot{
			ID: id, Kind: spec.Kind, Label: spec.Label,
			OutputLimitBytes: spec.OutputLimitBytes,
			Owner:            spec.Owner,
			Status:           StatusRunning,
			StartedAt:        time.Now(),
		},
		cancel:     hooks.Cancel,
		readOutput: hooks.ReadOutput,
		settled:    make(chan struct{}),
	}
	r.jobs[id] = j
	r.order = append(r.order, id)
	r.mu.Unlock()

	go func() {
		outcome := <-hooks.Done
		r.mu.Lock()
		j.snapshot.FinishedAt = time.Now()
		switch outcome.Status {
		case StatusCompleted:
			j.snapshot.Status = StatusCompleted
			j.output = outcome.Output
		case StatusKilled:
			j.snapshot.Status = StatusKilled
		case StatusFailed:
			j.snapshot.Status = StatusFailed
		default:
			j.snapshot.Status = StatusFailed
			outcome.Detail = "producer settled with invalid status"
		}
		j.snapshot.Detail = outcome.Detail
		close(j.settled)
		// 终态任务淘汰：防已完成 job 在 r.jobs/order 无限累积（P3-2）
		r.pruneLocked()
		// 完成监听器（contained）：在锁外逐个调用，异常隔离，不阻塞落定
		listeners := make([]func(JobSnapshot), 0, len(r.doneListeners))
		for _, fn := range r.doneListeners {
			listeners = append(listeners, fn)
		}
		snap := j.snapshot
		r.mu.Unlock()
		for _, fn := range listeners {
			func() {
				defer func() { _ = recover() }()
				fn(snap)
			}()
		}
	}()
	return id, nil
}

// OnJobDone 注册完成监听器（对齐 DSH onJobDone）：每个落定记录都会通知所有
// 监听器，携带 fresh 快照。监听器异常被隔离（contained），不影响落定与其他
// 监听器。返回取消函数（幂等）。
func (r *Registry) OnJobDone(fn func(snapshot JobSnapshot)) func() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.doneListeners == nil {
		r.doneListeners = make(map[int]func(JobSnapshot))
	}
	id := r.nextListener
	r.nextListener++
	r.doneListeners[id] = fn
	return func() {
		r.mu.Lock()
		delete(r.doneListeners, id)
		r.mu.Unlock()
	}
}

// authorize 校验任务存在且调用方被授权（owner 隔离：空调用方仅限无 owner 任务）。
func (r *Registry) authorize(id, caller string) (*Job, error) {
	j, ok := r.jobs[id]
	if !ok {
		return nil, fmt.Errorf("jobs: unknown job %q", id)
	}
	if j.snapshot.Owner != "" && j.snapshot.Owner != caller {
		return nil, fmt.Errorf("jobs: job %q belongs to another session", id)
	}
	return j, nil
}

// Get 返回非消费快照（不改变游标与 reported）。
func (r *Registry) Get(id, caller string) (JobSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, err := r.authorize(id, caller)
	if err != nil {
		return JobSnapshot{}, err
	}
	return j.snapshot, nil
}

// List 返回调用方可见任务（自己拥有 + 无 owner），按启动顺序。
func (r *Registry) List(caller string) []JobSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobSnapshot, 0, len(r.order))
	for _, id := range r.order {
		j := r.jobs[id]
		if j.snapshot.Owner == "" || j.snapshot.Owner == caller {
			out = append(out, j.snapshot)
		}
	}
	return out
}

// ListAll 返回全部任务快照（宿主管理视图：TUI /jobs 使用，不做 owner 隔离）。
func (r *Registry) ListAll() []JobSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]JobSnapshot, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.jobs[id].snapshot)
	}
	return out
}

// readLocked 读取单个任务输出（需已持有锁）：流式任务消费自上次以来的增量；
// 最终输出任务在终止后幂等返回终态输出（运行中为空）。终止读取标记 reported。
func (r *Registry) readLocked(j *Job) Read {
	if j.readOutput != nil {
		return Read{Text: j.readOutput(), Snapshot: j.snapshot}
	}
	if j.snapshot.Status != StatusRunning && j.snapshot.Status != StatusStopping {
		j.snapshot.Reported = true
		return Read{Text: j.output, Snapshot: j.snapshot}
	}
	return Read{Text: "", Snapshot: j.snapshot}
}

// Read 读取输出：流式任务消费自上次以来的增量（单一游标）；最终输出任务在
// 终止后幂等返回终态输出（运行中为空）。终止读取标记 reported。
func (r *Registry) Read(id, caller string) (Read, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, err := r.authorize(id, caller)
	if err != nil {
		return Read{}, err
	}
	return r.readLocked(j), nil
}

// ReadHost 读取任意任务输出与状态（宿主管理视图：TUI /jobs output，跳过 owner 隔离）。
func (r *Registry) ReadHost(id string) (Read, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return Read{}, fmt.Errorf("jobs: unknown job %q", id)
	}
	return r.readLocked(j), nil
}

// killLocked 请求取消单个任务（需已持有锁）：先标记 stopping + reported，再调用
// 生产方 cancel（reason 转发）。已终态返回 already-finished（幂等，不重复取消）。
func (r *Registry) killLocked(j *Job, reason string) KillResult {
	if j.snapshot.Status != StatusRunning && j.snapshot.Status != StatusStopping {
		return KillAlreadyFinished
	}
	j.snapshot.Status = StatusStopping
	j.snapshot.Reported = true
	if j.cancel != nil {
		j.cancel(reason)
	}
	return KillRequested
}

// Kill 请求取消：先标记 stopping + reported，再调用生产方 cancel（reason 转发）。
// 已终态返回 already-finished（幂等，不重复取消）。
func (r *Registry) Kill(id, caller, reason string) (KillResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, err := r.authorize(id, caller)
	if err != nil {
		return "", err
	}
	return r.killLocked(j, reason), nil
}

// KillHost 请求取消任意任务（宿主管理视图：TUI /jobs kill，跳过 owner 隔离）。
func (r *Registry) KillHost(id, reason string) (KillResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return "", fmt.Errorf("jobs: unknown job %q", id)
	}
	return r.killLocked(j, reason), nil
}

// Wait 有界等待落定（不取消任务）：超时返回存活快照；落定后返回终态快照并
// 标记 reported（该等待方已获知终态）。
func (r *Registry) Wait(id string, timeout time.Duration, caller string) (JobSnapshot, error) {
	r.mu.Lock()
	j, err := r.authorize(id, caller)
	if err != nil {
		r.mu.Unlock()
		return JobSnapshot{}, err
	}
	if j.snapshot.Status != StatusRunning && j.snapshot.Status != StatusStopping {
		r.mu.Unlock()
		return j.snapshot, nil
	}
	settled := j.settled
	r.mu.Unlock()

	select {
	case <-settled:
		r.mu.Lock()
		defer r.mu.Unlock()
		j.snapshot.Reported = true
		return j.snapshot, nil
	case <-time.After(timeout):
		r.mu.Lock()
		defer r.mu.Unlock()
		return j.snapshot, nil
	}
}
