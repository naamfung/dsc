package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"dsc/proto"
)

// spill-policy 外置策略（第三实例，对齐 DSH spill-policy 的 tools/post-execute
// 结果变换器形态，自 plugins/policy-spill 迁入的核心插件混合体驻留）：超长纯
// 文本工具结果不进模型上下文——全文保存到本插件管理的外置存储，模型侧只见
// 「头尾预览 + 定位符 + 取回指引」，需要完整内容时用标准 view 工具按定位符
//（文件路径）读取。
//
// 设计对齐（DSH spill-policy / spill-local）：
//   - locator 即文件路径（DSH spill-local：locator 为路径、retrievalHint 指引
//     「Use read with offset/limit」）；DSC 的等价取回路径是 str_replace_editor
//     的 view 命令（支持 view_range 分段）与 shell grep 搜索。
//   - 豁免取回工具（DSH 豁免 read）：str_replace_editor 的 view 命令结果不再
//     外置，避免「取回外置内容 → 又被外置 → 再取回」死循环（read → spill →
//     read again 语义同构）。编辑器的写命令（create/str_replace/insert）结果
//     远小于阈值，天然不触发。
//   - 告示成本在阈值内预留（DSH reserve 逻辑）：替换体（预览 + 告示）永不超阈值；
//     告示单独就超阈值（极小阈值/超长路径）时不产生阈内替换，保留内联——外置
//     文件成为无害孤儿（清理延后，对齐 DSH 注释）。
//   - 尽力而为（best-effort）：无会话属主、存储失败一律保留内联结果，绝不把
//     成功的工具调用变成失败、绝不掩盖内联结果（DSH：A spill failure must
//     NEVER turn a successful tool call into an isError）。
//   - 失败结果不处理（错误是权威观察，桥已把 error 透传；外置只塑造被接受的
//     成功结果，不动纠正性反馈——对齐 DSH block pass-through）。
//
// 与 DSH 的有意分歧（词汇随域，语义同构）：
//   - 阈值以字符计（DSC 既有约定，DSC_SPILL_THRESHOLD；对 CJK 更贴近 token 计量），
//     非 DSH 的 UTF-8 字节；显式 0 禁用本策略（对齐 policy-timeout 的 0 禁用约定）。
//   - 无 ptc-dispatch-log 臂（DSC 无 PTC 子调用分发日志）。
//   - 无嵌套组合调用豁免（DSC 流水线无 exec.parent 概念）。
//   - 存储归本插件（DSC 插件为独立进程，自然拥有自己的存储；DSH 经 ctx 注入
//     spillStore 后端），目录沿用宿主旧约定：exe 目录 temp/spill/<session>
//     （宿主 spawn 插件时 cmd.Dir=ExecDir，插件经工作目录定位；DSC_SPILL_DIR
//     显式覆盖时为精确目录、不分会话），24 小时清理仍由宿主 cleanupOldTempDirs 覆盖。

// spillDefaultThreshold 外置阈值的默认字符数（对齐宿主旧约定）。
const spillDefaultThreshold = 4000

// spillEnvDir / spillEnvThreshold 环境变量（宿主 env 白名单保留 DSC_* 键，
// 插件进程天然可见；与旧宿主实现同名同义）。
const (
	spillEnvDir       = "DSC_SPILL_DIR"
	spillEnvThreshold = "DSC_SPILL_THRESHOLD"
)

// retrievalTool 取回工具名（豁免表主体）：编辑器的 view 命令是本策略定位符的
// 标准取回路径，豁免外置以防死循环（对齐 DSH 豁免 read）。
const retrievalTool = "str_replace_editor"

// thresholdChars 返回外置阈值（字符数）：DSC_SPILL_THRESHOLD 显式 0 = 禁用
// （返回 0，调用方以空裁决放行一切）；未设或非法回退默认值。
func thresholdChars() int {
	if s := os.Getenv(spillEnvThreshold); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return n
		}
	}
	return spillDefaultThreshold
}

