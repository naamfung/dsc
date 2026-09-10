package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// newTestFeedbackStore 在临时目录创建 FeedbackStore，保证测试间隔离——
// 此前用 /tmp/test-feedback*.jsonl 固定路径，新版持久化会把上次测试遗留的反馈加载进来
// 污染本次断言（如 TestFeedbackStats 期望 4 条，实际加载 8 条）。
func newTestFeedbackStore(t *testing.T) *FeedbackStore {
	t.Helper()
	dir := t.TempDir()
	return NewFeedbackStore(filepath.Join(dir, "feedback.jsonl"))
}

func TestFeedbackAdd(t *testing.T) {
	fs := newTestFeedbackStore(t)
	ctx := context.Background()

	fb, err := fs.Add(ctx, "session-1", 1, FeedbackPositive, "很好的回答", "你好")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if fb.ID == "" {
		t.Error("feedback ID should not be empty")
	}
	if fb.Rating != FeedbackPositive {
		t.Errorf("rating = %q, want positive", fb.Rating)
	}
}

func TestFeedbackListBySession(t *testing.T) {
	fs := newTestFeedbackStore(t)
	ctx := context.Background()

	fs.Add(ctx, "session-1", 1, FeedbackPositive, "好", "msg1")
	fs.Add(ctx, "session-2", 1, FeedbackNegative, "差", "msg2")
	fs.Add(ctx, "session-1", 2, FeedbackNeutral, "评论", "msg3")

	list := fs.ListBySession(ctx, "session-1")
	if len(list) != 2 {
		t.Fatalf("expected 2 feedbacks for session-1, got %d", len(list))
	}
}

func TestFeedbackStats(t *testing.T) {
	fs := newTestFeedbackStore(t)
	ctx := context.Background()

	fs.Add(ctx, "s1", 1, FeedbackPositive, "", "")
	fs.Add(ctx, "s1", 2, FeedbackPositive, "", "")
	fs.Add(ctx, "s1", 3, FeedbackNegative, "", "")
	fs.Add(ctx, "s1", 4, FeedbackNeutral, "", "")

	stats := fs.Stats(ctx)
	if stats.Positive != 2 {
		t.Errorf("positive = %d, want 2", stats.Positive)
	}
	if stats.Negative != 1 {
		t.Errorf("negative = %d, want 1", stats.Negative)
	}
	if stats.Neutral != 1 {
		t.Errorf("neutral = %d, want 1", stats.Neutral)
	}
	if stats.Total != 4 {
		t.Errorf("total = %d, want 4", stats.Total)
	}
}

// TestFeedbackPersistence 验证新增的持久化语义：新增反馈后重启 Store，
// 历史反馈仍可经懒加载读出。
func TestFeedbackPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feedback.jsonl")
	ctx := context.Background()

	fs1 := NewFeedbackStore(path)
	fs1.Add(ctx, "s1", 1, FeedbackPositive, "first", "m1")
	fs1.Add(ctx, "s1", 2, FeedbackNegative, "second", "m2")

	// 验证文件已写入
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("feedback file not written: %v", err)
	}

	// 新建 Store（模拟进程重启）
	fs2 := NewFeedbackStore(path)
	stats := fs2.Stats(ctx)
	if stats.Total != 2 {
		t.Errorf("after reload: total = %d, want 2", stats.Total)
	}
	if stats.Positive != 1 || stats.Negative != 1 {
		t.Errorf("after reload: positive=%d negative=%d, want 1/1", stats.Positive, stats.Negative)
	}
}

func TestFeedbackTruncate(t *testing.T) {
	long := string(make([]rune, 300))
	for i := range long {
		long = long[:i] + "a" + long[i+1:]
	}
	result := truncateMessage(long, 200)
	if len([]rune(result)) != 201 { // 200 + …
		t.Errorf("truncated length = %d, want 201", len([]rune(result)))
	}
}
