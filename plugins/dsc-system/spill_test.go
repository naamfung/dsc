package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dsc/proto"
)

// longText 生成 n 个字符的确定性长文本。
func longText(n int) string {
	block := strings.Repeat("0123456789", 100) // 1000 字符
	return strings.Repeat(block, n/1000+1)[:n]
}

// newSpillTestServer 建立以 t.TempDir() 为外置根的策略服务（显式覆盖 DSC_SPILL_DIR，
// 不依赖进程工作目录）。
func newSpillTestServer(t *testing.T) *spillServer {
	t.Helper()
	t.Setenv(spillEnvDir, t.TempDir())
	return newSpillServer()
}

// postEvent 构造 post-execute 事件。
func postEvent(tool, argsJSON, result, toolErr, session string) *proto.PolicyEvent {
	return &proto.PolicyEvent{
		Kind:          kindPostExecute,
		Tool:          tool,
		ArgumentsJson: argsJSON,
		Result:        result,
		Error:         toolErr,
		Session:       session,
	}
}

// TestThresholdCharsByEnv 验证阈值解析：缺省 4000、env 覆盖、显式 0 禁用、非法回退。
func TestThresholdCharsByEnv(t *testing.T) {
	t.Setenv(spillEnvThreshold, "")
	if got := thresholdChars(); got != spillDefaultThreshold {
		t.Fatalf("default = %d, want %d", got, spillDefaultThreshold)
	}
	t.Setenv(spillEnvThreshold, "1234")
	if got := thresholdChars(); got != 1234 {
		t.Fatalf("override = %d, want 1234", got)
	}
	t.Setenv(spillEnvThreshold, "0")
	if got := thresholdChars(); got != 0 {
		t.Fatalf("explicit 0 = %d, want 0 (禁用)", got)
	}
	t.Setenv(spillEnvThreshold, "-5")
	if got := thresholdChars(); got != spillDefaultThreshold {
		t.Fatalf("invalid = %d, want default", got)
	}
	t.Setenv(spillEnvThreshold, "abc")
	if got := thresholdChars(); got != spillDefaultThreshold {
		t.Fatalf("non-numeric = %d, want default", got)
	}
}

// TestSpillDisabledByZero 验证 env 显式 0 禁用：超长结果也不外置（no-op）。
func TestSpillDisabledByZero(t *testing.T) {
	s := newSpillTestServer(t)
	t.Setenv(spillEnvThreshold, "0")
	dec, err := s.OnEvent(context.Background(), postEvent("shell", `{}`, longText(9000), "", "s1"))
	if err != nil || dec.GetAction() != "" {
		t.Fatalf("0 应禁用外置: dec=%+v err=%v", dec, err)
	}
}

// TestOnEventSpillsOversizedResult 验证核心语义：超长结果 → replace 裁决，
// 全文落盘、定位符即文件路径、预览含头尾且不含全文。
func TestOnEventSpillsOversizedResult(t *testing.T) {
	s := newSpillTestServer(t)
	content := longText(9000)
	dec, err := s.OnEvent(context.Background(), postEvent("shell", `{"command":"cat big.log"}`, content, "", "s1"))
	if err != nil {
		t.Fatalf("OnEvent: %v", err)
	}
	if dec.GetAction() != actionReplace || dec.GetResult() == "" {
		t.Fatalf("超长结果应 replace: action=%q result len=%d", dec.GetAction(), len(dec.GetResult()))
	}
	replaced := dec.GetResult()
	// 头尾预览在场，全文不再内联
	if !strings.Contains(replaced, "[内容已外置: ") {
		t.Fatalf("替换体应含外置告示头: %.80s", replaced)
	}
	if strings.Contains(replaced, content) {
		t.Fatal("完整内容不应残留于替换体")
	}
	// 结构（无换行测试内容）：第一行告示头，第二行原文头部，末行原文尾部
	lines := strings.Split(replaced, "\n")
	if !strings.HasPrefix(lines[1], content[:100]) {
		t.Fatalf("预览头部应保留原文开头: %.80s", lines[1])
	}
	if !strings.HasSuffix(lines[len(lines)-1], content[len(content)-100:]) {
		t.Fatalf("预览尾部应保留原文结尾: ...%.80s", lines[len(lines)-1])
	}
	if !strings.Contains(replaced, "view 命令") || !strings.Contains(replaced, "view_range") {
		t.Fatal("告示应指引 view 命令取回（可配 view_range 分段）")
	}
	// 定位符是真实存在的文件路径，内容与原文逐字一致（跨层保真）
	start := strings.Index(replaced, "[内容已外置: ") + len("[内容已外置: ")
	end := strings.Index(replaced[start:], "]") + start
	locator := replaced[start:end]
	if !filepath.IsAbs(locator) {
		t.Fatalf("定位符应为绝对路径: %q", locator)
	}
	got, err := os.ReadFile(locator)
	if err != nil {
		t.Fatalf("外置文件不可读: %v", err)
	}
	if string(got) != content {
		t.Fatalf("外置内容与原文不一致: len %d vs %d", len(got), len(content))
	}
}

