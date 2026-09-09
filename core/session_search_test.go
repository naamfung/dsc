package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionSearchBasic(t *testing.T) {
	dir := t.TempDir()
	// 创建测试会话文件
	os.WriteFile(filepath.Join(dir, "session-1.jsonl"), []byte(
		`{"seq":1,"type":"user/message","data":{"content":"帮我实现用户登录接口"}}
{"seq":2,"type":"assistant/message","data":{"content":"好的，我来实现用户登录"}}
`), 0644)
	os.WriteFile(filepath.Join(dir, "session-2.jsonl"), []byte(
		`{"seq":1,"type":"user/message","data":{"content":"帮我写一份周报"}}
`), 0644)

	searcher := NewSessionSearcher(dir)
	ctx := context.Background()

	results, err := searcher.Search(ctx, "用户登录", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].SessionID != "session-1" {
		t.Errorf("session = %q, want session-1", results[0].SessionID)
	}
	if results[0].Score < 1 {
		t.Errorf("score should be >= 1, got %d", results[0].Score)
	}
}

func TestSessionSearchMultipleKeywords(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(
		`{"seq":1,"type":"user/message","data":{"content":"实现用户登录和注册接口"}}
`), 0644)
	os.WriteFile(filepath.Join(dir, "s2.jsonl"), []byte(
		`{"seq":1,"type":"user/message","data":{"content":"用户登录"}}
`), 0644)

	searcher := NewSessionSearcher(dir)
	ctx := context.Background()

	// "用户 登录" → s1 命中两个词，s2 命中两个词，但 s1 内容更长
	results, err := searcher.Search(ctx, "用户 登录", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSessionSearchEmptyQuery(t *testing.T) {
	dir := t.TempDir()
	searcher := NewSessionSearcher(dir)
	ctx := context.Background()

	results, err := searcher.Search(ctx, "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if results != nil {
		t.Errorf("empty query should return nil, got %d results", len(results))
	}
}

func TestSessionSearchNoResults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(
		`{"seq":1,"type":"user/message","data":{"content":"hello world"}}
`), 0644)

	searcher := NewSessionSearcher(dir)
	ctx := context.Background()

	results, err := searcher.Search(ctx, "不存在的关键词", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSessionSearchRebuild(t *testing.T) {
	dir := t.TempDir()
	searcher := NewSessionSearcher(dir)

	// 初始无文件
	if err := searcher.Rebuild(); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// 添加文件后重建
	os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(
		`{"seq":1,"type":"user/message","data":{"content":"测试重建索引"}}
`), 0644)

	if err := searcher.Rebuild(); err != nil {
		t.Fatalf("Rebuild 2: %v", err)
	}

	results, err := searcher.Search(context.Background(), "测试", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after rebuild, got %d", len(results))
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []SessionSearchResult{
		{SessionID: "s1", Snippet: "用户登录", Score: 3, LastModified: 1700000000000},
	}
	formatted := FormatSearchResults(results)
	if formatted == "" {
		t.Error("formatted should not be empty")
	}
	if !containsStr(formatted, "s1") {
		t.Error("should contain session id")
	}
	if !containsStr(formatted, "用户登录") {
		t.Error("should contain snippet")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && indexOfStr(s, sub) >= 0
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
