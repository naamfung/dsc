package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store 会话存储：按会话 ID 在目录下各存一个 JSONL 文件（<id>.jsonl），
// 提供多会话的创建、加载、保存、删除与列表（事件数、最后活动时间、首条
// user 内容预览）。对齐 DSH SessionStore 的存储 seam；会话内容本身仍是
// 事件溯源日志（见 persist.go）。

// SessionInfo 会话列表条目（轻量元数据，不加载全部事件）。
type SessionInfo struct {
	ID       string    `json:"id"`
	Events   int       `json:"events"`
	LastTime time.Time `json:"last_time"`
	Preview  string    `json:"preview"` // 首条 user 消息截断预览
}

// Store 按目录管理多会话。
type Store struct {
	dir    string
	mu     sync.Mutex
	nextID int // 下一个会话编号（NewStore 时按目录现有最大编号初始化）
}

// NewStore 创建/打开会话存储目录（自动创建）。
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("session store: mkdir %s: %w", dir, err)
	}
	s := &Store{dir: dir, nextID: 1}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("session store: read dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(e.Name(), "session-%d.jsonl", &n); err == nil && n >= s.nextID {
			s.nextID = n + 1
		}
	}
	return s, nil
}

// path 返回指定会话的文件路径。
func (s *Store) path(id string) string {
	return filepath.Join(s.dir, id+".jsonl")
}

// Create 创建新会话，id 为 session-<n>（按内存计数器递增，避免未落盘会话重复编号）。
func (s *Store) Create() (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := New()
	sess.id = fmt.Sprintf("session-%d", s.nextID)
	s.nextID++
	return sess, nil
}

// Load 加载指定会话；不存在返回 (nil, nil)。
func (s *Store) Load(id string) (*Session, error) {
	sess, err := Load(s.path(id))
	if err != nil {
		return nil, err
	}
	if sess != nil {
		sess.id = id
	}
	return sess, nil
}

// Ensure 加载指定会话，不存在则新建并绑定该 id（供固定会话 id 的场景复用）。
func (s *Store) Ensure(id string) (*Session, error) {
	sess, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		sess = New()
		sess.id = id
	}
	return sess, nil
}

// Save 原子落盘指定会话（按会话 ID）。
func (s *Store) Save(sess *Session) error {
	if sess.id == "" {
		return fmt.Errorf("session store: cannot save session without id")
	}
	return sess.Save(s.path(sess.id))
}

// Delete 删除指定会话；不存在视为幂等成功。
func (s *Store) Delete(id string) error {
	if err := os.Remove(s.path(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("session store: remove %s: %w", id, err)
	}
	return nil
}

// List 列出所有会话的轻量元数据（按最后活动时间倒序）。
func (s *Store) List() ([]SessionInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("session store: read dir: %w", err)
	}
	var infos []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := scanInfo(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue // 单会话损坏不影响列表
		}
		info.ID = strings.TrimSuffix(e.Name(), ".jsonl")
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool {
		if !infos[i].LastTime.Equal(infos[j].LastTime) {
			return infos[i].LastTime.After(infos[j].LastTime)
		}
		return infos[i].ID > infos[j].ID // 同毫秒按 id 倒序（编号大者较新）
	})
	return infos, nil
}

// scanInfo 轻量扫描单个 JSONL：事件数（行数）、最后事件时间、首条 user 消息预览。
func scanInfo(path string) (SessionInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionInfo{}, err
	}
	defer f.Close()
	var info SessionInfo
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		info.Events++
		var ej eventJSON
		if err := json.Unmarshal([]byte(text), &ej); err != nil {
			continue
		}
		if ej.Time > 0 {
			info.LastTime = time.UnixMilli(ej.Time)
		}
		if info.Preview == "" && ej.Type == UserMessage {
			var d UserMessageData
			if err := json.Unmarshal(ej.Data, &d); err == nil && d.Source == "user" {
				info.Preview = truncate(d.Content, 40)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return SessionInfo{}, err
	}
	return info, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
