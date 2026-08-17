// Package tui 提供基于 Bubble Tea 的终端聊天界面。
// 该界面运行在宿主进程中（不通过 go-plugin 子进程），
// 因为 TUI 需要直接操作终端 raw mode 和 stdout，而插件子进程的 stdout 会被 go-plugin 捕获。
package tui

import (
	"context"
	"fmt"
	"strings"

	"dsc/plugin"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 主题色
var (
	userStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Bold(true)
	assistantSty = lipgloss.NewStyle().Foreground(lipgloss.Color("#05A5A5"))
	systemSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Italic(true)
	errorSty     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87"))
	titleSty     = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).Bold(true).Padding(0, 1)
)

// submitResult 是一次 agent.Run 完成后的结果消息
type submitResult struct {
	input  string
	result *plugin.AgentResult
	err    error
}

// Model 是聊天界面的状态模型
type Model struct {
	agent plugin.Agent
	ctx   context.Context
	modelName string

	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	ready    bool // viewport 在收到首个窗口尺寸后初始化
	thinking bool // 正在等待 agent 响应
	quit     bool

	lines []string // 渲染后的历史行
	width int
	high  int
}

// New 创建一个聊天界面模型
func New(agent plugin.Agent, ctx context.Context, modelName string) *Model {
	input := textinput.New()
	input.Placeholder = "输入消息，回车发送，Ctrl+C 退出"
	input.CharLimit = 4096
	input.Prompt = "❯ "

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

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
		spinner.Tick,
	)
}

// submitCmd 发起一次 agent.Run，结果通过 submitResult 消息返回
func (m *Model) submitCmd(input string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.agent.Run(m.ctx, input)
		return submitResult{input: input, result: result, err: err}
	}
}

// Update 处理消息
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.high = msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-4)
			m.viewport.YPosition = 0
			m.input.Width = msg.Width - len(m.input.Prompt) - 4
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 4
			m.input.Width = msg.Width - len(m.input.Prompt) - 4
		}
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

	case tea.KeyMsg:
		if m.thinking {
			// 响应期间不处理输入，但允许 Ctrl+C 退出
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" {
				return m, nil
			}
			m.lines = append(m.lines, userStyle.Render("你: ")+text)
			m.render()
			m.input.SetValue("")
			m.thinking = true
			m.viewport.GotoBottom()
			return m, tea.Batch(
				m.submitCmd(text),
				spinner.Tick,
			)
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

	case spinner.TickMsg:
		if m.thinking {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case submitResult:
		m.thinking = false
		if msg.err != nil {
			m.lines = append(m.lines, errorSty.Render("错误: ")+msg.err.Error())
		} else if msg.result != nil {
			if msg.result.Status == "success" {
				m.lines = append(m.lines, assistantSty.Render("助手: ")+msg.result.Output)
			} else {
				m.lines = append(m.lines, errorSty.Render("错误: ")+msg.result.Output)
			}
		}
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

// View 渲染视图
func (m *Model) View() string {
	if !m.ready {
		return "加载中..."
	}

	modelName := m.modelName
	if modelName == "" {
		modelName = m.agent.Name(m.ctx)
	}
	title := titleSty.Render(fmt.Sprintf(" DSC Agent Chat | 模型: %s ", modelName))
	title = lipgloss.PlaceHorizontal(m.width, lipgloss.Center, title)

	var status string
	if m.thinking {
		status = m.spinner.View() + " 助手思考中..."
	} else {
		status = "就绪 | 滚动: ↑/↓ 或鼠标滚轮"
	}
	statusLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(status)

	return strings.Join([]string{
		title,
		m.viewport.View(),
		statusLine,
		m.input.View(),
	}, "\n")
}

// Run 运行聊天界面，阻塞直到退出
func Run(agent plugin.Agent, ctx context.Context, modelName string) error {
	m := New(agent, ctx, modelName)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
