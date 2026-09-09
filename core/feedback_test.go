package core

import (
	"context"
	"testing"
)

func TestFeedbackAdd(t *testing.T) {
	fs := NewFeedbackStore("/tmp/test-feedback.jsonl")
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
	fs := NewFeedbackStore("/tmp/test-feedback2.jsonl")
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
	fs := NewFeedbackStore("/tmp/test-feedback3.jsonl")
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
