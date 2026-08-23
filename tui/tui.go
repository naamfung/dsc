// Package tui 提供基于 Bubble Tea 的终端聊天界面。
// 该界面运行在宿主进程中（不通过 go-plugin 子进程），
// 因为 TUI 需要直接操作终端 raw mode 和 stdout，而插件子进程的 stdout 会被 go-plugin 捕获。
package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dsc/cron"
	"dsc/jobs"
	"dsc/plugin"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
	"gopkg.in/yaml.v3"
)

// 布局尺寸常量：标题栏一行；状态栏两行（含分隔线）；指标行一行；输入区动态高度（1~composerMax 行）+ 上下边框 2 行。
const (
	titleRows   = 1
	statusRows  = 2
	infoRows    = 1
	composerMax = 8
	boxBorder   = 2
	thinkingRow = 1
)

// assistantGutter 助手/用户正文的统一左缩进（2 格），参考 REX 的 transcript gutter，
// 为一式的消息块留出稳定的左边距。
const assistantGutter = "  "

// assistantMark 助手身份头的标记符号：REX 用菱形 ◈/◆，观感偏硬，改用星号「✦」更轻盈。
const assistantMark = "✦"

// 主题样式
var (
	accent = lipgloss.Color("#7D56F4")

	titleSty = lipgloss.NewStyle().
			Background(accent).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1)

	userTextSty = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	assistantNameSty = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#05A5A5")).
				Bold(true)

	dimSty = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888"))

	// dividerSty 状态栏分隔线：U+2500 轻线已是最细的框线字符，改用更暗的灰色
	// 使其在视觉上更纤细、不抢注意力。
	dividerSty = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4A4A4A"))

	// reasonReapply 思考块的暗色 SGR 前缀：「前景 #6B6B6B(107,107,107)」。
	// 思考块先按 markdown 渲染，行内样式会以 \x1b[m 复位；本前缀用于整体着色，
	// 并在每次复位后重新套用，保证加粗/强调等行内样式不会清掉整块的弱化效果。
	// 仅用灰色、不加斜体，避免斜体影响长段思考内容的阅读。
	reasonReapply = "\x1b[38;2;107;107;107m"

	errorSty = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F87")).
			Bold(true)

	composerBoxSty = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, true, false).
			BorderForeground(accent).
			PaddingLeft(1)

	compSelSty = lipgloss.NewStyle().
			Foreground(accent).
			Bold(true)

	// selStyle 正文拖拽选中的反色高亮样式。
	selStyle = lipgloss.NewStyle().Reverse(true)
)

// indentBlock 为块中的每个非空行加上缩进，空行保持空白，供消息块统一做左边距。
func indentBlock(block, indent string) string {
	if indent == "" || block == "" {
		return block
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

// joinReasoningAnswer 拼装思考块与答案正文：两者都存在时中间留一个空行，
// 使思考块与正文之间有呼吸间距（REX 式版面），避免两段文本紧贴。
func joinReasoningAnswer(reasoning, answer string) string {
	reasoning = strings.TrimRight(reasoning, "\n")
	if reasoning == "" {
		return answer
	}
	if strings.TrimSpace(answer) == "" {
		return reasoning
	}
	return reasoning + "\n\n" + answer
}

// selPos 表示正文被选中区域中的位置：以「内容行号 + 可视列」定位，
// 行号是绝对值（与滚动无关），列是可视列。
type selPos struct{ line, col int }

// selection 是鼠标左键在正文区拖拽产生的文本选区。anchor 为按下起点，
// head 为当前终点；active 控制是否渲染与复制。坐标是绝对内容行，滚动不会移动它们。
type selection struct {
	active       bool
	anchor, head selPos
}

func (s selection) ordered() (start, end selPos) {
	if s.anchor.line > s.head.line || (s.anchor.line == s.head.line && s.anchor.col > s.head.col) {
		return s.head, s.anchor
	}
	return s.anchor, s.head
}

func (s selection) empty() bool { return s.anchor == s.head }

// submitResult 是一次 agent.Run 完成后的结果消息
type submitResult struct {
	input  string
	result *plugin.AgentResult
	err    error
}

// streamFrame 是流式響應的一幀
type streamFrame struct {
	input string
	frame *plugin.RunStreamResponse
	ch    <-chan *plugin.RunStreamResponse
	first bool
	done  bool
	err   error
}

// injectedMsg 实时注入完成的消息（仅用于错误回显；成功注入无需额外 UI 动作）。
type injectedMsg struct {
	err error
}

// compItem 是斜杆命令菜单的一行：label 是完整命令，hint 是右侧提示。
type compItem struct {
	label  string
	insert string
	hint   string
}

// completion 是斜杆命令补全菜单状态；active 为 false 时菜单关闭。
type completion struct {
	active bool
	items  []compItem
	sel    int
}

// Model 是聊天界面的状态模型
type Model struct {
	agent     plugin.Agent
	manager   *plugin.Manager // 插件管理器，用於實時切換模式
	ctx       context.Context
	modelName string
	mode      string // 當前預設模式（minimal / standard），實時反映切換

	// 上下文容量顯示：contextWindow 為總容量（token 數），usedTokens 為已用容量
	contextWindow int
	usedTokens    int

	// 当前轮运行指标（对齐 REX working 行）：runStart 记录启动时刻，elapsed 为
	// 已耗时秒数（每秒 elapsedTick 刷新）；turnTokens 为本轮累计下行生成 token；
	// cacheHit/cacheMiss 为最近一次请求的 prompt 缓存命中/写入 token。
	runStart   time.Time
	elapsed    int
	turnTokens int
	cacheHit   int32
	cacheMiss  int32

	// 正文拖拽选区的实时状态与选中后的宽度对齐渲染行缓存
	sel          selection
	wrappedLines []string

	// 输入历史：已提交命令 + 当前草稿，用于 ↑/↓ 翻阅
	history []string
	histPos int
	draft   string

	// 复制提示：选区复制成功后短暂显示，由 copyNoticeSeq 防过期竞态
	copyNotice    string
	copyNoticeSeq int

	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	ready     bool // viewport 在收到首个窗口尺寸后初始化
	thinking  bool // 正在等待 agent 响应
	streaming bool // 正在流式输出

	completion completion // 斜杆命令补全菜单

	lines []string // 渲染后的历史行
	width int
	high  int

	// 流式渲染状态
	streamBuffer string
	streamOpen   bool
	streamMsgIdx int

	// 滚动条拖拽状态：scrollbarDrag 标记左键按住滚动条列，
	// scrollbarGrabOffset 为抓取点在滑块内的行偏移（拖拽时保持相对位置）。
	scrollbarDrag       bool
	scrollbarGrabOffset int

	// 当前一轮的流式通道暂存：供运行中输入（注入）时维持泵取，避免流式流停滞
	streamInput string
	streamCh    <-chan *plugin.RunStreamResponse

	// 思考过程（reasoning）渲染状态：reasoningBuffer 累积增量，
	// reasoningOpen 标记当前正文块是否处于思考状态；transition 到答案时
	// 把已渲染的思考块并入 reasoningCommitted，随后与答案正文一起显示。
	reasoningBuffer    string
	reasoningOpen      bool
	reasoningCommitted string

	// 当前一轮的取消句柄：Ctrl+C 在响应/流式输出期间中断本轮（终止当前操作）
	turnCancel      context.CancelFunc
	streamCancelled bool // 中断标记：置位后丢弃后续 streamFrame，停止本轮泵取

	// 会话运行指标（对齐 DSH 定义）：curTurn 为当前轮次编号（一次受理输入的排空），
	// curStep 为当前步编号（一次模型请求及其引发的工具执行）。由 agent 发射的流帧
	// 携带的 Turn/Step 实时更新，避免本地计数与 agent 侧（含运行中注入）不一致。
	curTurn int32
	curStep int32

	// mouseCaptureOff 为 true 时释放鼠标给终端（MouseModeNone），恢复终端原生
	// 文字选中/复制（模型工作期间也可用）；由 /mouse 命令或 DSC_DISABLE_MOUSE
	// 切换，代价是应用内滚轮滚动与正文拖选复制暂时失效。
	mouseCaptureOff bool

	// 当前会话 id（初始 default；/session 切换或新建时更新，供 /export 使用）
	currentSessionID string

	// 用户评审通道：program 供 askProvider 把问题送进事件循环；question 为待回答的问题
	program  *tea.Program
	question *pendingQuestion

	// 后台任务完成通知唤醒（对齐 DSH completionDelivery: wakeup）：
	// wakeBudget 连续唤醒预算（用户消息恢复），pendingWakeups 忙碌/预算耗尽时排队
	// 的通知（下一条用户消息前置注入）。
	wakeBudget     int
	pendingWakeups []string
}

// New 创建一个聊天界面模型
func New(agent plugin.Agent, manager *plugin.Manager, ctx context.Context, modelName, mode string, contextWindow int) *Model {
	input := textarea.New()
	input.Placeholder = "输入消息，回车发送，Shift+Enter/Ctrl+J 换行，Ctrl+Q 退出"
	input.CharLimit = 4096
	input.ShowLineNumbers = false
	// 对齐 REX：关闭虚拟光标，改用真实终端光标。虚拟光标在消息区动态重排时无法
	// 稳定跟随输入位置，会导致中文输入法候选窗/文字光标随上方输出横向漂移；
	// 真实光标由 View 显式锚定到输入插入点（见 View 末尾的 v.Cursor 设置）。
	input.SetVirtualCursor(false)
	// 对齐 REX：动态高度让输入区随内容自动增长（1~composerMax 行），多行输入不再被
	// 压扁到固定 3 行；回车在宿主层拦截用于发送，换行交给 Shift+Enter/Ctrl+J/Alt+Enter。
	input.DynamicHeight = true
	input.MinHeight = 1
	input.MaxHeight = composerMax
	input.SetHeight(1)
	// 仅首行显示提示符「❯ 」，续行留空；高度随内容行数变化（见 syncInputHeight）
	input.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber != 0 {
			return ""
		}
		return "❯ "
	})
	// 回车在宿主层拦截用于发送，这里仅兜底新增换行键位。
	// 终端协议里 Enter/Ctrl+Enter 都发送相同的 CR 字节（Bubble Tea v2 无法区分），
	// 因此统一用 Ctrl+J（LF）/Shift+Enter/Alt+Enter 作为换行键（对齐 REX 键位）。
	input.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+j", "shift+enter", "alt+enter"),
		key.WithHelp("ctrl+j/shift+enter", "换行"),
	)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accent)

	return &Model{
		agent:            agent,
		manager:          manager,
		ctx:              ctx,
		modelName:        modelName,
		mode:             mode,
		contextWindow:    contextWindow,
		input:            input,
		spinner:          s,
		currentSessionID: "default",
		wakeBudget:       maxConsecutiveWakes,
		mouseCaptureOff:  mouseCaptureOffByDefault(),
	}
}

