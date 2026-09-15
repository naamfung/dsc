// 本文件实现 `dsc version`：输出版本信息，格式为「v3.0.0 短哈希」或「v3.0.0」。
//
// 版本号单一来源是项目根目录的 VERSION 纯文本文件（//go:embed 编译期嵌入二进制），
// 手工迭代版本号只改这一个文件，与源码分离。git 哈希由 Go 工具链在编译期自动注入
// （buildvcs：在 git 工作树内 go build 时，完整提交哈希经 debug.ReadBuildInfo 的
// vcs.revision 暴露）；在无 .git 的源码包（如从 GitHub 下载 ZIP 源码）中编译时无此
// 标记，此时仅输出版本号。
package main

import (
	_ "embed"
	"fmt"
	"runtime/debug"
	"strings"
)

//go:embed VERSION
var versionFile string

// isVersionCommand 判断命令行是否为 `dsc version`：第一个非 flag 参数（不以 -
// 开头）为 "version" 时输出版本信息；-input 等带值 flag 会被跳过（其值不算位置
// 参数），判定规则与 `dsc setup` 一致。
func isVersionCommand(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-input" || a == "-admin" || a == "-mode" || a == "-log":
			i++ // 跳过 flag 的取值（若存在）
		case strings.HasPrefix(a, "-"):
			// 无值 flag（-headless / -debugger 等），跳过
		default:
			return a == "version"
		}
	}
	return false
}

// versionString 返回 VERSION 文件中的规范化版本号：缺 v 前缀时补齐，
// 空文件兜底 v0.0.0（正常情况下 VERSION 缺失会导致编译失败而非走到这里）。
func versionString() string {
	v := strings.TrimSpace(versionFile)
	if v == "" {
		return "v0.0.0"
	}
	if !strings.HasPrefix(v, "v") && !strings.HasPrefix(v, "V") {
		return "v" + v
	}
	return v
}

// gitRevision 返回编译期注入的 git 提交短哈希；二进制不在 git 工作树内编译
// （ZIP 源码包、-buildvcs=false 等）时无 vcs.revision 标记，返回空串。
func gitRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			if len(s.Value) > 7 {
				return s.Value[:7]
			}
			return s.Value
		}
	}
	return ""
}

// printVersion 输出版本信息：git 工作树内编译输出「v3.0.0 短哈希」，
// 否则仅输出版本号「v3.0.0」。
func printVersion() {
	if rev := gitRevision(); rev != "" {
		fmt.Printf("%s %s\n", versionString(), rev)
		return
	}
	fmt.Println(versionString())
}
