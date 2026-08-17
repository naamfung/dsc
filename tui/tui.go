// Package tui 提供基于 Bubble Tea 的终端聊天界面。
// 该界面运行在宿主进程中（不通过 go-plugin 子进程），
// 因为 TUI 需要直接操作终端 raw mode 和 stdout，而插件子进程的 stdout 会被 go-plugin 捕获。
package tui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"dsc/plugin"
	"github.com/charmbracelet/x/ansi"
)

// 布局尺寸常量：标题栏、状态栏各占一行；输入区内部 3 行 + 2 行边框。
const (
	titleRows   = 1
	statusRows  = 1
	composerMin = 3
	boxBorder   = 2
	thinkingRow = 1
)

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

	errorSty = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF5F87")).
		Bold(true)

	composerBoxSty = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1)

	compSelSty = lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
)

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

// compItem 是斜杠命令菜单的一行：label 是完整命令，hint 是右侧提示。
type compItem struct {
	label  string
	insert string
	hint   string
}

// completion 是斜杠命令补全菜单状态；active 为 false 时菜单关闭。
type completion struct {
	active bool
	items  []compItem
	sel    int
}

// Model 是聊天界面的状态模型
type Model struct {
	agent     plugin.Agent
	ctx       context.Context
	modelName string

	viewport viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	ready    bool // viewport 在收到首个窗口尺寸后初始化
	thinking bool // 正在等待 agent 响应
	streaming bool // 正在流式输出

	completion completion // 斜杠命令补全菜单

	lines []string // 渲染后的历史行
	width int
	high  int

	// 流式渲染状态
	streamBuffer string
	streamOpen   bool
	streamMsgIdx int
}

// New 创建一个聊天界面模型
func New(agent plugin.Agent, ctx context.Context, modelName string) *Model {
	input := textarea.New()
	input.Placeholder = "输入消息，回车发送，Ctrl+J 换行，Ctrl+C 退出"
	input.CharLimit = 4096
	input.ShowLineNumbers = false
	input.SetHeight(1)
	// 仅首行显示提示符「❯ 」，续行留空；高度随内容行数变化（见 syncInputHeight）
	input.SetPromptFunc(2, func(info textarea.PromptInfo) string {
		if info.LineNumber != 0 {
			return ""
		}
		return "❯ "
	})
	// 回车在宿主层拦截用于发送，这里仅兜底新增换行键位。
	// 终端协议里 Enter/Ctrl+Enter 都发送相同的 CR 字节（Bubble Tea v1 无法区分），
	// Alt+Enter 又被 Windows Terminal 占用为全屏切换，因此统一用 Ctrl+J（LF）作为换行键。
	input.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+j"),
		key.WithHelp("ctrl+j", "换行"),
	)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(accent)

	return &Model{
		agent:     agent,
		ctx:       ctx,
		modelName: modelName,
		input:     input,
		spinner:   s,
	}
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
		ch, err := m.agent.RunStream(m.ctx, input)
		if err != nil {
			return streamFrame{input: input, err: err, done: true}
		}
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

// vpHeight 计算消息区高度：总高度减去标题、状态栏、输入区（含边框），思考时再减去指示行。
// 输入区高度随内容行数变化（1~composerMin 行），这里以当前实际高度为准。
func (m *Model) vpHeight() int {
	h := m.high - titleRows - statusRows - boxBorder - m.input.Height()
	if m.thinking {
		h -= thinkingRow
	}
	if h < 3 {
		h = 3
	}
	return h
}