// mouseCaptureOffByDefault 允许用户通过 DSC_DISABLE_MOUSE 环境变量在启动时默认
// 释放鼠标（对齐 REX 的 REX_DISABLE_MOUSE），免去每次会话敲 /mouse。
func mouseCaptureOffByDefault() bool {
	v := strings.TrimSpace(os.Getenv("DSC_DISABLE_MOUSE"))
	return v != "" && v != "0"
}

// Init 返回初始化命令
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.input.Focus(),
		m.spinner.Tick,
	)
}

// submitCmd 发起一次 agent.RunStream，结果通过 streamFrame 消息返回
func (m *Model) submitCmd(input string) tea.Cmd {
	return func() tea.Msg {
		// 为本轮单独建一个可取消的上下文，供 Ctrl+C 中断当前操作
		cctx, cancel := context.WithCancel(m.ctx)
		ch, err := m.agent.RunStream(cctx, input)
		if err != nil {
			cancel()
			return streamFrame{input: input, err: err, done: true}
		}
		m.turnCancel = cancel
		m.streamCancelled = false
		m.streamInput = input
		m.streamCh = ch
		return streamFrame{input: input, ch: ch, first: true}
	}
}

// pumpStream 读取通道的下一帧
func (m *Model) pumpStream(input string, ch <-chan *plugin.RunStreamResponse) tea.Cmd {
	return func() tea.Msg {
		frame, ok := <-ch
		if !ok {
			return streamFrame{input: input, done: true}
		}
		return streamFrame{input: input, frame: frame, ch: ch}
	}
}

// clearStream 清空本轮暂存的流式通道（一轮结束时调用）。
func (m *Model) clearStream() {
	m.streamInput = ""
	m.streamCh = nil
}

// pumpStreamIfOpen 若当前存在进行中的流式通道，返回继续泵取的命令；
// 否则返回 nil。用于运行中输入（注入/编辑）时维持流式流不断。
func (m *Model) pumpStreamIfOpen() tea.Cmd {
	if m.streamCh != nil && m.streamInput != "" {
		return m.pumpStream(m.streamInput, m.streamCh)
	}
	return nil
}

// injectCmd 将用户输入实时注入到运行中 agent 的会话历史（跨进程 RPC）。
// 成功注入无需额外 UI 动作（气泡已在上游同步渲染）；失败仅回显错误，不中断当前流。
func (m *Model) injectCmd(text string) tea.Cmd {
	return func() tea.Msg {
		err := m.agent.InjectMessage(m.ctx, text)
		return injectedMsg{err: err}
	}
}

// copySelected 复制当前选区文本到剪贴板并弹出来回提示；无选区或空文本则不处理。
func (m *Model) copySelected() tea.Cmd {
	text := m.selectedText()
	m.sel = selection{}
	m.render()
	if text == "" {
		return nil
	}
	_ = clipboard.WriteAll(text)
	m.copyNotice = fmt.Sprintf("已复制 %d 字符", runeCount(text))
	m.copyNoticeSeq++
	return copyNoticeExpire(m.copyNoticeSeq)
}

// interruptTurn 中断当前一轮（走 cancelable ctx，参考 rex）：调用按轮取消并置位中断标记，
// 丢弃后续 streamFrame，复位流式状态回到就绪。
func (m *Model) interruptTurn() (tea.Model, tea.Cmd) {
	if m.turnCancel != nil {
		m.turnCancel()
		m.turnCancel = nil
	}
	m.streamCancelled = true
	m.thinking = false
	m.streaming = false
	m.streamOpen = false
	m.clearStream()
	m.viewport.SetHeight(m.vpHeight())
	m.render()
	m.input.Focus()
	m.viewport.GotoBottom()
	return m, nil
}

// vpHeight 计算消息区高度：总高度减去标题、状态栏、输入区（含边框），思考时再减去指示行，补全菜单时再减去补全菜单行。
// 输入区高度随内容行数变化（1~composerMin 行），这里以当前实际高度为准。
func (m *Model) vpHeight() int {
	h := m.high - titleRows - statusRows - infoRows - boxBorder - m.input.Height()
	if m.thinking || m.streaming {
		h -= thinkingRow
	}
	if m.completion.active && len(m.completion.items) > 0 {
		// 减去补全菜单的高度：items 数量 + 1 行提示
		h -= (len(m.completion.items) + 1)
	}
	if h < 3 {
		h = 3
	}
	return h
}

// syncInputHeight 同步消息区高度：DynamicHeight 模式下 textarea 在内容变化时自动
// 调整自身高度（clamp 在 MinHeight~composerMax），这里只需据当前输入框高度重算
// viewport 高度并滚到底部，避免内容行变化后消息区多占/少占一行。
func (m *Model) syncInputHeight() {
	// 总是同步 viewport 高度，以应对 thinking 等状态变化导致的高度重算
	m.viewport.SetHeight(m.vpHeight())
	m.viewport.GotoBottom()
}

// displayModelName 返回展示用模型名（优先取命令行/配置传入的，其次取 agent 自身名）。
func (m *Model) displayModelName() string {
	if m.modelName != "" {
		return m.modelName
	}
	return m.agent.Name(m.ctx)
}