// TestOnEventKeepsShortResult 未达阈值放行（空裁决）。
func TestOnEventKeepsShortResult(t *testing.T) {
	s := newSpillTestServer(t)
	dec, err := s.OnEvent(context.Background(), postEvent("shell", `{}`, "short", "", "s1"))
	if err != nil || dec.GetAction() != "" || dec.GetResult() != "" {
		t.Fatalf("短结果应放行: dec=%+v err=%v", dec, err)
	}
}

// TestOnEventExemptsViewCommand 取回路径豁免（对齐 DSH 豁免 read）：编辑器
// view 命令的超长结果不外置——否则「取回外置内容 → 又被外置」死循环。
func TestOnEventExemptsViewCommand(t *testing.T) {
	s := newSpillTestServer(t)
	content := longText(9000)
	dec, err := s.OnEvent(context.Background(), postEvent("str_replace_editor", `{"command":"view","path":"/workspace/big.txt"}`, content, "", "s1"))
	if err != nil || dec.GetAction() != "" {
		t.Fatalf("view 命令应豁免: dec=%+v err=%v", dec, err)
	}
	// 写命令（结果远小于阈值）不豁免亦不触发——路径解析失败也不影响
	if dec, err = s.OnEvent(context.Background(), postEvent("str_replace_editor", `{"command":"str_replace"}`, content, "", "s1")); err != nil || dec.GetAction() != actionReplace {
		t.Fatalf("编辑器写命令不受豁免（超长时照常外置）: dec=%+v err=%v", dec, err)
	}
}

// TestOnEventSkipsFailedResults 失败结果不外置（错误是权威观察，外置只塑造
// 被接受的成功结果——对齐 DSH block pass-through）。
func TestOnEventSkipsFailedResults(t *testing.T) {
	s := newSpillTestServer(t)
	dec, err := s.OnEvent(context.Background(), postEvent("shell", `{}`, longText(9000), "no such file", "s1"))
	if err != nil || dec.GetAction() != "" {
		t.Fatalf("失败结果应放行: dec=%+v err=%v", dec, err)
	}
}

// TestOnEventSkipsNonPostExecute 非本槽事件一律放行（策略只在自己的领域发声）。
func TestOnEventSkipsNonPostExecute(t *testing.T) {
	s := newSpillTestServer(t)
	for _, kind := range []string{"tool/pre-execute", "tool/execute"} {
		dec, err := s.OnEvent(context.Background(), &proto.PolicyEvent{Kind: kind, Tool: "shell", Result: longText(9000)})
		if err != nil || dec.GetAction() != "" {
			t.Fatalf("%s 应放行: dec=%+v err=%v", kind, dec, err)
		}
	}
}

// TestReplacementWithinThresholdInvariant 替换体永不超阈值（DSH 不变量：
// the policy NEVER emits a replacement larger than the cap）。
func TestReplacementWithinThresholdInvariant(t *testing.T) {
	for _, threshold := range []int{500, 1000, 4000, 20000} {
		content := longText(threshold * 3)
		replaced, ok := spillReplacement(content, "/tmp/spill/spill-7.txt", threshold)
		if !ok {
			t.Fatalf("threshold=%d: 应产生阈内替换", threshold)
		}
		if got := len([]rune(replaced)); got > threshold {
			t.Fatalf("threshold=%d: 替换体 %d 字符超阈值", threshold, got)
		}
		if !strings.Contains(replaced, "spill-7.txt") {
			t.Fatalf("threshold=%d: 替换体应含定位符", threshold)
		}
	}
}

