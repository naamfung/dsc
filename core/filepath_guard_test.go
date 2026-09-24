package core

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// 哨兵测试：全仓扫描 filepath 包被禁函数的直接使用（黑名单），Windows 下检出即
// 报错。背景：filepath 包在 Windows 上会把路径结果归一化为原生反斜杆（实测
// Clean/Dir/Join/Abs/Rel/Split/FromSlash/EvalSymlinks/Glob/WalkDir/Walk 均如此，
// 甚至对正斜杆输入也归一化为反斜杆），违背「内部 POSIX shell 统一视窗与 UNIX
// 都以正斜杆处理路径输入输出」的约定——模型可见路径一旦泄漏反斜杆，跨平台行为
// 即漂移。本测试在 CI 就地拦截，防止新代码重新引入反斜杆路径。
//
// 新增路径处理代码的正确接入方式：
//   - 本仓主模块（core/session/tui/cron/main 等）：直接用 core.P* 系列；
//   - 插件（独立 go.mod）：用 SDK 转发 dsc.P*（sdk/fs.go）；
//   - libs/sh、plugin（独立模块，无法引入 dsc/core）：用其本地等价实现
//     （libs/sh/internal/posixpath、plugin/internal/pathx）。
//
// 豁免（skipDirs）仅限第三方非直管 vendored 代码与仓库内非运行时产物；本仓直管
// 模块（core、sdk、plugin fork、libs/sh fork、libs/cron、session、tui、各 plugins）
// 一律无豁免权。确属合法内部调用点须登记 fileAllowlist 并注明理由。
func TestFilepathBlacklistGuard(t *testing.T) {
	// 本测试文件位于 core/ 下，仓库根即其上一级。
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller 定位测试文件失败")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile))

	// 目录豁免（相对仓库根，"/" 分隔）：非直管的 vendored 第三方、语义例外与
	// 仓库内非运行时产物。
	// libs/anthropic-sdk-go、libs/go-openai、libs/go-lua、libs/jig-lisp、
	// libs/toon-go、plugins/tool-2fa-master/vendor：vendored 第三方依赖，非本仓
	// 直管（改动会造成与上游无谓分叉，runtime 路径链路亦不涉及）。
	// libs/fasttemplate、libs/bytebufferpool：仅离线构建 vendored 的第三方模板库，
	// 无路径处理逻辑。
	// libs/vodka：属本仓自研（Web 框架），归属上按直管论，登记为**语义例外**——
	// 其 filepath 用法集中在 HTTP 框架内部（静态文件服务、session 文件存储、模板
	// 加载），不进入模型可见的路径链路，该语境下原生分隔符本就是正确选择（改造
	// 无收益且会改坏框架内部路径语义）。例外须逐条登记理由（见 AGENTS.md）。
	// builder：开发者本机构建工具，其路径是构建产物路径，不进入模型可见的运行时
	// 路径链路（对齐 proc_attr 哨兵测试豁免口径）。
	// examples / testdata / docs / dist / webui / node_modules / .git：示例、测试
	// 数据与构建产物，非运行时链路。
	skipDirs := map[string]bool{
		".git":                         true,
		"node_modules":                 true,
		"webui":                        true,
		"dist":                         true,
		"docs":                         true,
		"examples":                     true,
		"testdata":                     true,
		filepath.ToSlash("libs/vodka"): true,
		filepath.ToSlash("libs/anthropic-sdk-go"):          true,
		filepath.ToSlash("libs/go-openai"):                 true,
		filepath.ToSlash("libs/go-lua"):                    true,
		filepath.ToSlash("libs/jig-lisp"):                  true,
		filepath.ToSlash("libs/toon-go"):                   true,
		filepath.ToSlash("libs/fasttemplate"):              true,
		filepath.ToSlash("libs/bytebufferpool"):            true,
		filepath.ToSlash("plugins/tool-2fa-master/vendor"): true,
		filepath.ToSlash("builder"):                        true,
	}

	// 文件豁免（相对仓库根，"/" 分隔）：唯一合法的内部 filepath 调用点，逐一注明理由。
	fileAllowlist := map[string]string{
		// core/posixpath.go：core.P* 系列唯一实现点（先原生计算再 ToSlash），
		// 全仓黑名单函数的合法调用源。
		"core/posixpath.go": "core.P* 正斜杆 helper 唯一实现点",
		// libs/sh/internal/posixpath：libs/sh 是独立模块（本仓维护的 mvdan fork），
		// 无法引入 dsc/core，提供本地等价 P*。
		"libs/sh/internal/posixpath/posixpath.go": "libs/sh 独立模块本地 P* 等价实现",
		// plugin/internal/pathx：plugin 是独立模块（本仓维护的 go-plugin fork），
		// 无法引入 dsc/core，提供本地等价 P*。
		"plugin/internal/pathx/pathx.go": "plugin 独立模块本地 P* 等价实现",
		// cron/posix.go 与 session/posix.go：cron、session 与 core 存在包级 import 环
		// （core 引用它们），无法复用 core.P*，提供本地等价实现。
		"cron/posix.go":    "cron 包本地 P* 等价实现（core 引用 cron，import 环）",
		"session/posix.go": "session 包本地 P* 等价实现（core 引用 session，import 环）",
	}

	// 黑名单：Windows 上会把结果归一化为原生反斜杆的 filepath 函数。
	// 安全不在此列：ToSlash / IsAbs / Base / Ext / Match / VolumeName / SplitList /
	// ListSeparator / SkipDir / SkipAll / ErrBadPattern（不返回路径或返回正斜杆）。
	// WalkDir 须排在 Walk 前；\b 防误捕 SplitList / ToSlash 等。
	blacklist := regexp.MustCompile(
		`filepath\.(Join|Abs|Clean|Rel|Split|FromSlash|EvalSymlinks|Glob|WalkDir|Walk|Dir|Separator)\b`)

	var violations []string
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[relSlash] || skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(data)
		if blacklist.FindStringIndex(src) == nil {
			return nil
		}
		if _, exempt := fileAllowlist[relSlash]; exempt {
			return nil
		}
		// 收集全部命中行号，便于报错直达。
		lines := []int{}
		for i, l := range strings.Split(src, "\n") {
			if blacklist.MatchString(l) {
				lines = append(lines, i+1)
			}
		}
		violations = append(violations,
			relSlash+":"+strconv.Itoa(lines[0])+" 使用了被禁 filepath 函数（Windows 下返回反斜杆路径）；修复：改用 core.P* / dsc.P* / 模块内 posixpath 等价实现（确属合法内部调用点则登记 fileAllowlist 并注明理由）")
		return nil
	})
	if err != nil {
		t.Fatalf("扫描仓库失败: %v", err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

// TestSlashTerminologyGuard 哨兵测试：全仓扫描源码中北方普通话用词（Unicode
// \u659c\u6760）的使用，项目统一用「斜杆」（南北通用词）。检出即报错。
//
// 背景：该北方用词是北方普通话说法，「斜杆」是粤语/南方用词且为南北通用词。
// 项目维护者来自南方，明确要求统一用「斜杆」。本测试在 CI 就地拦截，
// 防止新代码引入北方用词。
//
// 扫描范围：与 TestFilepathBlacklistGuard 一致的 skipDirs（第三方 vendored 与
// 非运行时产物），但包含 _test.go（测试注释也要统一用词）与 .md/.yaml 文件。
// 本测试文件自身通过 Unicode 转义引用被禁词，不命中自身扫描。
func TestSlashTerminologyGuard(t *testing.T) {
	// 本测试文件位于 core/ 下，仓库根即其上一级。
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller 定位测试文件失败")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile))

	// 被禁词：北方普通话用词（\u659c = 斜，\u6760 = 杠）。
	// 项目统一用「斜杆」（\u659c = 斜，\u6746 = 杆）——南北通用词。
	// 用 Unicode 转义避免本测试文件自命中。
	forbiddenTerm := "\u659c\u6760"

	// 目录豁免（同 TestFilepathBlacklistGuard 的口径：非直管的 vendored 第三方、
	// 逐条登记理由的语义例外 libs/vodka、以及仓库内非运行时产物）。
	skipDirs := map[string]bool{
		".git":                         true,
		"node_modules":                 true,
		"webui":                        true,
		"dist":                         true,
		"docs":                         true,
		"examples":                     true,
		"testdata":                     true,
		filepath.ToSlash("libs/vodka"): true,
		filepath.ToSlash("libs/anthropic-sdk-go"):          true,
		filepath.ToSlash("libs/go-openai"):                 true,
		filepath.ToSlash("libs/go-lua"):                    true,
		filepath.ToSlash("libs/jig-lisp"):                  true,
		filepath.ToSlash("libs/toon-go"):                   true,
		filepath.ToSlash("libs/fasttemplate"):              true,
		filepath.ToSlash("libs/bytebufferpool"):            true,
		filepath.ToSlash("plugins/tool-2fa-master/vendor"): true,
		filepath.ToSlash("builder"):                        true,
	}

	// 本测试文件自身豁免（用 Unicode 转义引用被禁词，理论上不自命中，
	// 但登记豁免以防 future 编辑引入直接引用）。
	selfFile := filepath.ToSlash(filepath.Join("core", "filepath_guard_test.go"))

	var violations []string
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if skipDirs[relSlash] || skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		// 扫描 .go（含 _test.go）、.md、.yaml、.yml 文件
		isGo := strings.HasSuffix(name, ".go")
		isMd := strings.HasSuffix(name, ".md")
		isYaml := strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
		if !isGo && !isMd && !isYaml {
			return nil
		}
		// 本测试文件自身豁免
		if relSlash == selfFile {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(data)
		if !strings.Contains(src, forbiddenTerm) {
			return nil
		}
		// 收集全部命中行号，便于报错直达。
		lines := []int{}
		for i, l := range strings.Split(src, "\n") {
			if strings.Contains(l, forbiddenTerm) {
				lines = append(lines, i+1)
			}
		}
		for _, ln := range lines {
			violations = append(violations,
				relSlash+":"+strconv.Itoa(ln)+" 使用了北方普通话用词「"+forbiddenTerm+
					"」；项目统一用「斜杆」（南北通用词）；修复：替换为「斜杆」")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描仓库失败: %v", err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}
