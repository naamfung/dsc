// Package cron 提供 cron 定时任务：任务定义、JSON 持久化与 robfig/cron 调度。
//
// 触发时由宿主注入的执行器（Manager.RunSubagent）执行，走宿主侧 LLM + 工具
// 流水线，不占用主 agent 的交互会话。
package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Job 一条定时任务定义。
type Job struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Cron          string `json:"cron"` // 标准 5 段 cron 表达式（分 时 日 月 周）
	Prompt        string `json:"prompt"`
	Enabled       bool   `json:"enabled"`
	MaxIterations int    `json:"max_iterations,omitempty"`
	CreatedAt     int64  `json:"created_at"`
	LastRunAt     int64  `json:"last_run_at,omitempty"`
	LastStatus    string `json:"last_status,omitempty"` // "success" | "error" | "running"
	LastOutput    string `json:"last_output,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// Store 任务定义的 JSON 持久化（单文件 cron.json，原子写：临时文件 + 重命名）。
type Store struct {
	mu   sync.Mutex
	path string
	jobs map[string]*Job
}

// NewStore 打开（必要时创建）cron.json 存储。返回 nil 表示目录不可用？不——
// 出错即返回错误，任务持久化是功能前提。
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cron: create dir: %w", err)
	}
	s := &Store{
		path: filepath.Join(dir, "cron.json"),
		jobs: make(map[string]*Job),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 首次使用，无历史任务
		}
		return fmt.Errorf("cron: read %s: %w", s.path, err)
	}
	var list []*Job
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("cron: parse %s: %w", s.path, err)
	}
	for _, j := range list {
		if j.ID != "" {
			s.jobs[j.ID] = j
		}
	}
	return nil
}

// List 返回所有任务（按创建时间排序，稳定输出）。
func (s *Store) List() []*Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	ordered := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		ordered = append(ordered, j)
	}
	sort.Slice(ordered, func(i, k int) bool { return ordered[i].CreatedAt < ordered[k].CreatedAt })
	return ordered
}

// Get 返回指定任务（副本）。
func (s *Store) Get(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	c := *j
	return &c, true
}

// Save 保存任务（新增或更新）。
func (s *Store) Save(j *Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[j.ID] = j
	return s.writeLocked()
}

// Remove 删除任务；不存在返回 false。
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[id]; !ok {
		return false
	}
	delete(s.jobs, id)
	if err := s.writeLocked(); err != nil {
		return false
	}
	return true
}

// writeLocked 原子写整个任务列表（需已持有 s.mu）。
func (s *Store) writeLocked() error {
	list := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("cron: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("cron: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("cron: rename %s -> %s: %w", tmp, s.path, err)
	}
	return nil
}

// newID 生成任务 id：时间戳毫秒（调用方保证单进程内唯一）。
func newID() string {
	return fmt.Sprintf("cron-%d", time.Now().UnixMilli())
}