// TestTinyThresholdKeepsInline 阈值小到告示单独就放不下：不产生阈内替换，
// 保留内联（外置文件成为无害孤儿——对齐 DSH no within-cap replacement）。
func TestTinyThresholdKeepsInline(t *testing.T) {
	s := newSpillTestServer(t)
	t.Setenv(spillEnvThreshold, "50")
	dec, err := s.OnEvent(context.Background(), postEvent("shell", `{}`, longText(9000), "", "s1"))
	if err != nil || dec.GetAction() != "" {
		t.Fatalf("阈内容不下替换体应保留内联: dec=%+v err=%v", dec, err)
	}
}

// TestPerSessionIsolation 默认根目录按会话属主隔离（per-session owner）：
// 不同会话的外置文件落在不同目录，定位符互不可见。
func TestPerSessionIsolation(t *testing.T) {
	t.Setenv(spillEnvDir, "")
	base := t.TempDir()
	s := newSpillServer()
	s.cwdFunc = func() (string, error) { return base, nil }
	loc1, err := s.mustSpill(t, "s1")
	if err != nil {
		t.Fatal(err)
	}
	loc2, err := s.mustSpill(t, "s2")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(loc1) == filepath.Dir(loc2) {
		t.Fatalf("不同会话应落在不同目录: %s vs %s", loc1, loc2)
	}
	if !strings.Contains(filepath.ToSlash(loc1), "/temp/spill/s1/") {
		t.Fatalf("会话目录约定不符: %s", loc1)
	}
}

// mustSpill 外置一次固定内容，返回定位符。
func (s *spillServer) mustSpill(t *testing.T, session string) (string, error) {
	t.Helper()
	dec, err := s.OnEvent(context.Background(), postEvent("shell", `{}`, longText(9000), "", session))
	if err != nil || dec.GetAction() != actionReplace {
		t.Fatalf("session=%s 应外置: dec=%+v err=%v", session, dec, err)
	}
	start := strings.Index(dec.GetResult(), "[内容已外置: ") + len("[内容已外置: ")
	return dec.GetResult()[start : strings.Index(dec.GetResult()[start:], "]")+start], nil
}

// TestNumberingContinuesAcrossRestart 编号续接：同根目录新开服务句柄（模拟
// 插件重启）不覆盖既有文件——定位符指向的内容不被篡改。
func TestNumberingContinuesAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(spillEnvDir, dir)
	first := newSpillServer()
	if _, err := first.mustSpill(t, "s1"); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "spill-*.txt"))
	if len(files) != 1 || !strings.HasSuffix(files[0], "spill-1.txt") {
		t.Fatalf("首次外置应产生 spill-1.txt: %v", files)
	}
	restarted := newSpillServer()
	if _, err := restarted.mustSpill(t, "s1"); err != nil {
		t.Fatal(err)
	}
	files, _ = filepath.Glob(filepath.Join(dir, "spill-*.txt"))
	if len(files) != 2 || !strings.HasSuffix(files[1], "spill-2.txt") {
		t.Fatalf("重启后应续接 spill-2.txt 而非覆盖: %v", files)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "spill-1.txt")); err != nil || len(got) != 9000 {
		t.Fatalf("既有文件内容不应被篡改: err=%v len=%d", err, len(got))
	}
}

// TestResolveRootDirOverride 显式覆盖：DSC_SPILL_DIR 为精确目录、不分会话
// （对齐旧宿主约定）；默认根为 <工作目录>/temp/spill/<session>。
func TestResolveRootDirOverride(t *testing.T) {
	s := newSpillServer()
	s.cwdFunc = func() (string, error) { return "/base", nil }
	t.Setenv(spillEnvDir, "/custom/spill")
	if got := s.resolveRoot("s1"); got != "/custom/spill" {
		t.Fatalf("显式覆盖应精确使用: %q", got)
	}
	if got := s.resolveRoot(""); got != "/custom/spill" {
		t.Fatalf("空会话亦用覆盖目录: %q", got)
	}
	t.Setenv(spillEnvDir, "")
	if got := s.resolveRoot("s9"); filepath.ToSlash(got) != "/base/temp/spill/s9" {
		t.Fatalf("默认根应按会话分目录: %q", got)
	}
	if got := s.resolveRoot(""); filepath.ToSlash(got) != "/base/temp/spill/default" {
		t.Fatalf("空会话回退 default: %q", got)
	}
}
