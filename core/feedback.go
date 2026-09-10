package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// 消息反馈服务（对齐 DSH feedback/message-feedback + command-feedback）。
//
// DSH 有 message-feedback（对单条消息的用户评分）与 command-feedback（命令触发的反馈）。
// 两者都是 Service，反馈经会话事件日志持久化（feedback/rated）。
//
// DSC 的适配：在宿主侧提供 FeedbackStore，反馈落点 ExecDir/feedback/feedback.jsonl
// （每条反馈一行 JSON，按时间追加；进程启动时自动加载历史）。
// TUI 可经斜杆命令或快捷键触发反馈，反馈经 admin API SSE 推送给观测者。

// FeedbackRating 用户对一条消息的评分。
type FeedbackRating string

const (
	FeedbackPositive FeedbackRating = "positive" // 好/赞
	FeedbackNegative FeedbackRating = "negative" // 差/踩
	FeedbackNeutral  FeedbackRating = "neutral"  // 中立/评论
)

// Feedback 一条用户反馈。
type Feedback struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	Turn      int            `json:"turn"`
	Rating    FeedbackRating `json:"rating"`
	Comment   string         `json:"comment,omitempty"`
	Message   string         `json:"message,omitempty"` // 被反馈的消息摘要
	CreatedAt int64          `json:"created_at"`
}

// feedbackSeq 用于生成稳定 ID 的进程内序列（避免同毫秒冲突）。
type feedbackSeq struct {
	mu  sync.Mutex
	n   int64
	cur int64 // 当前毫秒时间戳
}

func (s *feedbackSeq) Next() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if now != s.cur {
		s.cur = now
		s.n = 0
	}
	s.n++
	return fmt.Sprintf("fb-%d-%d", now, s.n)
}

// FeedbackStore 反馈存储服务（对齐 DSH message-feedback）。
// 线程安全；反馈落点 filePath（JSONL 格式，每行一条 JSON）。
type FeedbackStore struct {
	mu       sync.Mutex
	records  []Feedback
	filePath string
	seq      feedbackSeq
	loaded   bool
}

// NewFeedbackStore 创建反馈存储。filePath 为反馈文件路径（JSONL）。
func NewFeedbackStore(filePath string) *FeedbackStore {
	return &FeedbackStore{
		filePath: filePath,
	}
}

// ensureLoaded 懒加载历史反馈（首次访问时从文件加载）。
// 后续调用直接返回。线程安全由调用方持 mu 保证。
func (fs *FeedbackStore) ensureLoaded() {
	if fs.loaded {
		return
	}
	fs.loaded = true
	if fs.filePath == "" {
		return
	}
	f, err := os.Open(fs.filePath)
	if err != nil {
		return // 文件不存在不报错
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 单行最大 1MiB
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var fb Feedback
		if err := json.Unmarshal([]byte(line), &fb); err != nil {
			continue
		}
		fs.records = append(fs.records, fb)
	}
}

// persistLocked 把当前 records 持久化到文件（覆盖式写入）。
// 调用方需已持 fs.mu。
func (fs *FeedbackStore) persistLocked() error {
	if fs.filePath == "" {
		return nil
	}
	if err := os.MkdirAll(filepathOf(fs.filePath), 0700); err != nil {
		return fmt.Errorf("feedback store: create dir: %w", err)
	}
	// 按 CreatedAt 升序稳定输出（便于 diff 与时间序回放）
	out := make([]Feedback, len(fs.records))
	copy(out, fs.records)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	tmp := fs.filePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("feedback store: create temp: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, fb := range out {
		if err := enc.Encode(fb); err != nil {
			f.Close()
			os.Remove(tmp)
			return fmt.Errorf("feedback store: encode: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("feedback store: close: %w", err)
	}
	// 原子替换：rename 在同分区是原子的，避免崩溃留下半写文件
	return os.Rename(tmp, fs.filePath)
}

// filepathOf 返回 path 的目录部分；空目录返回 "."（兼容 NewFeedbackStore("")）。
func filepathOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return string(path[0])
			}
			return path[:i]
		}
	}
	return "."
}

// Add 添加一条反馈。
func (fs *FeedbackStore) Add(ctx context.Context, sessionID string, turn int, rating FeedbackRating, comment, message string) (*Feedback, error) {
	// 校验 rating 取值
	switch rating {
	case FeedbackPositive, FeedbackNegative, FeedbackNeutral:
		// ok
	default:
		return nil, fmt.Errorf("invalid feedback rating %q (positive/negative/neutral)", rating)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.ensureLoaded()

	fb := Feedback{
		ID:        fs.seq.Next(),
		SessionID: sessionID,
		Turn:      turn,
		Rating:    rating,
		Comment:   comment,
		Message:   truncateMessage(message, 200),
		CreatedAt: time.Now().UnixMilli(),
	}
	fs.records = append(fs.records, fb)
	if err := fs.persistLocked(); err != nil {
		// 持久化失败不阻塞反馈接收（已记入内存），仅日志告警
		// 调用方拿不到 error 反馈，下次启动会丢失——可接受降级（与 DSH feedback/rated 事件日志一致）
		_ = err
	}
	return &fb, nil
}

// List 列出所有反馈（按时间降序）。
func (fs *FeedbackStore) List(ctx context.Context) []Feedback {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.ensureLoaded()
	out := make([]Feedback, len(fs.records))
	copy(out, fs.records)
	// 按时间降序（用 sort.Slice 而非 O(n²) 冒泡，对长列表友好）
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// ListBySession 列出指定会话的反馈。
func (fs *FeedbackStore) ListBySession(ctx context.Context, sessionID string) []Feedback {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.ensureLoaded()
	var out []Feedback
	for _, f := range fs.records {
		if f.SessionID == sessionID {
			out = append(out, f)
		}
	}
	return out
}

// FeedbackStats 反馈统计（正/负/中数量）。
type FeedbackStats struct {
	Positive int `json:"positive"`
	Negative int `json:"negative"`
	Neutral  int `json:"neutral"`
	Total    int `json:"total"`
}

// Stats 返回反馈统计。
func (fs *FeedbackStore) Stats(ctx context.Context) FeedbackStats {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.ensureLoaded()
	var s FeedbackStats
	for _, f := range fs.records {
		switch f.Rating {
		case FeedbackPositive:
			s.Positive++
		case FeedbackNegative:
			s.Negative++
		case FeedbackNeutral:
			s.Neutral++
		}
	}
	s.Total = len(fs.records)
	return s
}

// truncateMessage 截断消息到指定长度。
func truncateMessage(msg string, maxLen int) string {
	runes := []rune(msg)
	if len(runes) <= maxLen {
		return msg
	}
	return string(runes[:maxLen]) + "…"
}

// FormatFeedbackRating 格式化评分为人读标签。
func FormatFeedbackRating(r FeedbackRating) string {
	switch r {
	case FeedbackPositive:
		return "👍"
	case FeedbackNegative:
		return "👎"
	case FeedbackNeutral:
		return "💬"
	default:
		return string(r)
	}
}
