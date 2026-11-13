package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
)

// 本文件守「摘要缓存落盘」这条线的语义：
//
//	dsc-system 的 pre-step 改写只作用于**本次请求**，会话日志与真实历史一字不改；
//	落盘的只是它自己的 <session>.dsc-system.json（摘要 + 覆盖范围 + 审计字段）。
//
// 因此丢文件/改坏文件/换 schema 的后果一律是「下次重新摘要一次」，绝不是「历史变了」。
// 这与 DSH 把压缩写成会话 surface 事件（遮蔽历史、不可逆）是两件事——也正是加这层
// 缓存时唯一要守住的边界。

// newPersistedServer 构造带落盘通道的驻留（目录由测试给，绝不写进真实 sessions/）。
func newPersistedServer(t *testing.T, window int, dir string) *compactionBasicServer {
	t.Helper()
	s := newTestCompactionBasicServer(t, window)
	s.store = newCompactionStore(dir)
	return s
}

func stateFile(t *testing.T, dir, sess string) string {
	t.Helper()
	return filepath.Join(dir, sess+".dsc-system.json")
}

// TestSummariesSurviveRestart 摘要落盘后，重启（新进程、新驻留实例）不再为同一段前缀
// 重新调用 LLM：指纹吻合即复用。
//
// 这是加落盘的全部理由——不落盘时，插件进程重启后的第一次请求会把同一段历史再摘要一遍。
func TestSummariesSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	msgs := bigMsgs(16, 1200) // 约 19264 token，窗口 10000（阈值 8000）

	first := newPersistedServer(t, 10000, dir)
	llm1 := &fakeCompactionBasicLLM{content: "SUMMARY-FROM-LLM"}
	first.llm = llm1
	res := mustPreStepSession(t, first, "sess-restart", msgs)
	if res == "" {
		t.Fatal("超阈值应改写")
	}
	// 块数由「剩余预算 × 吞吐估计」决定（慢引擎多块、快引擎一块），这里只要求真的压过。
	if llm1.calls < 1 {
		t.Fatalf("首次压缩应至少调用一次 LLM，got %d", llm1.calls)
	}
	if _, err := os.Stat(stateFile(t, dir, "sess-restart")); err != nil {
		t.Fatalf("应写出状态文件: %v", err)
	}

	// 新实例（模拟进程重启）：内存 map 为空，只有磁盘那一份
	second := newPersistedServer(t, 10000, dir)
	llm2 := &fakeCompactionBasicLLM{content: "SUMMARY-SHOULD-NOT-BE-USED"}
	second.llm = llm2
	res2 := mustPreStepSession(t, second, "sess-restart", msgs)
	got := parseRewrite(t, res2)
	if !strings.Contains(got[0].GetContent(), "SUMMARY-FROM-LLM") {
		t.Fatalf("重启后应复用磁盘摘要，got %q", got[0].GetContent())
	}
	if llm2.calls != 0 {
		t.Fatalf("重启后不得重新摘要（LLM calls = %d）", llm2.calls)
	}
}