// Update 处理消息
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.high = msg.Height
		m.input.SetWidth(max(msg.Width-8, 16))
		if !m.ready {
			m.viewport = viewport.New(viewport.WithWidth(max(msg.Width-1, 10)), viewport.WithHeight(m.vpHeight()))
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.SetWidth(max(msg.Width-1, 10))
			m.viewport.SetHeight(m.vpHeight())
		}
		m.render()
		m.viewport.GotoBottom()

	case tea.PasteMsg:
		// 终端括号粘贴（如 Windows Terminal 的 Ctrl+V）插入到输入框。
		if m.thinking || m.streaming {
			return m, nil
		}
		m.input.InsertString(strings.ReplaceAll(msg.Content, "\r\n", "\n"))
		m.syncInputHeight()
		m.updateCompletion()
		return m, nil

	case tea.MouseClickMsg:
		if m.thinking || m.streaming {
			return m, nil
		}
		mm := msg.Mouse()
		if mm.Button == tea.MouseLeft {
			if m.inScrollbar(mm.X, mm.Y) {
				// 按住滚动条列：进入拖拽模式（不产生正文选区）
				m.sel = selection{}
				m.scrollbarDrag = true
				m.scrollbarGrabOffset = m.scrollbarGrabRowOffset(mm.Y - 1)
				m.dragScrollbar(mm.Y - 1)
			} else if m.inBody(mm.X, mm.Y) {
				at := m.transcriptCaret(mm.X, mm.Y)
				m.sel = selection{active: true, anchor: at, head: at}
			} else {
				// 点按输入框或状态区等非正文区域 → 清除正文选区
				m.sel = selection{}
			}
			m.render()
		}
		return m, nil

	case tea.MouseMotionMsg:
		// CellMotion 下仅按住鼠标才会收到移动事件，即拖拽。
		mm := msg.Mouse()
		if m.scrollbarDrag {
			m.dragScrollbar(mm.Y - 1)
			m.render()
			return m, nil
		}
		if m.sel.active {
			if mm.Button == tea.MouseLeft {
				m.sel.head = m.transcriptCaret(mm.X, mm.Y)
				m.render()
			}
		}
		return m, nil

	case tea.MouseReleaseMsg:
		// 松开左键：若在滚动条拖拽中则结束拖拽；否则根据拖拽选区复制文本，然后清除选区。
		if m.scrollbarDrag {
			m.scrollbarDrag = false
			m.scrollbarGrabOffset = 0
			m.render()
			return m, nil
		}
		if m.sel.active {
			mm := msg.Mouse()
			if mm.Button == tea.MouseLeft {
				m.sel.head = m.transcriptCaret(mm.X, mm.Y)
			}
			cmd := m.copySelected()
			m.render()
			return m, cmd
		}
		return m, nil

	case copyNoticeMsg:
		if m.copyNoticeSeq == msg.seq {
			m.copyNotice = ""
			m.render()
		}
		return m, nil

	case questionMsg:
		if msg.request == nil {
			m.question = nil
		} else {
			m.question = &pendingQuestion{request: msg.request, answer: msg.answer, err: msg.err}
		}
		m.render()
		return m, nil

	case jobDoneMsg:
		// 后台任务完成：排队通知并尝试唤醒（空闲自动开启一轮）
		m.pendingWakeups = append(m.pendingWakeups, renderJobDoneNotice(msg.snapshot))
		m.render()
		return m, m.tryWakeup()

	case tea.KeyPressMsg:
		// 问题覆盖层激活时，按键只用于选择/确认/放弃
		if m.question != nil {
			return m.handleQuestionKey(msg)
		}
		// 运行/流式期间同样允许输入：打字回车走「实时注入」，不打扰当前工作（参考 rex，
		// 但注入无需等本轮完成——追加到历史末端的用户消息会在下一次 LLM 迭代被模型看到）。
		// Ctrl+Q 退出；Ctrl+C 中断当前操作。
		inRunning := m.thinking || m.streaming
		// 命令补全菜单打开时，导航/补全键由菜单接管（运行中输入时同样可用）
		if m.completion.active {
			switch msg.String() {
			case "up", "ctrl+p":
				m.moveCompletion(-1)
				return m, nil
			case "down", "ctrl+n":
				m.moveCompletion(1)
				return m, nil
			case "tab", "enter":
				if msg.String() == "enter" && m.completionExactLabel() {
					m.completion = completion{}
					break // 已输入完整命令，交给下面 Enter 的提交逻辑
				}
				m.acceptCompletion()
				return m, nil
			case "esc":
				m.completion = completion{}
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+q":
			return m, tea.Quit
		case "ctrl+c":
			if inRunning {
				return m.interruptTurn()
			}
			// 恢复复制语义（参考 rex）：有选区则复制；否则清空输入；都无则忽略（退出已移交给 Ctrl+Q）
			if m.sel.active {
				return m, m.copySelected()
			}
			if strings.TrimSpace(m.input.Value()) != "" {
				m.input.SetValue("")
				m.syncInputHeight()
				m.updateCompletion()
			}
			return m, nil
		case "up", "down":
			// 输入为单行时 ↑/↓ 翻阅历史命令；多行输入仍交给输入框移动光标。
			if !strings.Contains(m.input.Value(), "\n") {
				m.navigateHistory(msg.String() == "up")
				m.syncInputHeight()
				m.updateCompletion()
				return m, m.pumpStreamIfOpen()
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, tea.Batch(cmd, m.pumpStreamIfOpen())
		case "ctrl+v":
			// 某些终端不把 Ctrl+V 当成括号粘贴，而是交回应用处理，此时手动读剪贴板。
			if inRunning {
				return m, tea.Batch(readClipboardPaste(), m.pumpStreamIfOpen())
			}
			return m, readClipboardPaste()
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if handled, cmd := m.runSlashCommand(text); handled {
				return m, tea.Batch(cmd, m.pumpStreamIfOpen())
			}
			if text == "" {
				return m, m.pumpStreamIfOpen()
			}
			// 排队中的完成通知注入本轮输入（对齐 DSH：忙碌 owner 的通知进入 next-step inbox）
			sendText := text
			if len(m.pendingWakeups) > 0 {
				sendText = strings.Join(m.pendingWakeups, "\n\n") + "\n\n" + text
				m.pendingWakeups = nil
			}
			// 记录到历史（仅用户消息，斜杆命令不入历史）
			m.history = append(m.history, text)
			m.histPos = len(m.history)
			m.draft = ""
			m.input.SetValue("")
			m.completion = completion{}
			m.wakeBudget = maxConsecutiveWakes // 用户消息恢复唤醒预算（对齐 DSH）
			if inRunning {
				// 正在工作：不作为新轮，而是把消息实时注入会话历史（不停止当前工作），
				// 本地即刻渲染用户气泡；流式通道继续保持泵取。轮次编号由 agent 流帧
				// 携带的 Turn 维护（注入不新开物理轮次，对齐 DSH turn 定义）。
				m.appendMessage(renderUserBubble(text, m.width-4))
				m.syncInputHeight()
				m.render()
				m.viewport.GotoBottom()
				return m, tea.Batch(m.injectCmd(sendText), m.pumpStreamIfOpen(), m.input.Focus())
			}
			return m, m.startTurn(sendText, renderUserBubble(text, m.width-4))
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.syncInputHeight() // 新增/删除换行后同步输入框高度
			m.updateCompletion()
			return m, tea.Batch(cmd, m.pumpStreamIfOpen())
		}

	case spinner.TickMsg:
		if m.thinking || m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case elapsedTickMsg:
		// 每秒刷新「思考中」行的耗时（对齐 REX elapsed tick）
		if m.thinking || m.streaming {
			m.elapsed = int(time.Since(m.runStart).Seconds())
			m.render()
			return m, elapsedTick()
		}
		return m, nil

	case streamFrame:
		// 已被 Ctrl+C 中断本轮的残留帧直接丢弃，停止泵取
		if m.streamCancelled {
			return m, nil
		}
		// 跟踪 agent 发射的轮/步编号（对齐 DSH 定义），用于状态行实时显示。
		// 帧携带的编号是权威值（含运行中注入的续步），本地不做推算。
		if msg.frame != nil {
			if msg.frame.Turn != 0 {
				m.curTurn = msg.frame.Turn
			}
			if msg.frame.Step != 0 {
				m.curStep = msg.frame.Step
			}
		}
		if msg.first {
			m.thinking = false
			m.streaming = true
			m.streamBuffer = ""
			// 創建助手消息佔位符（僅身份頭）；新建块时重置思考/答案状态
			m.openAssistantBlock()
			m.render()
			m.viewport.GotoBottom()
			if msg.ch == nil {
				m.streaming = false
				m.streamOpen = false
				m.clearStream()
				if msg.err != nil {
					m.appendMessage(errorSty.Render("错误: ") + msg.err.Error())
				}
				m.input.Focus()
				m.viewport.GotoBottom()
				return m, nil
			}
			return m, m.pumpStream(msg.input, msg.ch)
		}
		if msg.done {
			m.thinking = false
			m.streaming = false
			m.streamOpen = false
			m.clearStream()
			if msg.err != nil {
				m.appendMessage(errorSty.Render("错误: ") + msg.err.Error())
			} else {
				// 通道关闭收尾：frame 为 nil，此前 success/error 帧可能只设置了 usedTokens，
				// 这里必须用累积缓冲重渲染最终助手块，确保正文与思考块都完整落屏。
				m.finalizeAssistant()
			}
			m.viewport.SetHeight(m.vpHeight())
			m.render()
			m.input.Focus()
			m.viewport.GotoBottom()
			// 轮次结束：若有排队通知且预算未耗尽，自动开启唤醒轮
			return m, m.tryWakeup()
		}
		if msg.err != nil {
			m.thinking = false
			m.streaming = false
			m.streamOpen = false
			m.clearStream()
			m.appendMessage(errorSty.Render("错误: ") + msg.err.Error())
			m.viewport.SetHeight(m.vpHeight())
			m.render()
			m.input.Focus()
			m.viewport.GotoBottom()
			return m, nil
		}
		f := msg.frame
		switch f.Status {
		case "reasoning":
			// 思考过程增量：新建/追加助手块，以暗色渲染思考文本
			m.streaming = true
			m.thinking = false
			if !m.streamOpen {
				m.openAssistantBlock()
			}
			if !m.reasoningOpen {
				m.reasoningOpen = true
				m.reasoningBuffer = ""
			}
			m.reasoningBuffer += f.Reasoning
			w := max(m.width-4, 20)
			reasoning := renderReasoning(m.reasoningBuffer, w)
			// 思考块一般先于正文到达；但部分服务端会先发 text、后发 thinking。
			// 若正文已先行输出，不要把思考块覆盖到正文上，而是作为前缀拼接到正文内容之前。
			var body string
			if m.streamBuffer != "" {
				body = joinReasoningAnswer(reasoning, renderMarkdown(m.streamBuffer, w))
			} else {
				body = reasoning
			}
			m.lines[m.streamMsgIdx] = m.renderAssistant(body)
			m.render()
			m.viewport.GotoBottom()
			return m, m.pumpStream(msg.input, msg.ch)
		case "streaming":
			m.streaming = true
			m.thinking = false
			if !m.streamOpen {
				m.openAssistantBlock()
			}
			// 从思考过渡到答案：把已渲染的思考块并入 committed，作为答案正文的前缀
			if m.reasoningOpen {
				m.reasoningOpen = false
				m.reasoningCommitted = renderReasoning(m.reasoningBuffer, max(m.width-4, 20))
				m.reasoningBuffer = ""
			}
			m.streamBuffer += f.Output
			body := joinReasoningAnswer(m.reasoningCommitted, renderMarkdown(m.streamBuffer, max(m.width-4, 20)))
			m.lines[m.streamMsgIdx] = m.renderAssistant(body)
			m.render()
			m.viewport.GotoBottom()
			return m, m.pumpStream(msg.input, msg.ch)
		case "tool":
			m.streamOpen = false
			// 工具帧随帧携带本步请求的 usage：每步工具调用后即刷新容量与运行指标
			// （对齐 REX 每步 usage 事件即时更新统计行），不必等轮末 success 帧。
			if f.Usage != nil {
				if f.Usage.TotalTokens > 0 {
					m.usedTokens = int(f.Usage.TotalTokens)
				}
				m.trackTurnUsage(f.Usage)
			}
			// 工具结果帧（ToolResult 非空）以「└」gutter 缩进展示，错误时用错误色；
			// 调用帧（ToolName 非空）以「● Verb(arg)」卡片展示；均无结构化信息时回退原文。
			toolLine := ""
			if f.ToolResult != "" {
				toolLine = renderToolResult(f.ToolResult, f.Error != "")
			} else {
				toolLine = renderToolCall(f.ToolName, f.ToolArgs)
			}
			if toolLine == "" {
				toolLine = dimSty.Render(strings.TrimSpace(f.Output))
			}
			if len(m.lines) > 0 && m.lines[len(m.lines)-1] == "\n"+toolLine {
				// 避免重複追加
			} else {
				m.appendToolFrame(toolLine)
			}
			m.render()
			m.viewport.GotoBottom()
			return m, m.pumpStream(msg.input, msg.ch)
		case "success", "error":
			m.thinking = false
			m.streaming = false
			m.streamOpen = false
			if f.Status == "success" && f.Usage != nil {
				if f.Usage.TotalTokens > 0 {
					m.usedTokens = int(f.Usage.TotalTokens)
				}
				m.trackTurnUsage(f.Usage)
			}
			if f.Status == "error" && f.Error != "" {
				m.appendMessage(errorSty.Render("错误: ") + f.Error)
			}
			// 流式收尾：无论思考/正文到达顺序如何，都以累积缓冲重渲染最终助手块，
			// 确保迟到的思考块不会覆盖掉已输出的正文。
			m.finalizeAssistant()
			m.viewport.SetHeight(m.vpHeight())
			m.render()
			m.input.Focus()
			m.viewport.GotoBottom()
			return m, nil
		}
		return m, m.pumpStream(msg.input, msg.ch)

	case injectedMsg:
		// 实时注入完成：仅失败时回显错误；成功无需动作（气泡已上游渲染）。
		if msg.err != nil {
			m.appendMessage(errorSty.Render("注入失败: ") + msg.err.Error())
			m.render()
			m.viewport.GotoBottom()
		}
		// 注入不应打断当前流；若有进行中的流式通道则继续保持泵取。
		return m, m.pumpStreamIfOpen()

	case submitResult:
		m.thinking = false
		if msg.err != nil {
			m.appendMessage(errorSty.Render("错误: ") + msg.err.Error())
		} else if msg.result != nil {
			if msg.result.Status == "success" {
				body := renderMarkdown(msg.result.Output, max(m.width-4, 20))
				m.appendMessage(m.renderAssistant(body))
			} else {
				m.appendMessage(errorSty.Render("错误: ") + msg.result.Output)
			}
		}
		m.viewport.SetHeight(m.vpHeight())
		m.render()
		m.input.Focus()
		m.viewport.GotoBottom()
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// render 将历史行切分成与被选中区域一致的宽对齐渲染行，并把当前选区反色高亮后交给 viewport。
// 内容宽度取 viewport 宽度（终端宽 -1，末列预留给自定义滚动条）。
func (m *Model) render() {
	w := m.viewport.Width()
	if w < 1 {
		w = 1
	}
	rows := m.buildWrappedLines(w)
	m.wrappedLines = rows
	if m.sel.active && !m.sel.empty() {
		start, end := m.sel.ordered()
		highlighted := make([]string, len(rows))
		for i, line := range rows {
			highlighted[i] = line
			if lo, hi, ok := selSpan(i, start, end, w); ok {
				highlighted[i] = lipgloss.StyleRanges(line, lipgloss.NewRange(lo, hi, selStyle))
			}
		}
		rows = highlighted
	}
	m.viewport.SetContent(strings.Join(rows, "\n"))
}

// buildWrappedLines 将每条语义行以固定宽度渲染，得到换行后并为宽度对齐的可视行。
// 每行恰好为宽度 w，故 viewport 对其不再折行，行号与可视列可直接映射。
func (m *Model) buildWrappedLines(w int) []string {
	var rows []string
	for _, line := range m.lines {
		rendered := lipgloss.NewStyle().Width(w).Render(line)
		rows = append(rows, strings.Split(rendered, "\n")...)
	}
	return rows
}

// selSpan 返回内容行 idx 上选区覆盖的 [lo, hi) 可视列跨度；不在选区内则返回 false。
func selSpan(idx int, start, end selPos, w int) (lo, hi int, ok bool) {
	if idx < start.line || idx > end.line {
		return 0, 0, false
	}
	lo, hi = 0, w
	if idx == start.line {
		lo = start.col
	}
	if idx == end.line {
		hi = end.col
	}
	if hi > w {
		hi = w
	}
	if lo >= hi {
		return 0, 0, false
	}
	return lo, hi, true
}

// selectedText 从宽对齐渲染行中按选区坐标提取纯文本副本。
func (m *Model) selectedText() string {
	if !m.sel.active || m.sel.empty() {
		return ""
	}
	start, end := m.sel.ordered()
	var out []string
	for idx := start.line; idx <= end.line && idx < len(m.wrappedLines); idx++ {
		lo, hi := 0, ansi.StringWidth(m.wrappedLines[idx])
		if idx == start.line {
			lo = start.col
		}
		if idx == end.line {
			hi = end.col
		}
		if lo > hi {
			lo = hi
		}
		out = append(out, strings.TrimRight(ansi.Strip(ansi.Cut(m.wrappedLines[idx], lo, hi)), " "))
	}
	return strings.Join(out, "\n")
}

// inBody 报告屏幕坐标 (x, y) 是否位于正文 viewport 区域内（viewport 占据标题行之下的区域）。
func (m *Model) inBody(x, y int) bool {
	h := m.viewport.Height()
	return y >= 1 && y < 1+h && x >= 0 && x < m.viewport.Width()
}

// transcriptCaret 把屏幕坐标映射到正文内容坐标（绝对行 + 可视列），并做边界钳制。
func (m *Model) transcriptCaret(x, y int) selPos {
	h := m.viewport.Height()
	yv := y - 1 // 正文首行位于屏幕第 1 行（标题占第 0 行）
	if yv < 0 {
		yv = 0
	}
	if yv > h-1 {
		yv = h - 1
	}
	if x < 0 {
		x = 0
	}
	if cw := m.viewport.Width(); x > cw {
		x = cw
	}
	return selPos{line: m.viewport.YOffset() + yv, col: x}
}

// navigateHistory 用 ↑/↓ 翻阅已提交命令历史。返回前会恢复用户半途编辑的草稿。
func (m *Model) navigateHistory(prev bool) {
	if prev {
		if m.histPos == len(m.history) {
			m.draft = m.input.Value()
		}
		if m.histPos > 0 {
			m.histPos--
			m.setInputFromHistory()
		}
		return
	}
	if m.histPos < len(m.history) {
		m.histPos++
		m.setInputFromHistory()
	}
}

// setInputFromHistory 将输入框内容设为 histPos 对应的历史命令；落在末尾则回到草稿。
func (m *Model) setInputFromHistory() {
	if m.histPos == len(m.history) {
		m.input.SetValue(m.draft)
	} else {
		m.input.SetValue(m.history[m.histPos])
	}
	m.input.CursorEnd()
}

// runeCount 返回字符串的字符（rune）数，用于复制提示。
func runeCount(s string) int {
	return len([]rune(s))
}

// copyNoticeMsg 是复制提示过期的一条内部消息；seq 用于忽略已经过期的陈年提示。
type copyNoticeMsg struct{ seq int }

const copyNoticeTTL = 1500 * time.Millisecond

func copyNoticeExpire(seq int) tea.Cmd {
	return tea.Tick(copyNoticeTTL, func(time.Time) tea.Msg {
		return copyNoticeMsg{seq: seq}
	})
}

// readClipboardPaste 读取系统剪贴板并通过标准的 PasteMsg 路径插入输入框。
func readClipboardPaste() tea.Cmd {
	return func() tea.Msg {
		text, err := clipboard.ReadAll()
		if err != nil {
			return nil
		}
		return tea.PasteMsg{Content: text}
	}
}

// renderUserBubble 把用户消息渲染为左对齐的紫色文本（统一缩进 + 前缀符号，不套气泡框）。
func renderUserBubble(text string, width int) string {
	maxW := width
	if maxW < 20 {
		maxW = 20
	}
	wrapped := ansi.Wrap(text, maxW, "")
	lines := strings.Split(wrapped, "\n")
	out := make([]string, len(lines))
	for i, l := range lines {
		if i == 0 {
			// 首行：统一左边距 assistantGutter + 「› 」前缀
			out[i] = userTextSty.Render(assistantGutter + "› " + l)
		} else {
			// 续行缩进对齐到「› 」文本之后
			out[i] = userTextSty.Render(assistantGutter + "  " + l)
		}
	}
	return strings.Join(out, "\n")
}

// appendMessage 追加一条已渲染消息；非首条消息前补一个空行，使消息之间留有间距。
func (m *Model) appendMessage(rendered string) {
	if len(m.lines) > 0 {
		m.lines = append(m.lines, "\n"+rendered)
		return
	}
	m.lines = append(m.lines, rendered)
}

// appendToolFrame 追加工具结果/调用帧：工具调用与结果帧之间无空行，紧凑展示。
func (m *Model) appendToolFrame(rendered string) {
	if len(m.lines) > 0 {
		m.lines = append(m.lines, "\n"+rendered)
	} else {
		m.lines = append(m.lines, rendered)
	}
}

// renderAssistant 组装助手回复：缩进身份头（✦ DSC · 模型名）+ markdown 正文。
// 头部与正文之间留一个空行，正文整体缩进 assistantGutter，营造 REX 式的呼吸版面。
func (m *Model) renderAssistant(body string) string {
	header := assistantGutter + assistantNameSty.Render(assistantMark+" DSC · "+m.displayModelName())
	if strings.TrimSpace(body) == "" {
		return header
	}
	return header + "\n\n" + indentBlock(body, assistantGutter)
}

// finalizeAssistant 根据已累积的思考/正文状态渲染助手块的最终内容，供流式收尾（success/error/done）时调用。
// 思考块可能先于正文、也可能迟于正文到达（部分服务端先发 text 后发 thinking），
// 因此这里统一用累积缓冲重新拼装，避免仅保留思考块导致正文丢失。
func (m *Model) finalizeAssistant() {
	if m.streamMsgIdx < 0 || m.streamMsgIdx >= len(m.lines) {
		return
	}
	w := max(m.width-4, 20)
	var body string
	// 思考块：已随流式过渡提交则用 committed；否则（纯思考或迟到的思考）用缓冲区渲染。
	reasoning := m.reasoningCommitted
	if reasoning == "" && m.reasoningBuffer != "" {
		reasoning = renderReasoning(m.reasoningBuffer, w)
	}
	if reasoning != "" {
		body += reasoning
	}
	if m.streamBuffer != "" {
		body = joinReasoningAnswer(body, renderMarkdown(m.streamBuffer, w))
	}
	m.lines[m.streamMsgIdx] = m.renderAssistant(body)
}

// openAssistantBlock 新建一个助手正文块：重置思考/答案累积状态并追加身份头占位。
func (m *Model) openAssistantBlock() {
	m.streamBuffer = ""
	m.reasoningBuffer = ""
	m.reasoningOpen = false
	m.reasoningCommitted = ""
	m.streamOpen = true
	header := assistantGutter + assistantNameSty.Render(assistantMark+" DSC · "+m.displayModelName())
	m.appendMessage(header + "\n")
	m.streamMsgIdx = len(m.lines) - 1
}

// renderReasoning 将思考过程原始文本渲染为暗色块：先按 markdown 渲染（加粗/列表/代码等），
// 再以「▎」标记与续行缩进区分普通答案。空思考返回空串。
func renderReasoning(raw string, width int) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if width < 8 {
		width = 8
	}
	rendered := renderMarkdown(raw, width)
	lines := strings.Split(rendered, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i == 0 {
			b.WriteString("▎ " + line)
		} else if line != "" {
			b.WriteString("  " + line)
		}
	}
	// 整体设为暗色；markdown 行内样式会以 \x1b[m 复位，须在其后重新套用弱化前缀，
	// 否则加粗/强调等样式会连带清掉整块的暗色。
	out := reasonReapply + strings.ReplaceAll(b.String(), "\x1b[m", "\x1b[m"+reasonReapply)
	// 末尾必须复位，否则开着的暗色会泄漏到紧随其后的正文，造成正文也被弱化。
	return out + "\x1b[m\n"
}

// toolVerb 把工具名映射为卡片动词（REX 式）：shell → Shell、str_replace_editor → Edit 等。
var toolVerb = map[string]string{
	"shell":              "Shell",
	"str_replace_editor": "Edit",
	"lisp_eval":          "Lisp",
	"fetch_url":          "Fetch",
	"web_search":         "Search",
	"browser_click":      "Click",
	"browser_type":       "Type",
	"browser_screenshot": "Shot",
	"read_skill":         "Read",
	"install_skill":      "Install",
	"uninstall_skill":    "Uninstall",
}

// toolArgKey 是各工具在卡片括号中展示的主参键；未收录的工具不显示括号参数。
var toolArgKey = map[string]string{
	"shell":              "command",
	"str_replace_editor": "path",
	"lisp_eval":          "expression",
	"fetch_url":          "url",
	"web_search":         "query",
	"browser_click":      "selector",
	"browser_type":       "selector",
	"browser_screenshot": "url",
	"read_skill":         "name",
	"install_skill":      "name",
	"uninstall_skill":    "name",
}

// toolDisplayName 返回工具卡片动词：已收录映射则用之，否则回退到原始工具名。
func toolDisplayName(name string) string {
	if v, ok := toolVerb[name]; ok {
		return v
	}
	return name
}

// toolArgValue 从参数 JSON 中提取工具卡片括号内展示的主参；无法解析或缺失时返回空串。
func toolArgValue(name, argsJSON string) string {
	key := toolArgKey[name]
	if key == "" || strings.TrimSpace(argsJSON) == "" {
		return ""
	}
	var m map[string]interface{}
	if json.Unmarshal([]byte(argsJSON), &m) != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.Itoa(int(x))
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// clampToolArg 把工具主参截断到最大宽度，避免超长命令占满整行。
func clampToolArg(arg string, max int) string {
	if r := []rune(arg); len(r) > max {
		return string(r[:max]) + "…"
	}
	return arg
}

// renderToolCall 以 REX 式卡片渲染工具调用：● Verb(arg)。
// 动词加粗、括号与参数用暗色，主参截断到 60 列；无法结构化时返回空串。
func renderToolCall(name, argsJSON string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}

	// 針對 str_replace_editor 工具，解析 command 和 path，顯示為 Edit(View, path) 等格式
	if name == "str_replace_editor" || strings.Contains(name, "editor") {
		var argsMap map[string]interface{}
		if err := json.Unmarshal([]byte(argsJSON), &argsMap); err == nil {
			if cmd, ok := argsMap["command"].(string); ok {
				var cmdDisplay string
				switch strings.ToLower(cmd) {
				case "view":
					cmdDisplay = "View"
				case "create":
					cmdDisplay = "Create"
				case "str_replace":
					cmdDisplay = "StrReplace"
				case "insert":
					cmdDisplay = "Insert"
				default:
					// Fallback: capitalize first letter
					runes := []rune(cmd)
					if len(runes) > 0 {
						runes[0] = rune(strings.ToUpper(string(runes[0]))[0])
					}
					cmdDisplay = string(runes)
				}

				verb := "Edit"
				head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#05A5A5")).Render(verb)

				if path, ok := argsMap["path"].(string); ok && strings.TrimSpace(path) != "" {
					clampedPath := clampToolArg(path, 60)
					// 顯示格式：Edit(View, /root/file/path)
					argStr := "(" + cmdDisplay + ", " + clampedPath + ")"
					head += dimSty.Render(argStr)
				} else {
					head += dimSty.Render("(" + cmdDisplay + ")")
				}
				return "  " + lipgloss.NewStyle().Foreground(accent).Render("●") + " " + head
			}
		}
	}

	// 默認顯示邏輯
	verb := toolDisplayName(name)
	dot := lipgloss.NewStyle().Foreground(accent).Render("●")
	head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#05A5A5")).Render(verb)
	if arg := toolArgValue(name, argsJSON); arg != "" {
		// 参数本身已是带括号的表达式（如 Lisp 的 (+ 1 2)）时不再叠加括号，
		// 避免出现 ● Lisp((+ 1 2)) 的双括号；否则按 ● Verb(arg) 包裹展示。
		if strings.HasPrefix(arg, "(") && strings.HasSuffix(arg, ")") {
			head += dimSty.Render(clampToolArg(arg, 60))
		} else {
			head += dimSty.Render("(" + clampToolArg(arg, 60) + ")")
		}
	}
	return "  " + dot + " " + head
}

// connector 结果块首行的 gutter：2 空格 + └ + 1 空格 = 4 列，
// 与调用卡片正文同列（卡片正文从第 4 列起），保证上下对齐。
// 不用 REX 的 ⎿（U+23BF 在多数终端字形不规整、视觉上对不齐），改用标准框线 └。
const connector = "  └ "

// renderToolResult 以 REX 式 gutter 渲染工具结果：首行「└ 首行」，后续行缩进对齐下方。
// isErr 时整块用错误色强调。空结果返回空串。
func renderToolResult(result string, isErr bool) string {
	result = strings.TrimSpace(result)
	if result == "" {
		return ""
	}

	// 嘗試將結果格式化為 JSON（如果它是有效的 JSON 且不是錯誤）
	formattedResult := result
	if !isErr {
		if isJSON(result) {
			var prettyJSON bytes.Buffer
			err := json.Indent(&prettyJSON, []byte(result), "", "  ")
			if err == nil {
				formattedResult = prettyJSON.String()
			}
		}
	}

	lines := strings.Split(formattedResult, "\n")
	indent := strings.Repeat(" ", len(connector))
	var b strings.Builder
	b.WriteString(dimSty.Render(connector))
	if isErr {
		b.WriteString(errorSty.Render(lines[0]))
	} else {
		b.WriteString(lines[0])
	}
	for _, ln := range lines[1:] {
		b.WriteString("\n" + indent)
		if strings.TrimSpace(ln) == "" {
			continue
		}
		if isErr {
			b.WriteString(errorSty.Render(ln))
		} else {
			b.WriteString(ln)
		}
	}
	return b.String()
}

// isJSON 檢查給定字符串是否是有效的 JSON。
func isJSON(s string) bool {
	var js map[string]any
	return json.Unmarshal([]byte(s), &js) == nil
}

// 内置斜杆命令列表（当前为宿主可直接执行的命令）。
var slashCommands = []compItem{
	{label: "/help", insert: "/help", hint: "显示帮助与快捷键"},
	{label: "/clear", insert: "/clear", hint: "清空聊天记录"},
	{label: "/skills", insert: "/skills", hint: "列出所有已安装的技能"},
	{label: "/mode minimal", insert: "/mode minimal", hint: "切换至极简模式"},
	{label: "/mode standard", insert: "/mode standard", hint: "切换至标准模式"},
	{label: "/mode creation", insert: "/mode creation", hint: "切换至创造模式（可经 tool-lua-host 创造 LUA 插件）"},
	{label: "/workspace on", insert: "/workspace on", hint: "启用工作空间机制保护"},
	{label: "/workspace off", insert: "/workspace off", hint: "关闭工作空间机制保护"},
	{label: "/sandbox on", insert: "/sandbox on", hint: "启用沙箱保护（只读，拒绝一切文件写）"},
	{label: "/sandbox off", insert: "/sandbox off", hint: "关闭沙箱保护（不额外拦截）"},
	{label: "/sessions", insert: "/sessions", hint: "列出所有会话"},
	{label: "/crons", insert: "/crons", hint: "列出所有定时任务"},
	{label: "/cron add", insert: "/cron add ", hint: "添加定时任务（如 /cron add \"0 8 * * *\" 写日报）"},
	{label: "/cron remove", insert: "/cron remove ", hint: "删除定时任务"},
	{label: "/cron on", insert: "/cron on ", hint: "启用定时任务"},
	{label: "/cron off", insert: "/cron off ", hint: "停用定时任务"},
	{label: "/plan", insert: "/plan", hint: "进入 plan 模式（先探索设计，再经 exit_plan_mode 呈现计划）"},
	{label: "/plan off", insert: "/plan off", hint: "退出 plan 模式"},
	{label: "/session new", insert: "/session new", hint: "新建会话并切换"},
	{label: "/session default", insert: "/session default", hint: "切换到指定会话（如 /session session-3）"},
	{label: "/session delete", insert: "/session delete ", hint: "删除指定会话（如 /session delete session-3）"},
	{label: "/export", insert: "/export", hint: "导出当前会话为 Markdown 文件"},
	{label: "/mouse", insert: "/mouse", hint: "切换鼠标捕获（释放鼠标以使用终端原生复制）"},
	{label: "/exit", insert: "/exit", hint: "退出聊天"},
}

// readSkillFrontmatter 解析 SKILL.md 可选 frontmatter 的 name/description。
func readSkillFrontmatter(path, fallback string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	content := string(data)
	name, desc := fallback, ""
	if strings.HasPrefix(content, "---") {
		rest := strings.TrimPrefix(content, "---")
		if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		if end := strings.Index(rest, "\n---"); end > 0 {
			var fm struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err == nil {
				if strings.TrimSpace(fm.Name) != "" {
					name = strings.TrimSpace(fm.Name)
				}
				desc = strings.TrimSpace(fm.Description)
			}
		}
	}
	if desc == "" {
		desc = "(no description)"
	}
	return name, desc
}

// getExecutableDir 獲取可執行文件所在目錄的絕對路徑
func getExecutableDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// listSkills 扫描 skills 目录，按内置（builtin/）与外置（installed/）分组返回展示行。
func listSkills() []string {
	dir := os.Getenv("DSC_SKILLS_DIR")
	if dir == "" {
		// 使用程序所在的目录，而不是当前工作目录
		exeDir, err := getExecutableDir()
		if err != nil {
			exeDir = "."
		}
		dir = filepath.Join(exeDir, "skills")
	}
	var out []string
	if builtin := scanSkillSection(filepath.Join(dir, "builtin")); len(builtin) > 0 {
		out = append(out, "内置技能（"+fmt.Sprint(len(builtin))+"）：")
		out = append(out, builtin...)
	}
	if installed := scanSkillSection(filepath.Join(dir, "installed")); len(installed) > 0 {
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, "外置技能（"+fmt.Sprint(len(installed))+"）：")
		out = append(out, installed...)
	}
	return out
}