// editorArgs 本插件关心的参数子集（豁免判定只认编辑器命令名；参数提取在插件
// 侧完成，宿主不解读任何领域语义）。
type editorArgs struct {
	Command string `json:"command"`
}

// isRetrievalCall 报告该事件是否为取回路径调用（view 命令）——豁免外置。
// 参数不可解读时不豁免（策略不放大参数错误；超长结果照常外置）。
func isRetrievalCall(tool, argsJSON string) bool {
	if tool != retrievalTool {
		return false
	}
	var args editorArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return false
	}
	return args.Command == "view"
}

// sessionStore 单会话的外置存储：递增 id 存为 <dir>/spill-<n>.txt。
// 首次触及时扫描目录续接编号（对齐旧宿主 SpillStore 的重启续号语义），
// 避免插件重启后覆盖既有文件（定位符指向的内容被篡改）。
type sessionStore struct {
	dir  string
	mu   sync.Mutex
	next int64
}

// spillServer 外置策略服务（dsc-system 驻留）。状态仅限编号续接的存储句柄
// （无模型可见语义），按 PolicyEvent.session 的会话属主隔离目录（per-session
// owner，对齐 DSH SpillOwner.sessionId——后端按产出会话归组存储）。
type spillServer struct {
	proto.UnimplementedPolicyServiceServer
	mu      sync.Mutex
	stores  map[string]*sessionStore
	cwdFunc func() (string, error) // 工作目录解析（宿主 spawn 时 cmd.Dir=ExecDir；可注入替身）
}

func newSpillServer() *spillServer {
	return &spillServer{
		stores:  make(map[string]*sessionStore),
		cwdFunc: os.Getwd,
	}
}

// resolveRoot 解析外置根目录：DSC_SPILL_DIR 显式覆盖（精确目录，不分会话——
// 对齐旧宿主约定）；否则 <工作目录>/temp/spill/<session>（工作目录即宿主
// ExecDir——宿主 spawn 插件时统一 cmd.Dir=ExecDir）。空会话回退 "default"。
func (s *spillServer) resolveRoot(session string) string {
	if d := os.Getenv(spillEnvDir); d != "" {
		return d
	}
	if session == "" {
		session = "default"
	}
	cwd, err := s.cwdFunc()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "temp", "spill", session)
}

// storeFor 取得（或创建）会话存储句柄：首次触及时建目录并扫描续接编号。
func (s *spillServer) storeFor(session string) (*sessionStore, error) {
	root := s.resolveRoot(session)
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.stores[root]; ok {
		return st, nil
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("spill store: mkdir %s: %w", root, err)
	}
	st := &sessionStore{dir: root, next: 1}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("spill store: read dir: %w", err)
	}
	for _, e := range entries {
		var n int64
		if _, err := fmt.Sscanf(e.Name(), "spill-%d.txt", &n); err == nil && n >= st.next {
			st.next = n + 1
		}
	}
	s.stores[root] = st
	return st, nil
}

// saveText 保存全文并返回定位符（绝对文件路径，对齐 DSH spill-local 的
// 路径形态 locator——消费方以 retrievalHint 呈现，不解析）。
func (st *sessionStore) saveText(content string) (string, error) {
	st.mu.Lock()
	id := st.next
	st.next++
	st.mu.Unlock()
	path := filepath.Join(st.dir, fmt.Sprintf("spill-%d.txt", id))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("spill store: write %s: %w", path, err)
	}
	return path, nil
}

