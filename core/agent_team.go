package core

import (
        "fmt"
        "sort"
        "strings"
        "sync"
        "time"
)

// Agent Team 实验性多代理团队（对齐 DSH packages/experimental/agent-team）。
//
// DSH 的 Agent Team 允许在一个会话中运行多个代理，共享任务板与持久消息——
// 代理间经共享任务板协调工作，消息可跨代理持久化传递。
//
// DSC 的适配：AgentTeam 管理多个子代理实例，共享 TaskBoard：
//   - 子代理经 workflow/subagent 机制扇出
//   - TaskBoard 经内存共享（线程安全），代理间经它协调任务分配
//   - 消息经 MessageQueue 持久化传递（代理间异步通信）
//
// 当前为实验性实现——基础的任务板 + 消息传递，后续可扩展为完整的团队编排。

// TaskStatus 任务状态。
type TaskStatus string

const (
        TaskStatusTodo       TaskStatus = "todo"
        TaskStatusInProgress TaskStatus = "in_progress"
        TaskStatusDone       TaskStatus = "done"
        TaskStatusBlocked    TaskStatus = "blocked"
)

// TeamTask 团队任务板中的一个任务。
type TeamTask struct {
        ID        string     `json:"id"`
        Title     string     `json:"title"`
        Status    TaskStatus `json:"status"`
        Assignee  string     `json:"assignee,omitempty"` // 被分配的代理名
        Priority  int        `json:"priority,omitempty"`
        CreatedAt int64      `json:"created_at"`
        UpdatedAt int64      `json:"updated_at"`
}

// TeamMessage 代理间的消息。
type TeamMessage struct {
        ID        string `json:"id"`
        From      string `json:"from"` // 发送代理名
        To        string `json:"to"`   // 接收代理名（空 = 广播）
        Content   string `json:"content"`
        CreatedAt int64  `json:"created_at"`
}

// TaskBoard 共享任务板（线程安全）。
type TaskBoard struct {
        mu     sync.RWMutex
        tasks  map[string]*TeamTask
        nextID int
}

// NewTaskBoard 创建空任务板。
func NewTaskBoard() *TaskBoard {
        return &TaskBoard{
                tasks:  make(map[string]*TeamTask),
                nextID: 1,
        }
}

// AddTask 添加任务到任务板。
func (tb *TaskBoard) AddTask(title string, priority int) *TeamTask {
        tb.mu.Lock()
        defer tb.mu.Unlock()
        id := fmt.Sprintf("task-%d", tb.nextID)
        tb.nextID++
        now := time.Now().UnixMilli()
        task := &TeamTask{
                ID:        id,
                Title:     title,
                Status:    TaskStatusTodo,
                Priority:  priority,
                CreatedAt: now,
                UpdatedAt: now,
        }
        tb.tasks[id] = task
        return task
}

// GetTask 按 ID 获取任务。
func (tb *TaskBoard) GetTask(id string) (*TeamTask, bool) {
        tb.mu.RLock()
        defer tb.mu.RUnlock()
        t, ok := tb.tasks[id]
        if !ok {
                return nil, false
        }
        cp := *t
        return &cp, true
}

// UpdateTask 更新任务状态/分配。
func (tb *TaskBoard) UpdateTask(id string, status TaskStatus, assignee string) error {
        tb.mu.Lock()
        defer tb.mu.Unlock()
        t, ok := tb.tasks[id]
        if !ok {
                return fmt.Errorf("task %q not found", id)
        }
        t.Status = status
        if assignee != "" {
                t.Assignee = assignee
        }
        t.UpdatedAt = time.Now().UnixMilli()
        return nil
}

// ListTasks 列出所有任务（按优先级降序，相同优先级按创建时间升序）。
func (tb *TaskBoard) ListTasks() []*TeamTask {
        tb.mu.RLock()
        defer tb.mu.RUnlock()
        out := make([]*TeamTask, 0, len(tb.tasks))
        for _, t := range tb.tasks {
                cp := *t
                out = append(out, &cp)
        }
        // 排序：优先级降序，然后创建时间升序（用 sort.SliceStable 替代 O(n²) 插入排序）
        sort.SliceStable(out, func(i, j int) bool {
                if out[i].Priority != out[j].Priority {
                        return out[i].Priority > out[j].Priority
                }
                return out[i].CreatedAt < out[j].CreatedAt
        })
        return out
}

// ClaimTask 认领任务（原子操作：状态从 todo → in_progress + 设置 assignee）。
func (tb *TaskBoard) ClaimTask(taskID, agentName string) error {
        tb.mu.Lock()
        defer tb.mu.Unlock()
        t, ok := tb.tasks[taskID]
        if !ok {
                return fmt.Errorf("task %q not found", taskID)
        }
        if t.Status != TaskStatusTodo {
                return fmt.Errorf("task %q is not todo (current: %s)", taskID, t.Status)
        }
        t.Status = TaskStatusInProgress
        t.Assignee = agentName
        t.UpdatedAt = time.Now().UnixMilli()
        return nil
}

// CompleteTask 标记任务完成。
func (tb *TaskBoard) CompleteTask(taskID string) error {
        tb.mu.Lock()
        defer tb.mu.Unlock()
        t, ok := tb.tasks[taskID]
        if !ok {
                return fmt.Errorf("task %q not found", taskID)
        }
        t.Status = TaskStatusDone
        t.UpdatedAt = time.Now().UnixMilli()
        return nil
}