// scanSkillSection 扫描单个技能子目录（builtin/ 或 installed/），返回 "  - name — desc" 行。
func scanSkillSection(dir string) []string {
	var out []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() {
			// 目录布局：<name>/SKILL.md
			p := filepath.Join(dir, e.Name(), "SKILL.md")
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				if name, desc := readSkillFrontmatter(p, e.Name()); name != "" {
					out = append(out, fmt.Sprintf("  - %s — %s", name, desc))
				}
			}
		} else if strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if n, desc := readSkillFrontmatter(filepath.Join(dir, e.Name()), name); n != "" {
				out = append(out, fmt.Sprintf("  - %s — %s", n, desc))
			}
		}
	}
	sort.Strings(out)
	return out
}

// runSlashCommand 处理斜杆命令；返回是否已处理以及要执行的命令。
func (m *Model) runSlashCommand(cmd string) (bool, tea.Cmd) {
	switch cmd {
	case "/help":
		help := strings.Join([]string{
			"快捷键:",
			"  Enter        发送消息",
			"  Ctrl+J       换行（终端协议不区分 Ctrl+Enter 与 Enter，故用 LF 键）",
			"  ↑/↓          翻阅历史命令（单行输入时）",
			"  Ctrl+V       粘贴剪贴板内容",
			"  /            在输入框首字符唤起命令菜单",
			"  Ctrl+C       有选区复制 / 运行中中断 / 否则清空输入",
			"  Ctrl+Q       退出",
			"",
			"鼠标:",
			"  在正文区按住左键拖拽即可选中文字，松开自动复制到剪贴板；滚轮滚动消息；输入框区域不可选中。",
			"  /mouse 可释放鼠标给终端（终端原生选中/复制，模型工作时也可用）；状态栏会显示当前状态。",
			"",
			"斜杆命令:",
			"  /help        显示本帮助",
			"  /clear       清空聊天记录",
			"  /skills      列出所有已安装的技能",
			"  /mode minimal   切换至极简模式",
			"  /mode standard  切换至标准模式",
			"  /mode creation  切换至创造模式（可经 tool-lua-host 创造 LUA 插件，lua-plugin-creator 技能提供指导）",
			"  /workspace on   启用工作空间机制保护（限制文件操作在工作区目录内）",
			"  /workspace off  关闭工作空间机制保护（允许模型访问整个文件系统而不作限制）",
			"  /sandbox on   启用沙箱保护（只读策略，拒绝一切文件写操作）",
			"  /sandbox off  关闭沙箱保护（full 策略，不再额外拦截文件写）",
			"  /sessions    列出所有会话",
			"  /crons       列出所有定时任务",
			"  /cron add <cron> <prompt>  添加定时任务（cron 为 5 段表达式，如 0 8 * * *）",
			"  /cron remove <id>  删除定时任务",
			"  /cron on|off <id>  启用/停用定时任务",
			"  /plan       进入 plan 模式（先探索与设计，再经 exit_plan_mode 呈现完整计划）",
			"  /plan off   退出 plan 模式",
			"  /session <id>  切换到指定会话（如 /session session-3）",
			"  /session new  新建会话并切换",
			"  /session delete <id>  删除指定会话",
			"  /export    导出当前会话为 Markdown 文件",
			"  /exit        退出聊天",
		}, "\n")
		m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 帮助") + "\n" + help)
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/clear":
		m.lines = nil
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		return true, nil
	case "/skills":
		skills := listSkills()
		if len(skills) == 0 {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 技能") + "\n尚未安装任何技能。可让模型调用 install_skill 安装，或将 SKILL.md 放入 ./skills/installed 目录。")
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 技能") + "\n" + strings.Join(skills, "\n"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/mode minimal":
		if m.manager != nil {
			err := m.manager.SwitchMode("minimal")
			if err != nil {
				m.appendMessage(errorSty.Render("切換模式失敗: ") + err.Error())
			} else {
				m.mode = "minimal" // 實時反映標題欄模式
				err := plugin.UpdateMode("minimal", plugin.ConfigPath)
				if err != nil {
					m.appendMessage(errorSty.Render("保存配置失敗: ") + err.Error())
				} else {
					m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 模式切換") + "\n已切換至極簡模式 (minimal)。")
				}
			}
		} else {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/mode standard":
		if m.manager != nil {
			err := m.manager.SwitchMode("standard")
			if err != nil {
				m.appendMessage(errorSty.Render("切換模式失敗: ") + err.Error())
			} else {
				m.mode = "standard" // 實時反映標題欄模式
				err := plugin.UpdateMode("standard", plugin.ConfigPath)
				if err != nil {
					m.appendMessage(errorSty.Render("保存配置失敗: ") + err.Error())
				} else {
					m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 模式切換") + "\n已切換至標準模式 (standard)。")
				}
			}
		} else {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/mode creation":
		if m.manager != nil {
			err := m.manager.SwitchMode("creation")
			if err != nil {
				m.appendMessage(errorSty.Render("切換模式失敗: ") + err.Error())
			} else {
				m.mode = "creation" // 實時反映標題欄模式
				err := plugin.UpdateMode("creation", plugin.ConfigPath)
				if err != nil {
					m.appendMessage(errorSty.Render("保存配置失敗: ") + err.Error())
				} else {
					m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 模式切換") + "\n已切換至創造模式 (creation)：可經 tool-lua-host 編寫 LUA 插件（參考 lua-plugin-creator 技能）。")
				}
			}
		} else {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/workspace on":
		plugin.WorkspaceProtectionEnabled = true
		err := plugin.UpdateWorkspaceProtectionEnabled(true, plugin.ConfigPath)
		if err != nil {
			m.appendMessage(errorSty.Render("保存配置失敗: ") + err.Error())
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 工作空間保護") + "\n已啟用工作空間機制保護（限制文件操作在工作區目錄內）。")
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/workspace off":
		plugin.WorkspaceProtectionEnabled = false
		err := plugin.UpdateWorkspaceProtectionEnabled(false, plugin.ConfigPath)
		if err != nil {
			m.appendMessage(errorSty.Render("保存配置失敗: ") + err.Error())
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 工作空間保護") + "\n已關閉工作空間機制保護（允許模型訪問整個文件系統而不作限制）。")
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/sandbox on":
		if m.manager != nil {
			m.manager.SetSandboxPolicy(plugin.SandboxReadOnly)
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 沙箱") + "\n已启用沙箱保护（只读策略，拒绝一切文件写操作）。")
		} else {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/sandbox off":
		if m.manager != nil {
			m.manager.SetSandboxPolicy(plugin.SandboxFullAccess)
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 沙箱") + "\n已关闭沙箱保护（full 策略，不再额外拦截文件写）。")
		} else {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/sessions":
		if m.manager != nil {
			summaries, err := m.manager.ListSessions()
			if err != nil {
				m.appendMessage(errorSty.Render("列出会话失败: ") + err.Error())
			} else if len(summaries) == 0 {
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 会话") + "\n（暂无会话）")
			} else {
				var b strings.Builder
				b.WriteString(assistantNameSty.Render(assistantMark + " DSC · 会话列表\n"))
				for _, s := range summaries {
					fmt.Fprintf(&b, "  %s · %d 事件", s.ID, s.Events)
					if s.Preview != "" {
						b.WriteString(" · " + s.Preview)
					}
					b.WriteString("\n")
				}
				b.WriteString("切换: /session <id>（如 /session session-3）")
				m.appendMessage(b.String())
			}
		} else {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/plan", "/plan on":
		if err := m.agent.SetPlanMode(m.ctx, true); err != nil {
			m.appendMessage(errorSty.Render("进入 plan 模式失败: ") + err.Error())
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · Plan") + "\n已进入 plan 模式：先探索与设计，再经 exit_plan_mode 呈现完整计划。")
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/plan off":
		if err := m.agent.SetPlanMode(m.ctx, false); err != nil {
			m.appendMessage(errorSty.Render("退出 plan 模式失败: ") + err.Error())
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · Plan") + "\n已退出 plan 模式。")
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/session":
		m.appendMessage(errorSty.Render("用法: /session <会话 id>，如 /session session-3 或 /session default"))
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/export":
		if m.manager == nil {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		} else {
			path, err := m.manager.ExportSession(m.currentSessionID)
			if err != nil {
				m.appendMessage(errorSty.Render("导出会话失败: ") + err.Error())
			} else {
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 导出") + fmt.Sprintf("\n已导出会话 %s 到 %s", m.currentSessionID, path))
			}
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/crons":
		if m.manager == nil {
			m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 定时任务") + "\n" + m.renderCrons())
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/cron":
		m.appendMessage(errorSty.Render("用法: /cron add <cron(5 段)> <prompt>，/cron remove <id>，/cron on|off <id>"))
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/mouse":
		m.mouseCaptureOff = !m.mouseCaptureOff
		if m.mouseCaptureOff {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 鼠标") + "\n已释放鼠标给终端：终端原生文字选中/复制可用（模型工作期间也可用）；应用内滚轮滚动与拖选复制暂停。")
		} else {
			m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 鼠标") + "\n已恢复应用内鼠标捕获：滚轮滚动与正文拖选复制生效。")
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	case "/quit", "/exit":
		return true, tea.Quit
	}
	// /cron add|remove|on|off（前缀匹配）
	if strings.HasPrefix(cmd, "/cron ") {
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "/cron "))
		switch {
		case strings.HasPrefix(rest, "add "):
			tokens := strings.Fields(strings.TrimPrefix(rest, "add "))
			if len(tokens) < 6 {
				m.appendMessage(errorSty.Render("用法: /cron add <cron> <prompt>，如 /cron add \"0 8 * * *\" 写今日日报"))
			} else if m.manager == nil {
				m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
			} else {
				j := &cron.Job{
					Name:    "cron-" + strconv.FormatInt(time.Now().UnixMilli(), 10),
					Cron:    strings.Join(tokens[:5], " "),
					Prompt:  strings.Join(tokens[5:], " "),
					Enabled: true,
				}
				if err := m.manager.AddCronJob(j); err != nil {
					m.appendMessage(errorSty.Render("添加任务失败: ") + err.Error())
				} else {
					m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 定时任务") + fmt.Sprintf("\n已添加任务 %s（cron: %s）。", j.ID, j.Cron))
				}
			}
		case strings.HasPrefix(rest, "remove "):
			id := strings.TrimSpace(strings.TrimPrefix(rest, "remove "))
			if id == "" {
				m.appendMessage(errorSty.Render("用法: /cron remove <任务 id>"))
			} else if m.manager == nil {
				m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
			} else if err := m.manager.RemoveCronJob(id); err != nil {
				m.appendMessage(errorSty.Render("删除任务失败: ") + err.Error())
			} else {
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 定时任务") + fmt.Sprintf("\n已删除任务 %s。", id))
			}
		case strings.HasPrefix(rest, "on ") || strings.HasPrefix(rest, "off "):
			enabled := strings.HasPrefix(rest, "on ")
			id := strings.TrimSpace(strings.TrimPrefix(rest, "on "))
			if !enabled {
				id = strings.TrimSpace(strings.TrimPrefix(rest, "off "))
			}
			if id == "" {
				m.appendMessage(errorSty.Render("用法: /cron on|off <任务 id>"))
			} else if m.manager == nil {
				m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
			} else if err := m.manager.SetCronJobEnabled(id, enabled); err != nil {
				m.appendMessage(errorSty.Render("切换任务状态失败: ") + err.Error())
			} else if enabled {
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 定时任务") + fmt.Sprintf("\n已启用任务 %s。", id))
			} else {
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 定时任务") + fmt.Sprintf("\n已停用任务 %s。", id))
			}
		default:
			m.appendMessage(errorSty.Render("用法: /cron add <cron> <prompt>，/cron remove <id>，/cron on|off <id>"))
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	}
	// /session <id> 切换 / new 新建 / delete <id> 删除（前缀匹配）
	if strings.HasPrefix(cmd, "/session ") {
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "/session "))
		switch {
		case rest == "new":
			if m.manager == nil {
				m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
			} else {
				id, err := m.manager.CreateSession()
				if err != nil {
					m.appendMessage(errorSty.Render("新建会话失败: ") + err.Error())
				} else if err := m.agent.SwitchSession(m.ctx, id); err != nil {
					m.appendMessage(errorSty.Render("切换会话失败: ") + err.Error())
				} else {
					m.currentSessionID = id
					m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 会话") + fmt.Sprintf("\n已新建并切换到会话 %s。", id))
				}
			}
		case strings.HasPrefix(rest, "delete "):
			id := strings.TrimSpace(strings.TrimPrefix(rest, "delete "))
			if id == "" {
				m.appendMessage(errorSty.Render("用法: /session delete <会话 id>"))
			} else if m.manager == nil {
				m.appendMessage(errorSty.Render("錯誤: 插件管理器不可用"))
			} else if err := m.manager.DeleteSession(id); err != nil {
				m.appendMessage(errorSty.Render("删除会话失败: ") + err.Error())
			} else {
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 会话") + fmt.Sprintf("\n已删除会话 %s。", id))
			}
		default:
			if rest == "" {
				m.appendMessage(errorSty.Render("用法: /session <会话 id>，或 /session new，或 /session delete <id>"))
			} else if err := m.agent.SwitchSession(m.ctx, rest); err != nil {
				m.appendMessage(errorSty.Render("切换会话失败: ") + err.Error())
			} else {
				m.currentSessionID = rest
				m.appendMessage(assistantNameSty.Render(assistantMark+" DSC · 会话") + fmt.Sprintf("\n已切换到会话 %s。", rest))
			}
		}
		m.input.SetValue("")
		m.completion = completion{}
		m.syncInputHeight()
		m.render()
		m.viewport.GotoBottom()
		return true, nil
	}
	return false, nil
}

// renderCrons 渲染定时任务列表（/crons 命令）。
func (m *Model) renderCrons() string {
	list := m.manager.ListCronJobs()
	if len(list) == 0 {
		return "（暂无定时任务，可用 /cron add <cron> <prompt> 添加）"
	}
	var b strings.Builder
	for _, j := range list {
		state := "停用"
		if j.Enabled {
			state = "启用"
		}
		last := "-"
		if j.LastStatus != "" {
			last = j.LastStatus
			if j.LastRunAt > 0 {
				last += " @" + time.UnixMilli(j.LastRunAt).Format("15:04")
			}
		}
		fmt.Fprintf(&b, "  %s  %s  [%s]  cron=%s  上次=%s\n", j.ID, j.Name, state, j.Cron, last)
	}
	return b.String()
}

// updateCompletion 根据当前输入重新计算命令补全菜单：
// 仅在输入是「/」开头的单个词（不含空格）时弹出，并按前缀/子序列过滤。
func (m *Model) updateCompletion() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") || strings.ContainsAny(val, " \t\n") {
		m.completion = completion{}
		// 更新 viewport 高度以移除補全菜單佔用的空間
		m.viewport.SetHeight(m.vpHeight())
		return
	}
	items := filterSlash(slashCommands, val)
	if len(items) == 0 {
		m.completion = completion{}
		// 更新 viewport 高度以移除補全菜單佔用的空間
		m.viewport.SetHeight(m.vpHeight())
		return
	}
	m.completion = completion{active: true, items: items}
	// 更新 viewport 高度以適應補全菜單
	m.viewport.SetHeight(m.vpHeight())
}

// filterSlash 按大小写不敏感的前缀或子序列过滤命令项。
func filterSlash(items []compItem, query string) []compItem {
	if query == "" {
		return items
	}
	lq := strings.ToLower(query)
	var out []compItem
	for _, it := range items {
		l := strings.ToLower(it.label)
		if strings.HasPrefix(l, lq) || subsequenceMatch(l, lq) {
			out = append(out, it)
		}
	}
	return out
}

// subsequenceMatch 判断 query 是否以子序列形式出现在 target 中（顺序一致、不必连续）。
func subsequenceMatch(target, query string) bool {
	if query == "" {
		return true
	}
	qr := []rune(query)
	ti := 0
	for _, r := range target {
		if r == qr[ti] {
			ti++
			if ti == len(qr) {
				return true
			}
		}
	}
	return false
}

// moveCompletion 上下移动菜单选择（循环）。
func (m *Model) moveCompletion(delta int) {
	n := len(m.completion.items)
	if n == 0 {
		return
	}
	m.completion.sel = ((m.completion.sel+delta)%n + n) % n
}

// acceptCompletion 用选中的命令填充输入框，并重新过滤菜单。
func (m *Model) acceptCompletion() {
	if m.completion.sel >= len(m.completion.items) {
		m.completion = completion{}
		return
	}
	it := m.completion.items[m.completion.sel]
	m.input.SetValue(it.insert)
	m.input.CursorEnd()
	m.updateCompletion()
}

// completionExactLabel 报告当前输入是否与选中项的命令完全一致（此时 Enter 应直接执行）。
func (m *Model) completionExactLabel() bool {
	if !m.completion.active || m.completion.sel >= len(m.completion.items) {
		return false
	}
	return strings.TrimSpace(m.input.Value()) == m.completion.items[m.completion.sel].label
}

// completionView 渲染命令补全菜单，显示在输入框上方。
func (m *Model) completionView() string {
	if !m.completion.active || len(m.completion.items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, it := range m.completion.items {
		var line string
		if i == m.completion.sel {
			line = "› " + compSelSty.Render(it.label)
		} else {
			line = "  " + it.label
		}
		if it.hint != "" {
			line += dimSty.Render("  " + it.hint)
		}
		b.WriteString(padToWidth(line, m.width))
		b.WriteByte('\n')
	}
	b.WriteString(padToWidth(dimSty.Render("↑/↓ 选择 · Tab 补全 · Esc 关闭"), m.width))
	return b.String()
}

// padToWidth 用 NBSP 把行补到固定宽度，避免终端擦除后残留幽灵字符。
func padToWidth(s string, w int) string {
	pad := w - ansi.StringWidth(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat("\u00a0", pad)
}

// displayMode 返回展示用预设模式名（首字母大写；未知时返回原值）。
func (m *Model) displayMode() string {
	switch m.mode {
	case "minimal":
		return "Minimal"
	case "standard":
		return "Standard"
	case "creation":
		return "Creation"
	}
	return m.mode
}

// shortTokens 把 token 数格式化为以 1024 为底的短单位（如 128K），小于 1024 显示原数。
func shortTokens(n int) string {
	switch {
	case n >= 999*1024:
		return fmt.Sprintf("%.1fM", float64(n)/1024/1024)
	case n >= 1024:
		return fmt.Sprintf("%dK", n/1024)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// capacityTag 渲染「已用/总容量」，如 12K/128K；容量未设置时只显示已用。
func (m *Model) capacityTag() string {
	if m.contextWindow > 0 {
		return fmt.Sprintf("%s/%s", shortTokens(m.usedTokens), shortTokens(m.contextWindow))
	}
	return shortTokens(m.usedTokens)
}

// composerView 渲染输入区：每行正文 pad 到「宽度-1」（输入框左侧 1 格内边距），
// 使上下边框线延伸到终端全宽，避免右侧净余空白。
func (m *Model) composerView() string {
	inner := strings.TrimRight(m.input.View(), "\n")
	w := max(m.width-1, 10) // padding-left 占 1 列
	var b strings.Builder
	for i, line := range strings.Split(inner, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if pad := w - ansi.StringWidth(line); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
	}
	return composerBoxSty.Render(b.String())
}

// runInfoLine 渲染输入框与状态栏之间的会话指标行：轮次 + 步数 + 已用容量。
// 填在两条平行线（输入框下边框与状态栏分隔线）之间，避免双线紧贴，参考 REX 的指标带布局。
// 轮/步编号对齐 DSH 定义：轮为一次受理输入的排空，步为一次模型请求及其引发的工具执行。
func (m *Model) runInfoLine() string {
	info := fmt.Sprintf("轮次 %d · 步数 %d", m.curTurn, m.curStep)
	if m.usedTokens > 0 {
		if m.contextWindow > 0 {
			// 已知总容量时显示已用百分比；小于 1% 也至少显示 1，避免 0% 误导
			pct := m.usedTokens * 100 / m.contextWindow
			if pct < 1 {
				pct = 1
			}
			info += fmt.Sprintf(" · 已用 %d%%", pct)
		} else {
			info += " · 已用 " + shortTokens(m.usedTokens)
		}
	}
	// prompt 缓存命中率（对齐 REX cacheTag）：仅当服务端报告了缓存字段时显示
	if hit, miss := int64(m.cacheHit), int64(m.cacheMiss); hit+miss > 0 {
		info += fmt.Sprintf(" · 缓存命中 %d%%", hit*100/(hit+miss))
	}
	return "  " + dimSty.Render(info)
}

// trackTurnUsage 累计当前轮的运行指标：下行生成 token 与 prompt 缓存命中/写入。
func (m *Model) trackTurnUsage(u *plugin.Usage) {
	if u == nil {
		return
	}
	if u.CompletionTokens > 0 {
		m.turnTokens += int(u.CompletionTokens)
	}
	if u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
		m.cacheHit = u.CacheReadInputTokens
		m.cacheMiss = u.CacheCreationInputTokens
	}
}

// elapsedTickMsg 每秒触发一次，驱动「思考中」行耗时刷新（对齐 REX elapsed tick）。
type elapsedTickMsg struct{}

func elapsedTick() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg { return elapsedTickMsg{} })
}

// statusBar 渲染底部状态栏（两行）：首行一条通栏分隔线，次行容纳模型/复制提示与快捷键。
// 采用 REX 风格的缩进 + 分隔线布局，把交互信息与模型信息区分开，营造呼吸感。
func (m *Model) statusBar() string {
	divider := dividerSty.Render(strings.Repeat("─", m.width))
	left := "模型: " + m.displayModelName()
	if m.copyNotice != "" {
		left += " · " + m.copyNotice
	}
	left = "  " + left
	if m.mouseCaptureOff {
		left += " · " + dimSty.Render("鼠标已释放(/mouse 恢复)")
	}
	right := "Enter 发送 · 选中复制 · Ctrl+J 换行 · Ctrl+Q 退出"
	pad := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if pad < 1 {
		pad = 1
	}
	return divider + "\n" + dimSty.Render(left+strings.Repeat(" ", pad)+right)
}

// View 渲染视图
func (m *Model) View() tea.View {
	if !m.ready {
		return m.viewOf("加载中...")
	}

	title := titleSty.Render(" ◆ DSC  |  " + m.displayMode() + "  |  " + m.capacityTag() + " ")
	title = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, title)

	var parts []string
	parts = append(parts, title)
	parts = append(parts, m.viewportView())
	if q := m.questionView(); q != "" {
		parts = append(parts, q)
	}
	if m.thinking || m.streaming {
		line := "  " + m.spinner.View() + fmt.Sprintf(" 思考中... (%d 秒 · Ctrl+C 取消)", m.elapsed)
		if m.turnTokens > 0 {
			line += " · ↓" + shortTokens(m.turnTokens)
		}
		parts = append(parts, dimSty.Render(line))
	}
	if c := m.completionView(); c != "" {
		parts = append(parts, c)
	}
	parts = append(parts, m.composerView())
	parts = append(parts, m.runInfoLine())
	parts = append(parts, m.statusBar())
	return m.viewOf(strings.Join(parts, "\n"))
}

// viewOf 把内容包装成视图并声明终端特性：进入备用屏幕并保持鼠标捕获。
// 鼠标捕获开启（CellMotion）时滚轮滚动、正文拖拽选中自动复制内建工作；
// /mouse 或 DSC_DISABLE_MOUSE 可释放鼠标给终端（MouseModeNone），恢复终端
// 原生文字选中/复制（模型工作期间同样可用）。
// 真实光标由 View 显式锚定到输入插入点：SetVirtualCursor(false) 后 textarea
// 不再渲染虚拟光标，若不在此给出位置，输入框光标会丢失。
func (m *Model) viewOf(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	if m.mouseCaptureOff {
		v.MouseMode = tea.MouseModeNone
	} else {
		v.MouseMode = tea.MouseModeCellMotion
	}
	if cur := m.inputCursorAbs(); cur != nil {
		v.Cursor = cur
	}
	return v
}

// inputCursorAbs 计算 textarea 真实光标在整屏中的绝对坐标（0-based）。
// textarea.Cursor() 返回相对输入框视图的坐标（已含 prompt 宽度与 textarea
// 自身的边框/内边距）；这里叠加外层 composer 的左侧 padding 与上方各部件行数
// （标题、viewport、问题/思考/补全行），再经 viewOf 赋给 v.Cursor。
func (m *Model) inputCursorAbs() *tea.Cursor {
	if !m.ready {
		return nil
	}
	cur := m.input.Cursor()
	if cur == nil {
		return nil
	}
	// X：外层 composer 左侧 padding 1 列（无左边框线）
	cur.X += 1
	// Y：标题(1) + viewport + 中间各部件 + composer 顶边框(1)
	y := titleRows + m.viewport.Height()
	if q := m.questionView(); q != "" {
		y += strings.Count(q, "\n") + 1
	}
	if m.thinking || m.streaming {
		y += thinkingRow
	}
	if c := m.completionView(); c != "" {
		y += strings.Count(c, "\n") + 1
	}
	cur.Y += y + 1 // +1 为 composer 顶边框行
	return cur
}

// Run 运行聊天界面，阻塞直到退出
func Run(agent plugin.Agent, manager *plugin.Manager, ctx context.Context, modelName, mode string, contextWindow int) error {
	m := New(agent, manager, ctx, modelName, mode, contextWindow)
	p := tea.NewProgram(m)
	m.program = p
	// 注册用户评审 provider：agent 侧 exit_plan_mode 等经宿主向 TUI 提问
	if manager != nil {
		if err := manager.RegisterUserQuestionProvider(m.askProvider); err != nil {
			// 已有 provider（重复注册）仅记录，不阻塞启动
			_ = err
		}
		// 后台任务完成 → 宿主事件总线 → 通知唤醒（对齐 DSH completionDelivery: wakeup）
		manager.OnEvent(plugin.JobDoneEvent, func(ctx plugin.EventContext) (any, error) {
			if s, ok := ctx.Data.(jobs.JobSnapshot); ok {
				m.program.Send(jobDoneMsg{snapshot: s})
			}
			return nil, nil
		})
	}
	_, err := p.Run()
	return err
}