// OnEvent 实现 proto.PolicyServiceServer：仅 tool/post-execute 槽参与裁决；
// 其他槽一律放行（空裁决）。裁决仅两种形态：replace（已外置并写出预览）或
// 空裁决（未达阈值/取回豁免/失败结果/存储失败/阈内无法替换——保留内联）。
func (s *spillServer) OnEvent(_ context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
	if ev.GetKind() != kindPostExecute {
		return &proto.PolicyDecision{}, nil
	}
	threshold := thresholdChars()
	if threshold <= 0 {
		// env 显式 0：本策略禁用（对齐 DSH「未配置 = 完全不注册」的 no-op 语义）
		return &proto.PolicyDecision{}, nil
	}
	// 失败结果不外置（错误是权威观察；外置只塑造被接受的成功结果）
	if ev.GetError() != "" || isRetrievalCall(ev.GetTool(), ev.GetArgumentsJson()) {
		return &proto.PolicyDecision{}, nil
	}
	result := ev.GetResult()
	runes := []rune(result)
	if len(runes) <= threshold {
		return &proto.PolicyDecision{}, nil
	}
	st, err := s.storeFor(ev.GetSession())
	if err != nil {
		// 尽力而为：存储不可用保留内联结果（策略缺失降级为无策略，而非工具不可用）
		log.Printf("spill-policy: store unavailable, keeping inline result: %v", err)
		return &proto.PolicyDecision{}, nil
	}
	locator, err := st.saveText(result)
	if err != nil {
		// 保存失败保留内联结果：外置失败绝不把成功调用变成失败（DSH best-effort）
		log.Printf("spill-policy: save failed, keeping inline result: %v", err)
		return &proto.PolicyDecision{}, nil
	}
	replaced, ok := spillReplacement(result, locator, threshold)
	if !ok {
		// 阈内无法容纳替换体（告示/定位符单独就超阈值）：保留内联，
		// 已写出的外置文件为无害孤儿（对齐 DSH：cleanup is deferred）
		log.Printf("spill-policy: no within-threshold replacement for %s; keeping inline", ev.GetTool())
		return &proto.PolicyDecision{}, nil
	}
	return &proto.PolicyDecision{Action: actionReplace, Result: replaced}, nil
}

// spillReplacement 把完整结果替换为「头尾预览 + 定位符 + 取回指引」：
// 预览预算 = 阈值 − 告示预留（DSH reserve 逻辑：以最坏情形字符数计价告示，
// 其位数覆盖真实省略数的位数，预留即安全上界）；预览超预算分半分布头尾。
// 返回 ok=false 表示阈内容不下替换体（调用方保留内联）。
func spillReplacement(content, locator string, threshold int) (string, bool) {
	runes := []rune(content)
	// 告示以最坏情形计数（完整字符总数）计价：其位数 ≥ 真实计数的位数
	worst := spillNotice(locator, len(runes))
	header := fmt.Sprintf("[内容已外置: %s]", locator)
	// 替换体 = header + "\n" + head + "\n" + notice + "\n" + tail；固定成本 =
	// header、notice 与三个换行连接符
	reserve := len([]rune(header)) + len([]rune(worst)) + 3
	budget := threshold - reserve
	if budget <= 0 {
		return "", false
	}
	head := (budget + 1) / 2
	tail := budget / 2
	if len(runes) <= head+tail {
		return "", false // 预览即可容纳全文（不会发生：调用方已保证超阈值）
	}
	headStr := string(runes[:head])
	tailStr := string(runes[len(runes)-tail:])
	notice := spillNotice(locator, len(runes)-(head+tail))
	replaced := header + "\n" + headStr + "\n" + notice + "\n" + tailStr
	// 不变量：替换体永不超阈值（DSH：the policy NEVER emits a replacement
	// larger than the cap）。防御性终检（预算推导已保证，此处兜底）。
	if len([]rune(replaced)) > threshold {
		return "", false
	}
	return replaced, true
}

// spillNotice 告示文案：省略计数 + 定位符（文件路径）+ 取回指引。
// 取回路径 = 标准 view 命令（支持 view_range 分段）或 shell grep 搜索——
// 对齐 DSH retrievalHint「Use read with offset/limit, or grep this path」。
func spillNotice(locator string, omitted int) string {
	return fmt.Sprintf("...(中间省略 %d 字符。完整内容已存于 %s：用 str_replace_editor 的 view 命令读取该文件（可配 view_range 分段），或用 shell grep 在该文件中搜索。不要重复调用原工具——重复调用只会返回相同结果并生成新的外置文件)",
		omitted, locator)
}
