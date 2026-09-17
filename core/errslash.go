package core

import (
	"errors"
	"os"
	"strings"
)

// 错误文本路径归一：Windows 上 os.* 系列错误内嵌反斜杆原生路径（如
// GetFileAttributesEx D:\dir\src: The system cannot find the file specified.），
// 与 DSC「POSIX 内置 shell、所有输入输出统一正斜杆」的平台规范相悖。归一职责
// 分两层：插件侧 SlashErr 在错误产生点结构化归一（保留错误类型供 errors.Is/As
// 判断）；宿主聚合工具服务出口（ToolGRPCServer.ExecuteTool）以 SlashErrText
// 对错误文本单点兜底——任何工具插件（含未自净者）的错误到达模型与 TUI 前都
// 已归一。对齐 AGENTS.md 第 10 条：禁止 filepath.ToSlash，必须两行连续替换
// （先反引号替换连续双反斜杆——UNC 前缀 \\server\share → //server/share；
// 后双引号替换单反斜杆）。

// SlashErrText 把文本里内嵌的 Windows 反斜杆路径归一为正斜杆。不含反斜杆时
// 零分配原样返回（工具错误高频路径上避免无谓拷贝）。UNC 前缀的连续双反斜杆
// 归一为双正斜杆（\\server\share → //server/share，Windows 可解析的等价形式）。
func SlashErrText(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	s = strings.ReplaceAll(s, `\\`, "//") // 先：反引号（连续双反斜杆，UNC 前缀）
	s = strings.ReplaceAll(s, "\\", "/")  // 后：双引号（单反斜杆）
	return s
}

// SlashErr 归一 error 内嵌路径：*os.PathError 结构化归一 Path 字段（保留错误
// 类型与 Op/Err 字段，errors.Is/As 判断不中断）；其余错误原样返回——文本级
// 归一由宿主聚合出口（SlashErrText）统一兜底，不在本函数破坏错误包装链。
func SlashErr(err error) error {
	if err == nil {
		return nil
	}
	var pe *os.PathError
	if errors.As(err, &pe) {
		p := SlashErrText(pe.Path)
		if p == pe.Path {
			return err
		}
		return &os.PathError{Op: pe.Op, Path: p, Err: pe.Err}
	}
	return err
}
