package dsc

import (
	"encoding/json"

	"dsc/core"
)

// 结构化工具结果视图（对齐 DSH 显示契约）：插件用 dsc.CardView / dsc.TableView /
// dsc.PlainView 声明"显示什么"，TUI 统一渲染"怎么画"，使各工具结果呈现为风格一致
// 的专用卡片/表格/文本块。视图缺失时 TUI 回退到通用展示。

// View / ViewBadge / ViewField / ViewColumn / ViewRow 是视图 spec 的 SDK 侧便捷类型
// （与 core 共享同一契约）。
type View = core.ToolView
type ViewBadge = core.ViewBadge
type ViewField = core.ViewField
type ViewColumn = core.ViewColumn
type ViewRow = core.ViewRow

// CardView 构造一个卡片视图 spec 的 JSON（供 Tool.ViewFn 返回）。
// title 为卡片标题，badge 为头部状态徽标（如 phase），fields 为按逻辑顺序对齐的键值字段。
func CardView(title string, badge *ViewBadge, fields []ViewField) json.RawMessage {
	b, err := json.Marshal(View{Kind: "card", Title: title, Badge: badge, Fields: fields})
	if err != nil {
		return nil
	}
	return b
}

// TableView 构造一个表格视图 spec 的 JSON（供 Tool.ViewFn 返回）。
// columns 定义列（含列头与整列着色），rows 为行数据（每行按列 key 取值）。
// 适合多行同构记录（搜索结果、任务清单、会话列表等）。
func TableView(title string, badge *ViewBadge, columns []ViewColumn, rows []ViewRow) json.RawMessage {
	b, err := json.Marshal(View{Kind: "table", Title: title, Badge: badge, Columns: columns, Rows: rows})
	if err != nil {
		return nil
	}
	return b
}

// PlainView 构造一个纯文本视图 spec 的 JSON（供 Tool.ViewFn 返回）。
// title 可选（如文件路径、表达式），badge 可选（如耗时/退出码），body 为正文文本。
func PlainView(title string, badge *ViewBadge, body string) json.RawMessage {
	b, err := json.Marshal(View{Kind: "plain", Title: title, Badge: badge, Body: body})
	if err != nil {
		return nil
	}
	return b
}
