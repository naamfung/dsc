package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"dsc/proto"
)

// 重复工具调用提醒（第四实例，对齐 DSH guard/repeat-tool-reminder，advisory
// 形态）：统计以完全相同的规范化参数连续调用同一工具的次数，达到配置阈值时
// 经 PolicyDecision.notice 注入逐级增强的提醒，要求模型停止重复、重新分析
// 上次结果。仅建议性——不否决、不改写工具结果（notice 由宿主机械收集，调用方
// 在工具结果之后以合成用户消息投喂模型，对齐 DSH additionalContexts）。
//
// 对齐 DSH 的关键语义：
//   - 计数在 post-execute——被 deny/失败的调用同样流经此槽并计数：模型反复
//     硬敲一个被拒调用正是最值得打断的循环（Event.Error 非空照常计数）
//   - 链键为 (tool name, 深度规范化参数)；不同已跟踪调用重置链；排除工具
//     对链透明（不增不减，记录类工具穿插不能掩盖循环）
//   - 阈值升序归一后，首个阈值为简短通用提醒（键定 thresholds[0] 而非字面 3），
//     后续阈值为详细提醒（工具名/连续次数/参数预览）；配置非法装载即失败
//     （fail-loud：空表/非整数/<2/重复/预览上限 <1 一律拒绝启动）
//   - per-session 链属主（PolicyEvent.session，对齐 DSH per-agent WeakMap）；
//     会话无属主（空）不跟踪——对齐 DSH 直连 ctx.tools.execute 无 agent 不计数
//   - 用户插话重置链（agent/pre-step 事件载荷 UserInput=true，对齐 DSH
//     claimed-inbox 语义：用户插话改变了上下文，跨插话的重复不是循环）
//
// 配置（宿主 env 白名单放行 DSC_* 键，插件进程天然可见）：
//   DSC_REPEAT_THRESHOLDS     逗号分隔阈值，缺省 "3,5,8"；显式空/非法 = 启动失败
//   DSC_REPEAT_INCLUDE        逗号分隔跟踪工具通配（*），空 = 全部跟踪
//   DSC_REPEAT_EXCLUDE        逗号分隔排除工具通配（*），缺省 "todo_write"
//   DSC_REPEAT_PREVIEW_CHARS  详细提醒的参数预览上限（字符），缺省 500；检测
//                             恒用全量规范化串，预览上限只约束模型可见文本

const (
	reminderEventPreStep = "agent/pre-step"

	envRepeatThresholds = "DSC_REPEAT_THRESHOLDS"
	envRepeatInclude    = "DSC_REPEAT_INCLUDE"
	envRepeatExclude    = "DSC_REPEAT_EXCLUDE"
	envRepeatPreview    = "DSC_REPEAT_PREVIEW_CHARS"
)

// repeatReminderConfig 校验后的插件配置（装载期 fail-loud，运行期零回退分支）。
type repeatReminderConfig struct {
	thresholds   []int    // 升序、无重复、全部 >= 2
	include      []string // 空 = 全部跟踪；条目为 * 通配模式
	exclude      []string // 对链透明（不计数也不重置）
	previewChars int      // >= 1
}

// loadRepeatReminderConfig 读取并校验配置；任何非法值返回错误（调用方启动即退）。
func loadRepeatReminderConfig() (repeatReminderConfig, error) {
	cfg := repeatReminderConfig{
		thresholds:   []int{3, 5, 8},
		previewChars: 500,
	}
	if v := os.Getenv(envRepeatThresholds); v != "" {
		cfg.thresholds = nil
		for _, part := range strings.Split(v, ",") {
			p := strings.TrimSpace(part)
			if p == "" {
				continue // 空项不计入；全空项 = 显式空表，由下方 fail-loud 拒绝
			}
			n, err := strconv.Atoi(p)
			if err != nil {
				return cfg, fmt.Errorf("repeat-reminder: invalid %s %q — thresholds must be integers", envRepeatThresholds, v)
			}
			cfg.thresholds = append(cfg.thresholds, n)
		}
	}
	if len(cfg.thresholds) == 0 {
		return cfg, fmt.Errorf("repeat-reminder: %s must not be empty", envRepeatThresholds)
	}
	for _, t := range cfg.thresholds {
		if t < 2 {
			return cfg, fmt.Errorf("repeat-reminder: invalid threshold %d — every threshold must be an integer >= 2", t)
		}
	}
	seen := map[int]bool{}
	for _, t := range cfg.thresholds {
		if seen[t] {
			return cfg, fmt.Errorf("repeat-reminder: %s must not contain duplicates", envRepeatThresholds)
		}
		seen[t] = true
	}
	sort.Ints(cfg.thresholds) // 升序归一：thresholds[0] 恒为简短档
	cfg.include = splitPatterns(os.Getenv(envRepeatInclude))
	cfg.exclude = splitPatterns(os.Getenv(envRepeatExclude))
	if cfg.exclude == nil {
		cfg.exclude = []string{"todo_write"} // 缺省排除记录类工具（穿插不掩盖循环）
	}
	if v := os.Getenv(envRepeatPreview); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 {
			return cfg, fmt.Errorf("repeat-reminder: invalid %s %q — must be an integer >= 1", envRepeatPreview, v)
		}
		cfg.previewChars = n
	}
	return cfg, nil
}

