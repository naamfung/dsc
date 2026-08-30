package core

// 结构化工具结果视图（对齐 DSH 显示契约）。
// 实现工具的插件在结果里声明"显示什么"（版式、标题、徽标、字段/列/行/正文），
// TUI 统一渲染"怎么画"（配色/对齐/折行），保证各工具结果风格一致；
// 视图缺失或 kind 未实现时 TUI 回退到通用展示。经 ExecuteToolResponse.view_json 传输。

// 视图 kind：
//   - "card"   键值卡片：标题 + 徽标 + 对齐键值字段（缺省即 card）
//   - "table"  表格：标题 + 徽标 + 对齐列（含列头），适合多行记录
//   - "plain"  纯文本块：可选标题 + 正文，适合大段文本结果

// ToolView 是工具结果的可选结构化视图 spec。
type ToolView struct {
	Kind    string       `json:"kind"`              // "card" | "table" | "plain"（缺省即 card）
	Title   string       `json:"title,omitempty"`   // 标题（如 "Goal"）
	Badge   *ViewBadge   `json:"badge,omitempty"`   // 头部状态徽标（如 phase）
	Fields  []ViewField  `json:"fields,omitempty"`  // card：键值字段（同层值列对齐）
	Columns []ViewColumn `json:"columns,omitempty"` // table：列定义（含列头）
	Rows    []ViewRow    `json:"rows,omitempty"`    // table：行数据（每行按列 key 取值）
	Body    string       `json:"body,omitempty"`    // plain：正文
}

// ViewBadge 卡片/表格头部的状态徽标。
type ViewBadge struct {
	Text string `json:"text"`
	Tone string `json:"tone,omitempty"` // 色板：teal/green/yellow/red/gray
}

// ViewField 卡片中的一行键值字段。
type ViewField struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Tone  string `json:"tone,omitempty"` // 值着色（色板同 ViewBadge.Tone）
}

// ViewColumn 表格中的一列。
type ViewColumn struct {
	Key   string `json:"key"`             // 行数据取值键
	Title string `json:"title,omitempty"` // 列标题（缺省用 Key）
	Tone  string `json:"tone,omitempty"`  // 整列值着色（如状态列）
}

// ViewRow 表格中的一行：列 key → 单元格文本。
type ViewRow map[string]string
