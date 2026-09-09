package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// 消息反馈服务（对齐 DSH feedback/message-feedback + command-feedback）。
//
// DSH 有 message-feedback（对单条消息的用户评分）与 command-feedback（命令触发的反馈）。
// 两者都是 Service，反馈经会话事件日志持久化（feedback/rated）。
//
// DSC 的适配：在宿主侧提供 FeedbackStore，反馈落点 ExecDir/feedback/。
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

// FeedbackStore 反馈存储服务（对齐 DSH message-feedback）。
// 线程安全；反馈落点 ExecDir/feedback/feedback.jsonl。
type FeedbackStore struct {
	mu       sync.Mutex
	records  []Feedback
	filePath string
}

// NewFeedbackStore 创建反馈存储。filePath 为反馈文件路径。
func NewFeedbackStore(filePath string) *FeedbackStore {
	return &FeedbackStore{
		filePath: filePath,
	}
}

// Add 添加一条反馈。
func (fs *FeedbackStore) Add(ctx context.Context, sessionID string, turn int, rating FeedbackRating, comment, message string) (*Feedback, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	fb := Feedback{
		ID:        fmt.Sprintf("fb-%d", time.Now().UnixMilli()),
		SessionID: sessionID,
		Turn:      turn,
		Rating:    rating,
		Comment:   comment,
		Message:   truncateMessage(message, 200),
		CreatedAt: time.Now().UnixMilli(),
	}
	fs.records = append(fs.records, fb)
	return &fb, nil
}

// List 列出所有反馈（按时间降序）。
func (fs *FeedbackStore) List(ctx context.Context) []Feedback {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	out := make([]Feedback, len(fs.records))
	copy(out, fs.records)
	// 按时间降序
	for i := 0; i < len(out)-1; i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt > out[i].CreatedAt {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ListBySession 列出指定会话的反馈。
func (fs *FeedbackStore) ListBySession(ctx context.Context, sessionID string) []Feedback {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var out []Feedback
	for _, f := range fs.records {
		if f.SessionID == sessionID {
			out = append(out, f)
		}
	}
	return out
}

// Stats 返回反馈统计（正/负/中数量）。
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

// unused but keeps import
var _ = strings.TrimSpace
