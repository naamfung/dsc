package coderuntime

import "strings"

// ToolSpec 供生成 Lua SDK 使用的一个工具定义。
type ToolSpec struct {
	Name        string
	Description string
	JSONSchema  string
}

// GenerateSDK 由工具目录生成一段 Lua SDK 源码：每个工具一个同名全局函数，
// 转发到内置 tool(name, args)。函数签名统一为单表参数、失败 raise：
//
//	--- <description 首行>
//	function read_file(args)
//	  local ok, r = tool("read_file", args or {})
//	  if not ok then error(r, 0) end
//	  return r
//	end
//
// 仅对「合法 Lua 标识符」的工具名生成同名函数；其余名字仍可经 tool(name,args)
// 直接调用。调用方把这段 SDK 文本拼接在用户脚本之前一起执行即可。
func GenerateSDK(tools []ToolSpec) string {
	var sb strings.Builder
	sb.WriteString("-- auto-generated tool SDK (from the host tool catalog)\n")
	for _, t := range tools {
		if !isLuaIdent(t.Name) {
			continue // 非合法标识符工具：不生成同名函数，模型仍可用 tool(name, args)
		}
		if desc := oneLine(t.Description); desc != "" {
			sb.WriteString("-- " + desc + "\n")
		}
		sb.WriteString("function " + t.Name + "(args)\n")
		sb.WriteString("  local ok, r = tool(\"" + t.Name + "\", args or {})\n")
		sb.WriteString("  if not ok then error(r, 0) end\n")
		sb.WriteString("  return r\n")
		sb.WriteString("end\n")
	}
	return sb.String()
}

// isLuaIdent 判断是否为合法的 Lua 标识符（字母/下划线开头，后随字母数字下划线）。
func isLuaIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		valid := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || (i > 0 && r >= '0' && r <= '9')
		if !valid {
			return false
		}
	}
	return true
}

// oneLine 把多行描述压成单行并规避 Lua 注释边界，确保作为 -- 注释安全。
func oneLine(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	s = strings.ReplaceAll(s, "--", "- -")
	return s
}
