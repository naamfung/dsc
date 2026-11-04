package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bcadapter "dsc-billion-context/adapter"
	billion "dsc-billion-context/billion"
	dsc "dsc-sdk"
)

// TestGetSessionIDPrefersEventSession 会话 ID 优先级回归。
//
// 背景：此前 getSessionID 只能回落到「工作区派生键」，同一工作区开多个会话/fork
// 会共用一份 state（refs/blocks/nudge 基线互相污染）。宿主在 pre-step 事件里本来
// 就带真实会话 ID，必须优先采用；未收到事件时回落到 SDK 的项目级键（唯一实现）。
func TestGetSessionIDPrefersEventSession(t *testing.T) {
	bc := &BillionContext{}
	if got := bc.getSessionID(); got != dsc.ProjectSessionKey() {
		t.Fatalf("未收到 pre-step 时应回退项目级键 %q，got %q", dsc.ProjectSessionKey(), got)
	}

	bc.setSessionID("sess-from-event")
	if got := bc.getSessionID(); got != "sess-from-event" {
		t.Fatalf("pre-step 事件里的会话应优先，got %q", got)
	}

	// 空值不得覆盖已有会话（避免偶发空 Session 把状态键打回工作区派生值）
	bc.setSessionID("")
	if got := bc.getSessionID(); got != "sess-from-event" {
		t.Fatalf("空会话不应覆盖已有值，got %q", got)
	}
}

// TestHandlePreStepAdoptsEventSession 端到端：pre-step 载荷的 session 成为 state 键，
// 即不同会话各自落一个 state 文件（会话隔离）。
func TestHandlePreStepAdoptsEventSession(t *testing.T) {
	dir := t.TempDir()
	bc := &BillionContext{
		store:  bcadapter.NewStateStore(dir),
		config: billion.DefaultConfig(200000),
	}

	payload := `{"session":"sess-A","messages_json":"[{\"role\":\"user\",\"content\":\"hi\"}]","token_count":10}`
	if _, err := bc.handlePreStep(context.Background(), payload); err != nil {
		t.Fatalf("pre-step 失败: %v", err)
	}
	if got := bc.getSessionID(); got != "sess-A" {
		t.Fatalf("应采用事件会话，got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "sess-A"+bcadapter.StateFileSuffix)); err != nil {
		t.Fatalf("state 应写入 sess-A.json（会话隔离）: %v", err)
	}

	// 切到另一会话：写入新文件，旧文件保留
	payload = `{"session":"sess-B","messages_json":"[{\"role\":\"user\",\"content\":\"hi\"}]","token_count":10}`
	if _, err := bc.handlePreStep(context.Background(), payload); err != nil {
		t.Fatalf("pre-step(B) 失败: %v", err)
	}
	for _, name := range []string{"sess-A" + bcadapter.StateFileSuffix, "sess-B" + bcadapter.StateFileSuffix} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("应保留 %s: %v", name, err)
		}
	}
}

// TestCCREndToEnd 走一遍真实链路：卸载 → 落盘（附件库）→ 回取。
//
// 覆盖的正是纯逻辑测不到的部分：sha 与 dsc-txt:// 引用是否对得上、超限是否真的
// 导出、导出的文件是否可读。附件库用 DSC_ATTACHMENT_DIR 指到临时目录。
func TestCCREndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSC_ATTACHMENT_DIR", filepath.Join(dir, "attachments"))
	// 工作区根在进程 init 时冻结，t.Setenv 改不动——要用 SDK 的显式设值入口
	oldRoot := dsc.WorkspaceRoot()
	dsc.SetWorkspaceRoot(dir)
	t.Cleanup(func() { dsc.SetWorkspaceRoot(oldRoot) })

	bc := &BillionContext{config: billion.DefaultConfig(200_000)}
	bc.config.CCR.Enabled = true
	bc.config.CCR.MinToolTokens = 100
	bc.config.CCR.InlineLimitTokens = 200

	body := strings.Repeat("build output line\n", 400)
	msg := billion.CoreMessage{
		ID: "t1", Role: billion.RoleTool, ToolName: "shell", Text: body,
		ContentType: billion.ContentTypeToolResult, ToolCallID: "call-t1",
	}
	state := billion.CreateInitialState()
	out, offloads := billion.ApplyCCR([]billion.CoreMessage{msg}, state, bc.config)
	if len(offloads) != 1 {
		t.Fatalf("应产生 1 条卸载，实际 %d", len(offloads))
	}
	// 落盘：正文进附件库
	persisted := bc.persistCCROffloads(out, offloads)
	if billion.IsCCRPlaceholder(persisted[0].Text) == false {
		t.Fatalf("落盘成功后应保留占位符: %q", persisted[0].Text[:40])
	}
	// 附件库里的文件应存在且内容一致
	ref := dsc.TextRefPrefix + offloads[0].Entry.Shasum
	if got, err := dsc.ResolveTextRef(ref); err != nil || got != body {
		t.Fatalf("附件库应存有完整原文: err=%v len=%d", err, len(got))
	}

	// 回取：超内联上限 → 导出成文件并给指针
	bc.state = state
	args, _ := json.Marshal(map[string]string{"ref": offloads[0].Entry.Ref})
	got, err := bc.handleRetrieve(context.Background(), args)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if !strings.Contains(got, "billion-retrieved/") {
		t.Fatalf("超限时应返回导出指针: %s", got)
	}
	exportPath := filepath.Join(dir, "billion-retrieved", offloads[0].Entry.Shasum+".txt")
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("导出文件应存在: %v", err)
	}
	if string(data) != body {
		t.Fatalf("导出内容应与原文一致: len=%d", len(data))
	}

	// 幻觉 ref：给提示而不是报错
	badArgs, _ := json.Marshal(map[string]string{"ref": "c99999"})
	if msg, err := bc.handleRetrieve(context.Background(), badArgs); err != nil || !strings.Contains(msg, "c99999") {
		t.Fatalf("未知 ref 应给提示: %q err=%v", msg, err)
	}
}