// RemoveTask 从任务板删除任务。
func (tb *TaskBoard) RemoveTask(taskID string) {
        tb.mu.Lock()
        defer tb.mu.Unlock()
        delete(tb.tasks, taskID)
}

// MessageQueue 代理间消息队列（线程安全）。
type MessageQueue struct {
        mu       sync.Mutex
        messages []TeamMessage
        nextID   int
}

// NewMessageQueue 创建空消息队列。
func NewMessageQueue() *MessageQueue {
        return &MessageQueue{nextID: 1}
}

// Send 发送消息。
func (mq *MessageQueue) Send(from, to, content string) *TeamMessage {
        mq.mu.Lock()
        defer mq.mu.Unlock()
        id := fmt.Sprintf("msg-%d", mq.nextID)
        mq.nextID++
        msg := TeamMessage{
                ID:        id,
                From:      from,
                To:        to,
                Content:   content,
                CreatedAt: time.Now().UnixMilli(),
        }
        mq.messages = append(mq.messages, msg)
        return &msg
}

// Receive 接收指定代理的消息（to == agentName 或 to == "" 广播），按时间升序返回。
// 不删除已读消息（peek 语义）——若需消费式删除，调用 Acknowledge。
// 广播消息（to == ""）对每个代理返回的副本相同，便于各代理独立消费状态。
func (mq *MessageQueue) Receive(agentName string) []*TeamMessage {
        mq.mu.Lock()
        defer mq.mu.Unlock()
        var out []*TeamMessage
        for i := 0; i < len(mq.messages); i++ {
                m := mq.messages[i]
                if m.To == agentName || m.To == "" {
                        cp := m
                        out = append(out, &cp)
                }
        }
        return out
}

// Acknowledge 删除指定代理已处理的消息（消费语义）。
// 删除条件：to == agentName（点对点消息）；广播消息仅在 broadcastAckAll=true 时删除。
// 此前 Receive 不删除消息，反复调用会返回同批消息导致重复处理；
// 显式 Acknowledge 让调用方决定何时「消费」已读消息。
func (mq *MessageQueue) Acknowledge(agentName string, broadcastAckAll bool) int {
        mq.mu.Lock()
        defer mq.mu.Unlock()
        removed := 0
        out := mq.messages[:0]
        for _, m := range mq.messages {
                if m.To == agentName || (m.To == "" && broadcastAckAll) {
                        removed++
                        continue
                }
                out = append(out, m)
        }
        mq.messages = out
        return removed
}

// ListAll 列出所有消息。
func (mq *MessageQueue) ListAll() []TeamMessage {
        mq.mu.Lock()
        defer mq.mu.Unlock()
        out := make([]TeamMessage, len(mq.messages))
        copy(out, mq.messages)
        return out
}

// AgentTeam 多代理团队（对齐 DSH experimental agent-team）。
type AgentTeam struct {
        mu        sync.Mutex
        name      string
        taskBoard *TaskBoard
        msgQueue  *MessageQueue
        agents    map[string]bool // 代理名集合
}

// NewAgentTeam 创建代理团队。
func NewAgentTeam(name string) *AgentTeam {
        return &AgentTeam{
                name:      name,
                taskBoard: NewTaskBoard(),
                msgQueue:  NewMessageQueue(),
                agents:    make(map[string]bool),
        }
}

// RegisterAgent 注册代理到团队。
func (at *AgentTeam) RegisterAgent(agentName string) {
        at.mu.Lock()
        defer at.mu.Unlock()
        at.agents[agentName] = true
}

// UnregisterAgent 从团队注销代理。
func (at *AgentTeam) UnregisterAgent(agentName string) {
        at.mu.Lock()
        defer at.mu.Unlock()
        delete(at.agents, agentName)
}

// TaskBoard 返回共享任务板。
func (at *AgentTeam) TaskBoard() *TaskBoard {
        return at.taskBoard
}

// MessageQueue 返回共享消息队列。
func (at *AgentTeam) MessageQueue() *MessageQueue {
        return at.msgQueue
}

// Agents 返回已注册的代理名列表。
func (at *AgentTeam) Agents() []string {
        at.mu.Lock()
        defer at.mu.Unlock()
        out := make([]string, 0, len(at.agents))
        for name := range at.agents {
                out = append(out, name)
        }
        return out
}

// FormatTaskBoard 格式化任务板为模型可读文本。
func FormatTaskBoard(tb *TaskBoard) string {
        tasks := tb.ListTasks()
        if len(tasks) == 0 {
                return "任务板为空。"
        }
        var sb strings.Builder
        sb.WriteString(fmt.Sprintf("任务板（%d 项）：\n", len(tasks)))
        for _, t := range tasks {
                statusIcon := map[TaskStatus]string{
                        TaskStatusTodo:       "○",
                        TaskStatusInProgress: "◐",
                        TaskStatusDone:       "●",
                        TaskStatusBlocked:    "✗",
                }[t.Status]
                assignee := ""
                if t.Assignee != "" {
                        assignee = " → " + t.Assignee
                }
                sb.WriteString(fmt.Sprintf("  %s [%s] %s%s\n", statusIcon, t.ID, t.Title, assignee))
        }
        return sb.String()
}