// TestRestartStateCarriesAuditFields 落盘记录能回答「什么时候、用哪个模型、压了多少、
// 是模型写的还是截断的」——对齐 DSH compaction/summary 记 provider/model/usage 的意图。
func TestRestartStateCarriesAuditFields(t *testing.T) {
	dir := t.TempDir()
	msgs := bigMsgs(16, 1200)
	s := newPersistedServer(t, 10000, dir)
	s.llm = &fakeCompactionBasicLLM{content: "AUDITED"}
	mustPreStepSession(t, s, "sess-audit", msgs)

	raw, err := os.ReadFile(stateFile(t, dir, "sess-audit"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var f compactionStateFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	if f.Schema != compactionStateSchema || f.Session != "sess-audit" {
		t.Fatalf("文件头不符: %+v", f)
	}
	if len(f.Records) < 1 {
		t.Fatalf("应有压缩记录，got %d", len(f.Records))
	}
	r := f.Records[0]
	if r.Source != "llm" || r.Covered <= 0 || r.CreatedMS <= 0 || r.HeadHash == "" {
		t.Fatalf("审计字段缺失: %+v", r)
	}
}

// TestTruncatedSummariesAreMarkedAsSuch LLM 不可用时的退化摘要必须标注来源：
// 事后要能区分「模型写的摘要」与「一段截断」，否则无从解释某段历史为何被压得这么狠。
func TestTruncatedSummariesAreMarkedAsSuch(t *testing.T) {
	dir := t.TempDir()
	s := newPersistedServer(t, 10000, dir)
	// 不给 llm：走截断式退化
	mustPreStepSession(t, s, "sess-trunc", bigMsgs(16, 1200))

	raw, err := os.ReadFile(stateFile(t, dir, "sess-trunc"))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var f compactionStateFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	if len(f.Records) != 1 || f.Records[0].Source != "truncated" {
		t.Fatalf("退化摘要应标注 source=truncated，got %+v", f.Records)
	}
}

// TestChangedPrefixDiscardsPersistedState 历史被改写（前缀指纹失配）时，磁盘记录必须
// 作废并清掉，然后重新摘要——复用一份已经对不上的摘要是最坏的结果（模型看到的世界
// 与真实历史脱节）。
func TestChangedPrefixDiscardsPersistedState(t *testing.T) {
	dir := t.TempDir()
	msgs := bigMsgs(16, 1200)

	first := newPersistedServer(t, 10000, dir)
	first.llm = &fakeCompactionBasicLLM{content: "OLD"}
	mustPreStepSession(t, first, "sess-changed", msgs)

	// 改写被压前缀中的一条（保留区不动）
	tampered := append([]*proto.Message(nil), msgs...)
	tampered[0] = &proto.Message{Role: tampered[0].GetRole(), Content: "完全不同的开头"}

	second := newPersistedServer(t, 10000, dir)
	llm2 := &fakeCompactionBasicLLM{content: "NEW"}
	second.llm = llm2
	res := mustPreStepSession(t, second, "sess-changed", tampered)
	if llm2.calls < 1 {
		t.Fatalf("前缀失配后必须重新摘要（LLM calls = %d）", llm2.calls)
	}
	if got := parseRewrite(t, res); !strings.Contains(got[0].GetContent(), "NEW") {
		t.Fatalf("应采用新摘要，got %q", got[0].GetContent())
	}
}

// TestSchemaMismatchDiscardsState 格式版本不符即整体丢弃（AGENTS.md §4：不写逐字段迁移）。
// 表现：重新摘要一次，而不是把语义可能已变的数据读进来。
func TestSchemaMismatchDiscardsState(t *testing.T) {
	dir := t.TempDir()
	msgs := bigMsgs(16, 1200)

	first := newPersistedServer(t, 10000, dir)
	first.llm = &fakeCompactionBasicLLM{content: "V1"}
	mustPreStepSession(t, first, "sess-schema", msgs)

	// 把 schema 改成未来版本
	p := stateFile(t, dir, "sess-schema")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var f compactionStateFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("parse: %v", err)
	}
	f.Schema = compactionStateSchema + 1
	out, err := json.Marshal(&f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := newCompactionStore(dir).Load("sess-schema"); got != nil {
		t.Fatalf("schema 不符必须丢弃，got %d records", len(got))
	}

	second := newPersistedServer(t, 10000, dir)
	llm2 := &fakeCompactionBasicLLM{content: "V2"}
	second.llm = llm2
	mustPreStepSession(t, second, "sess-schema", msgs)
	if llm2.calls < 1 {
		t.Fatalf("丢弃后应重新摘要，LLM calls = %d", llm2.calls)
	}
}

// TestUnsafeSessionIDIsNotPersisted 会话 id 不可安全用于文件名时不落盘（只留内存）：
// 宁可重启后重压一次，也不要在 sessions/ 之外造文件。
func TestUnsafeSessionIDIsNotPersisted(t *testing.T) {
	dir := t.TempDir()
	s := newPersistedServer(t, 10000, dir)
	s.llm = &fakeCompactionBasicLLM{content: "NO-FILE"}
	if res := mustPreStepSession(t, s, "../evil", bigMsgs(16, 1200)); res == "" {
		t.Fatal("内存路径仍应正常改写")
	}
	if entries, err := os.ReadDir(filepath.Dir(dir)); err == nil {
		for _, e := range entries {
			if strings.Contains(e.Name(), "evil") {
				t.Fatalf("不得在 sessions/ 之外造文件: %s", e.Name())
			}
		}
	}
	if _, err := os.Stat(stateFile(t, dir, "../evil")); err == nil {
		t.Fatal("非法会话 id 不应落盘")
	}
}

// TestNilStoreIsPureMemory 未装配 store 时行为与从前一致（纯内存）：测试与未接线部署
// 都不该偷偷写盘。
func TestNilStoreIsPureMemory(t *testing.T) {
	s := newTestCompactionBasicServer(t, 10000)
	if s.store != nil {
		t.Fatal("newCompactionBasicServer 不应自带 store（装配在 main）")
	}
	s.llm = &fakeCompactionBasicLLM{content: "MEM-ONLY"}
	if res := mustPreStepSession(t, s, "sess-mem", bigMsgs(16, 1200)); res == "" {
		t.Fatal("纯内存模式下仍应改写")
	}
	// 同实例重放：内存记录命中，不再调 LLM
	second := &fakeCompactionBasicLLM{content: "X"}
	s.llm = second
	mustPreStepSession(t, s, "sess-mem", bigMsgs(16, 1200))
	if second.calls != 0 {
		t.Fatalf("内存复用不应再调 LLM，calls=%d", second.calls)
	}
}

// TestStoreDropsRecordWithoutFooter 结构残缺的记录在读回时被丢弃：留着它会让
// validRecords 误判整串有效（UpTo 对得上、摘要却是空的）。
func TestStoreDropsRecordWithoutFooter(t *testing.T) {
	dir := t.TempDir()
	f := compactionStateFile{
		Schema:  compactionStateSchema,
		Session: "sess-bad",
		Records: []compactionRecord{
			{UpTo: 5, HeadHash: "h", Summary: ""},    // 缺摘要
			{UpTo: 5, HeadHash: "", Summary: "s"},    // 缺指纹
			{UpTo: 0, HeadHash: "h", Summary: "s"},   // 缺范围
			{UpTo: 9, HeadHash: "h2", Summary: "ok"}, // 唯一有效
		},
	}
	raw, _ := json.Marshal(&f)
	if err := os.WriteFile(filepath.Join(dir, "sess-bad.dsc-system.json"), raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := newCompactionStore(dir).Load("sess-bad")
	if len(got) != 1 || got[0].Summary != "ok" {
		t.Fatalf("应只保留 1 条有效记录，got %+v", got)
	}
}