// TestCCRPersistFailureRestoresBody 附件库写不进去时必须回滚成原文，
// 不能留一个取不回来的占位符。
func TestCCRPersistFailureRestoresBody(t *testing.T) {
	dir := t.TempDir()
	// 指向一个"文件"——无法当目录用，SaveTextAttachment 必然失败
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DSC_ATTACHMENT_DIR", blocker)

	bc := &BillionContext{config: billion.DefaultConfig(200_000)}
	body := strings.Repeat("z", 4000)
	msg := billion.CoreMessage{
		ID: "t1", Role: billion.RoleTool, ToolName: "shell", Text: body,
		ContentType: billion.ContentTypeToolResult, ToolCallID: "call-t1",
	}
	state := billion.CreateInitialState()
	bc.config.CCR.Enabled = true
	bc.config.CCR.MinToolTokens = 100
	out, offloads := billion.ApplyCCR([]billion.CoreMessage{msg}, state, bc.config)
	if len(offloads) != 1 {
		t.Fatal("应产生卸载")
	}
	persisted := bc.persistCCROffloads(out, offloads)
	if persisted[0].Text != body {
		t.Fatalf("落盘失败应回滚原文，实际 %q", persisted[0].Text[:40])
	}
}

// TestRunCommandRendersStatus 命令由插件自己渲染：宿主只转发、只展示，
// 不认识这些指标。没有状态时也要给一句人话，而不是空输出。
func TestRunCommandRendersStatus(t *testing.T) {
	// 窗口取 DSC 的真实 1M 口径（1024 底）：1048576 必须显示成 1.0M，
	// 而不是按 1000 折算出来的 1.05M。
	bc := &BillionContext{config: billion.DefaultConfig(1_048_576)}

	out, err := bc.runCommand(context.Background(), "context", "")
	if err != nil {
		t.Fatalf("runCommand: %v", err)
	}
	if !strings.Contains(out, "尚未建立") {
		t.Fatalf("无状态时应说明原因，实际：%q", out)
	}

	state := billion.CreateInitialState()
	state.Blocks = []billion.CompressionBlock{
		{BlockID: "b0", Tier: billion.Tier1, Active: true, CompressedTokens: 40_000},
		{BlockID: "b1", Tier: billion.Tier2, Active: true, CompressedTokens: 70_000},
	}
	state.Stats.TokensCompressed = 110_000
	state.Stats.CompressionCount = 2
	state.Stats.CrushCount = 3
	state.Stats.CrushTokensSaved = 9_500
	state.Stats.CCRStored = 4
	state.Stats.CCRStoredTokens = 31_000
	state.Stats.CCRRetrieved = 2
	state.Stats.CCRExported = 1
	bc.state = state
	bc.lastTokenCount = 430_080 // 420K（1024 底）

	out, err = bc.runCommand(context.Background(), "context", "")
	if err != nil {
		t.Fatalf("runCommand(billion): %v", err)
	}
	for _, want := range []string{"420K / 1.0M", "41%", "T1=1 T2=1 T3=0", "107K", "确定性压缩: 3 次",
		"已卸载可回取: 4 条", "回取: 2 次（导出 1 · 落空 0）"} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出应包含 %q，实际：\n%s", want, out)
		}
	}
	// 1000 进制会把 1048576 写成 1.05M，与 DSC 其它地方（1024 底）不一致
	if strings.Contains(out, "1.05M") {
		t.Fatalf("容量应按 1024 底显示（1.0M），不该出现 1.05M：\n%s", out)
	}

	// 未声明的名字：报错（宿主的转发只应出现在声明清单里）。
	// 顺带锚定「一个命令一个名字」：别名会让补全菜单出现两条同义项。
	if _, err := bc.runCommand(context.Background(), "nope", ""); err == nil {
		t.Fatal("未知命令应返回错误")
	}
	if _, err := bc.runCommand(context.Background(), "billion", ""); err == nil {
		t.Fatal("已移除的别名不应再被当作有效命令")
	}
}
