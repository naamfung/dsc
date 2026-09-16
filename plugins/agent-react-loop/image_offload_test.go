package main

import (
	"os"
	"strings"
	"testing"

	"dsc/proto"
)

// imgRef 生成测试用图像引用（形态合法即可，卸载决策不触达存储）。
func imgRef(n int) string {
	return "dsc-shot://" + strings.Repeat("a", 8) + string(rune('0'+n%10))
}

// TestOffloadRequestImagesBelowBudget 预算内（含边界恰好等于上限）原样返回，
// 零拷贝零改写。
func TestOffloadRequestImagesBelowBudget(t *testing.T) {
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{imgRef(1)}},
		{Role: "tool", Content: "t1", Images: []string{imgRef(2), imgRef(3)}},
		{Role: "tool", Content: "t2"},
	}
	got := offloadRequestImages(msgs, 3)
	if len(got) != len(msgs) {
		t.Fatalf("length changed: %d != %d", len(got), len(msgs))
	}
	for i := range msgs {
		if got[i] != msgs[i] {
			t.Fatalf("message %d was copied despite being within budget", i)
		}
	}
}

// TestOffloadRequestImagesOldestFirst 超预算时最旧优先退役：退役的引用从
// Images 移除并在该消息 Content 尾部追加稳定占位文本；最新图像保持在场。
func TestOffloadRequestImagesOldestFirst(t *testing.T) {
	r1, r2, r3, r4 := imgRef(1), imgRef(2), imgRef(3), imgRef(4)
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{r1}},
		{Role: "tool", Content: "t1", Images: []string{r2, r3}},
		{Role: "tool", Content: "t2", Images: []string{r4}},
	}
	got := offloadRequestImages(msgs, 2)
	if len(got) != len(msgs) {
		t.Fatalf("length changed: %d != %d", len(got), len(msgs))
	}
	// 最旧两张（r1、r2）退役：user 消息图像清空 + 占位；t1 只留 r3
	if len(got[0].Images) != 0 || !strings.Contains(got[0].Content, "[image omitted to fit request image limits; "+r1+"]") {
		t.Fatalf("oldest image not offloaded: %+v", got[0])
	}
	if len(got[1].Images) != 1 || got[1].Images[0] != r3 {
		t.Fatalf("second oldest not offloaded: %+v", got[1].Images)
	}
	if !strings.Contains(got[1].Content, "[image omitted to fit request image limits; "+r2+"]") {
		t.Fatalf("placeholder missing on partially offloaded message: %q", got[1].Content)
	}
	if len(got[2].Images) != 1 || got[2].Images[0] != r4 {
		t.Fatalf("newest image must stay: %+v", got[2].Images)
	}
}

// TestOffloadRequestImagesTransient 瞬态投影不改传入消息（会话存储不受影响）：
// 消息本体浅拷贝，原消息的 Content/Images 保持原样。
func TestOffloadRequestImagesTransient(t *testing.T) {
	r1, r2 := imgRef(1), imgRef(2)
	msgs := []*proto.Message{
		{Role: "user", Content: "u", Images: []string{r1}},
		{Role: "tool", Content: "t1", Images: []string{r2}},
	}
	_ = offloadRequestImages(msgs, 1)
	if len(msgs[0].Images) != 1 || msgs[0].Images[0] != r1 || msgs[0].Content != "u" {
		t.Fatalf("durable message mutated: %+v", msgs[0])
	}
	if len(msgs[1].Images) != 1 || msgs[1].Content != "t1" {
		t.Fatalf("durable message mutated: %+v", msgs[1])
	}
}

// TestOffloadRequestImagesUnlimited "0" 表示不限制：任何数量原样返回。
func TestOffloadRequestImagesUnlimited(t *testing.T) {
	msgs := []*proto.Message{{Role: "tool", Content: "t", Images: []string{imgRef(1), imgRef(2), imgRef(3)}}}
	if got := offloadRequestImages(msgs, 0); got[0] != msgs[0] {
		t.Fatal("maxImages<=0 must be a no-op (unlimited)")
	}
}

// TestMaxRequestImages 上限配置：env 覆盖、"0"=不限制、非法值回退默认。
func TestMaxRequestImages(t *testing.T) {
	t.Setenv(envMaxRequestImages, "5")
	if got := maxRequestImages(); got != 5 {
		t.Fatalf("maxRequestImages = %d, want 5", got)
	}
	t.Setenv(envMaxRequestImages, "0")
	if got := maxRequestImages(); got != 0 {
		t.Fatalf("maxRequestImages(0) = %d, want 0（不限制）", got)
	}
	t.Setenv(envMaxRequestImages, "bogus")
	if got := maxRequestImages(); got != defaultMaxRequestImages {
		t.Fatalf("非法值应回退默认，got %d", got)
	}
	os.Unsetenv(envMaxRequestImages)
	if got := maxRequestImages(); got != defaultMaxRequestImages {
		t.Fatalf("未设置应取默认，got %d", got)
	}
}
