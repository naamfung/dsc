package main

import (
	"context"
	"os"
	"testing"

	bcadapter "dsc-billion-context/adapter"
	billion "dsc-billion-context/billion"

	dsc "dsc-sdk"
)

// TestEmptyPreStepPayloadDoesNothing 轻载荷（宿主只发给岗位持有者完整消息列表）必须
// 什么都不做：不建 state、不覆盖 lastMessages。
//
// 非岗位时每步都会收到这种信封。若照样 Load/写 state，等于让一个不在值班的插件不
// 停改写自己的状态；而 lastMessages 是 compress 工具的材料缓存，清空会让它失明。
func TestEmptyPreStepPayloadDoesNothing(t *testing.T) {
	dir := t.TempDir()
	bc := &BillionContext{
		store:  bcadapter.NewStateStore(dir),
		config: billion.DefaultConfig(1_000_000),
	}

	out, err := bc.handlePreStep(context.Background(), `{"session":"s-empty","token_count":900000,"projected_tokens":900000}`)
	if err != nil {
		t.Fatalf("空载荷不应报错: %v", err)
	}
	if out != "" {
		t.Fatalf("空载荷不应产出改写，got %.80s", out)
	}
	if _, err := os.Stat(dsc.PJoin(dir, "s-empty"+bcadapter.StateFileSuffix)); err == nil {
		t.Fatal("空载荷不应创建 state 文件")
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()
	if len(bc.lastMessages) != 0 {
		t.Fatalf("不应缓存消息，got %d 条", len(bc.lastMessages))
	}
}