// syncInputHeight 让输入框高度跟随内容行数：空/单行时 1 行，最多 composerMin 行。
// 高度变化会占用/释放一行的消息区，需同步重算 viewport 高度并滚到底部。
func (m *Model) syncInputHeight() {
	n := m.input.LineCount()
	if n < 1 {
		n = 1
	}
	if n > composerMin {
		n = composerMin
	}
	if n != m.input.Height() {
		m.input.SetHeight(n)
		m.viewport.SetHeight(m.vpHeight())
		m.viewport.GotoBottom()
	}
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
			m.viewport = viewport.New(viewport.WithWidth(msg.Width), viewport.WithHeight(m.vpHeight()))
			m.viewport.YPosition = 0
			m.ready = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(m.vpHeight())
		}
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

	case tea.KeyPressMsg:
		if m.thinking || m.streaming {
			// 响应/流式输出期间不处理输入，但允许 Ctrl+C 退出
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		// 命令补全菜单打开时，导航/补全键由菜单接管
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
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if handled, cmd := m.runSlashCommand(text); handled {
				return m, cmd
			}
			if text == "" {
				return m, nil
			}
			m.appendMessage(renderUserBubble(text, m.width-4))
			m.input.SetValue("")
			m.completion = completion{}
			m.thinking = true
			m.syncInputHeight() // 输入清空回落单行 + 思考行占位 → 重算消息区高度
			m.render()
			m.viewport.GotoBottom()
			return m, tea.Batch(
				m.submitCmd(text),
				m.spinner.Tick,
			)
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.syncInputHeight() // 新增/删除换行后同步输入框高度
			m.updateCompletion()
			return m, cmd
		}

	case spinner.TickMsg:
		if m.thinking {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case streamFrame:
		if msg.first {
			m.thinking = false
			m.streaming = true
			m.streamBuffer = ""
			m.streamOpen = true
			// 創建助手消息佔位符（僅身份頭）
			header := assistantNameSty.Render("◈ DSC · " + m.displayModelName())
			m.appendMessage(header + "\n")
			m.streamMsgIdx = len(m.lines) - 1
			m.render()
			m.viewport.GotoBottom()
			if msg.ch == nil {
				m.streaming = false
				m.streamOpen = false
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
			if msg.err != nil {
				m.appendMessage(errorSty.Render("错误: ") + msg.err.Error())
			} else if msg.frame != nil && msg.frame.Status == "success" {
				// 最終渲染：將 streamBuffer 作為正文添加到助手消息中
				if m.streamMsgIdx < len(m.lines) {
					body := renderMarkdown(m.streamBuffer, max(m.width-4, 20))
					header := assistantNameSty.Render("◈ DSC · " + m.displayModelName())
					m.lines[m.streamMsgIdx] = header + "\n" + body
				}
			}
			m.viewport.SetHeight(m.vpHeight())
			m.render()
			m.input.Focus()
			m.viewport.GotoBottom()
			return m, nil
		}
		if msg.err != nil {
			m.thinking = false
			m.streaming = false
			m.streamOpen = false
			m.appendMessage(errorSty.Render("错误: ") + msg.err.Error())
			m.viewport.SetHeight(m.vpHeight())
			m.render()
			m.input.Focus()
			m.viewport.GotoBottom()
			return m, nil
		}
		f := msg.frame
		switch f.Status {
		case "streaming":
			m.streaming = true
			if !m.streamOpen {
				// 開始新的助手消息塊
				header := assistantNameSty.Render("◈ DSC · " + m.displayModelName())
				m.appendMessage(header + "\n")
				m.streamMsgIdx = len(m.lines) - 1
				m.streamOpen = true
			}
			m.streamBuffer += f.Output
			body := renderMarkdown(m.streamBuffer, max(m.width-4, 20))
			header := assistantNameSty.Render("◈ DSC · " + m.displayModelName())
			m.lines[m.streamMsgIdx] = header + "\n" + body
			m.render()
			m.viewport.GotoBottom()
			return m, m.pumpStream(msg.input, msg.ch)
		case "tool":
			m.streaming = false
			m.streamOpen = false
			toolLine := dimSty.Render(f.Output)
			if len(m.lines) > 0 && m.lines[len(m.lines)-1] == "\n"+toolLine {
				// 避免重複追加
			} else {
				m.appendMessage(toolLine)
			}
			m.render()
			m.viewport.GotoBottom()
			return m, m.pumpStream(msg.input, msg.ch)
		case "success", "error":
			m.thinking = false
			m.streaming = false
			m.streamOpen = false
			if f.Status == "error" && f.Error != "" {
				m.appendMessage(errorSty.Render("错误: ") + f.Error)
			}
			m.viewport.SetHeight(m.vpHeight())
			m.render()
			m.input.Focus()
			m.viewport.GotoBottom()
			return m, nil
		}
		return m, m.pumpStream(msg.input, msg.ch)

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

// render 将历史行渲染到 viewport
func (m *Model) render() {
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
}

// renderUserBubble 把用户消息渲染为左对齐的紫色文本（颜色 + 前缀符号与助手区分，不套气泡框）。
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
			out[i] = userTextSty.Render("› " + l)
		} else {
			out[i] = userTextSty.Render(strings.Repeat(" ", 2) + l)
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

// renderAssistant 组装助手回复：身份头（◈ DSC · 模型名）+ markdown 正文。
func (m *Model) renderAssistant(body string) string {
	header := assistantNameSty.Render("◈ DSC · " + m.displayModelName())
	return header + "\n" + body
}

// 内置斜杠命令列表（当前为宿主可直接执行的命令）。
var slashCommands = []compItem{
	{label: "/help", insert: "/help", hint: "显示帮助与快捷键"},
	{label: "/clear", insert: "/clear", hint: "清空聊天记录"},
	{label: "/exit", insert: "/exit", hint: "退出聊天"},
}

// runSlashCommand 处理斜杠命令；返回是否已处理以及要执行的命令。
func (m *Model) runSlashCommand(cmd string) (bool, tea.Cmd) {
	switch cmd {
	case "/help":
		help := strings.Join([]string{
			"快捷键:",
			"  Enter        发送消息",
			"  Ctrl+J       换行（终端协议不区分 Ctrl+Enter 与 Enter，故用 LF 键）",
			"  ↑/↓          选择命令 / 滚动消息",
			"  /            在输入框首字符唤起命令菜单",
			"  Ctrl+C       退出",
			"",
			"斜杠命令:",
			"  /help        显示本帮助",
			"  /clear       清空聊天记录",
			"  /exit        退出聊天",
		}, "\n")
		m.appendMessage(assistantNameSty.Render("◈ DSC · 帮助") + "\n" + help)
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
	case "/quit", "/exit":
		return true, tea.Quit
	}
	return false, nil
}

// updateCompletion 根据当前输入重新计算命令补全菜单：
// 仅在输入是「/」开头的单个词（不含空格）时弹出，并按前缀/子序列过滤。
func (m *Model) updateCompletion() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") || strings.ContainsAny(val, " \t\n") {
		m.completion = completion{}
		return
	}
	items := filterSlash(slashCommands, val)
	if len(items) == 0 {
		m.completion = completion{}
		return
	}
	m.completion = completion{active: true, items: items}
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

// statusBar 渲染底部状态栏：左侧模型/状态，右侧快捷键提示。
func (m *Model) statusBar() string {
	left := "模型: " + m.displayModelName()
	if m.thinking {
		left = "思考中... · " + left
	}
	right := "Enter 发送 · Ctrl+J 换行 · Ctrl+C 退出"
	pad := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if pad < 1 {
		pad = 1
	}
	return dimSty.Render(left + strings.Repeat(" ", pad) + right)
}

// View 渲染视图
func (m *Model) View() tea.View {
	if !m.ready {
		return m.viewOf("加载中...")
	}

	title := titleSty.Render(" ◆ DSC  |  模型: " + m.displayModelName() + " ")
	title = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, title)

	var parts []string
	parts = append(parts, title)
	parts = append(parts, m.viewport.View())
	if m.thinking {
		parts = append(parts, dimSty.Render("  "+m.spinner.View()+" 思考中..."))
	}
	if c := m.completionView(); c != "" {
		parts = append(parts, c)
	}
	parts = append(parts, composerBoxSty.Render(strings.TrimRight(m.input.View(), "\n")))
	parts = append(parts, m.statusBar())
	return m.viewOf(strings.Join(parts, "\n"))
}

// viewOf 把内容包装成视图并声明终端特性：进入备用屏幕、开启单元格级鼠标移动。
func (m *Model) viewOf(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// Run 运行聊天界面，阻塞直到退出
func Run(agent plugin.Agent, ctx context.Context, modelName string) error {
	m := New(agent, ctx, modelName)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