// splitPatterns 逗号分隔通配模式列表（去空白、去空项）；空串返回 nil。
func splitPatterns(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// repeatChain 单会话的连续重复链：最近一次已跟踪调用的身份键与连击次数。
type repeatChain struct {
	tool      string
	canonical string
	count     int
}

// reminderServer 重复工具调用提醒策略服务。状态仅限 per-session 重复链
// （无模型可见语义之外的副作用），advisory 形态：永远 allow，只携带 notice。
type reminderServer struct {
	proto.UnimplementedPolicyServiceServer
	cfg repeatReminderConfig

	mu     sync.Mutex
	chains map[string]*repeatChain // key: PolicyEvent.session（per-session 属主）
}

func newReminderServer() (*reminderServer, error) {
	cfg, err := loadRepeatReminderConfig()
	if err != nil {
		return nil, err
	}
	return &reminderServer{cfg: cfg, chains: map[string]*repeatChain{}}, nil
}

// OnEvent 处理工具流水线事件：仅 tool/post-execute 观察计数（被拒/失败调用
// 同样计数），命中阈值时返回带 notice 的 allow 裁决；其余 kind 一律放行。
func (s *reminderServer) OnEvent(ctx context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
	if ev.GetKind() != kindPostExecute {
		return &proto.PolicyDecision{}, nil
	}
	// 无会话属主不跟踪（对齐 DSH：直连执行无 agent 可提醒、无链可挂）。
	if ev.GetSession() == "" {
		return &proto.PolicyDecision{}, nil
	}
	if !s.tracked(ev.GetTool()) {
		return &proto.PolicyDecision{}, nil
	}
	canonical := canonicalArgs(ev.GetArgumentsJson())

	s.mu.Lock()
	ch := s.chains[ev.GetSession()]
	var count int
	if ch != nil && ch.tool == ev.GetTool() && ch.canonical == canonical {
		ch.count++
		count = ch.count
	} else {
		s.chains[ev.GetSession()] = &repeatChain{tool: ev.GetTool(), canonical: canonical, count: 1}
		count = 1
	}
	s.mu.Unlock()

	idx := thresholdIndex(s.cfg.thresholds, count)
	if idx < 0 {
		return &proto.PolicyDecision{}, nil
	}
	return &proto.PolicyDecision{
		Notice: reminderText(ev.GetTool(), count, canonical, idx > 0, s.cfg.previewChars),
	}, nil
}

// handleHostEvent 宿主事件订阅（agent/pre-step）：载荷带 UserInput（新用户
// 输入进入会话）且可归属会话时重置该会话的重复链。恒返回 ("", nil)——纯重置
// 钩子，不改写载荷、不否决（waterfall 透传语义）。
func (s *reminderServer) handleHostEvent(ctx context.Context, eventType, dataJSON string) (string, error) {
	if eventType != reminderEventPreStep {
		return "", nil
	}
	var payload struct {
		Session   string `json:"session"`
		UserInput bool   `json:"user_input"`
	}
	if err := json.Unmarshal([]byte(dataJSON), &payload); err != nil {
		return "", nil
	}
	if payload.Session == "" || !payload.UserInput {
		return "", nil
	}
	s.mu.Lock()
	delete(s.chains, payload.Session)
	s.mu.Unlock()
	return "", nil
}

// tracked 判断工具是否参与链跟踪：include 非空时须命中其一，且不得命中 exclude
// （* 通配，path.Match；未命中 include 的调用与排除的调用同等透明）。
func (s *reminderServer) tracked(name string) bool {
	if len(s.cfg.include) > 0 && !matchAny(s.cfg.include, name) {
		return false
	}
	return !matchAny(s.cfg.exclude, name)
}

// matchAny 报告 name 是否命中任一 * 通配模式（其余元字符按字面匹配）。
func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// thresholdIndex 返回 count 命中的阈值下标（-1 = 未命中）。
func thresholdIndex(thresholds []int, count int) int {
	for i, t := range thresholds {
		if t == count {
			return i
		}
	}
	return -1
}

// canonicalArgs 规范化工具参数：JSON 解析后重序列化（Go 的 map 键序列化天然
// 深度升序），仅属性顺序不同的参数对象视为相同（对齐 DSH 深度排序后 stringify）。
// 解析失败回退原文（与 DSH raw-string fallback 同构）。
func canonicalArgs(argsJSON string) string {
	var v any
	if err := json.Unmarshal([]byte(argsJSON), &v); err != nil {
		return argsJSON
	}
	b, err := json.Marshal(v)
	if err != nil {
		return argsJSON
	}
	return string(b)
}

// previewArguments 头部截断规范化参数用于详细提醒引用，并标注省略量。
// 只约束模型可见文本——链键恒用全量规范化串。
func previewArguments(canonical string, capChars int) string {
	if len(canonical) <= capChars {
		return canonical
	}
	return canonical[:capChars] + fmt.Sprintf("… (+%d more chars)", len(canonical)-capChars)
}

// reminderText 渲染提醒：首个阈值简短通用提醒（键定 thresholds[0]），后续
// 阈值详细列出工具/连续次数/参数预览（对齐 DSH gentle/detailed 两档文本）。
func reminderText(name string, count int, canonical string, detailed bool, previewChars int) string {
	if !detailed {
		return "You are repeating the exact same tool call with identical arguments. " +
			"Carefully analyze the previous result before calling again: if the task is not " +
			"complete, try a different approach or different arguments instead of repeating the call."
	}
	return fmt.Sprintf("Repeated tool call detected:\n"+
		"- tool: %s\n- consecutive_calls: %d\n- arguments: %s\n"+
		"The repeated calls are not making progress. Do not call this tool with these exact "+
		"arguments again. Inspect the latest result and choose a different action, different "+
		"arguments, or finish the task if enough evidence has been gathered.",
		name, count, previewArguments(canonical, previewChars))
}
