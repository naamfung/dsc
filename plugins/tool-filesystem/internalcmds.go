package main

// 进程内实现的常用 POSIX 工具（内建式）。选用 mvdan/sh 的初衷是希望它自带标准
// POSIX 命令、不依赖外部 PATH；但其 interp 只实现了 shell 语言内建（cd/pwd/echo/
// printf/test 等），mkdir/ls/cat 等常用工具系外部程序，须按 PATH 查找。在 Windows
// 且插件子进程 PATH 被宿主过滤时这些工具全部失效（exit 127）。
//
// 这里是修复：经 interp.ExecHandler 拦截「既非内建、也非 shell 函数」的简单命令，
// 命中下列命令即进程内用 Go stdlib 实现（纯 filepath/os/io，无外部依赖、跨全平台
// 编译），未命中则回退默认 PATH 外部执行。语义对齐各自标准的常见用法（含 -p/-r/
// -l/-a/-d/-i/-v/-n 等常用旗标），并额外保留 tool-filesystem 已生效的
// /workspace 虚拟根映射（见 mapWorkspacePaths，AST 层已把字面量重写为真实路径，
// 故此处拿到的均是已映射后的绝对/相对路径）。

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"dsc-sdk"
	"dsc/core"
	"mvdan.cc/sh/v3/interp"
)

// defaultExecHandler 保持「未命中内部工具」时原有的 PATH 外部程序执行语义
// （mvdan/sh 默认 kill timeout 为 2s，对齐未设置 ExecHandler 前的库默认行为）。
var defaultExecHandler = interp.DefaultExecHandler(2 * time.Second)

// windowsToPosix 记录【确属 Windows 专属、POSIX 无效】的命令 → 标准 POSIX 替代，供模型
// 误用 Windows 命令时给出可执行的修正提示。DSC 的 shell 是 POSIX（mvdan/sh）解释器，
// 这些命令不是可执行文件也不在内部命令集，直接执行必然失败——与其让模型空转，不如在
// 报错里明确把它引导回标准 POSIX 命令。mkdir/rmdir/date/ping 等本就是 POSIX 命令，不列入。
var windowsToPosix = map[string]string{
	"dir":      "ls 或 ls -la",
	"findstr":  "grep",
	"copy":     "cp",
	"xcopy":    "cp -r",
	"del":      "rm",
	"erase":    "rm",
	"move":     "mv",
	"ren":      "mv",
	"rename":   "mv",
	"more":     "cat（可分页用 cat | less）",
	"where":    "which",
	"cls":      "无需（DSC 终端由 TUI 管理）",
	"tasklist": "ps",
	"taskkill": "kill",
	"ipconfig": "ip addr 或 ifconfig",
	"md":       "mkdir -p",
	"rd":       "rm -r",
	"attrib":   "chmod",
}

// posixHint 返回对已知 Windows 专属命令的 POSIX 替代指引；非 Windows 专属命令返回空。
// 只在命令执行失败时追加，避免干扰真实外部命令的正常报错。
func posixHint(cmd string) string {
	if alt, ok := windowsToPosix[cmd]; ok {
		return fmt.Sprintf("\n提示：%q 是 Windows 专属命令，DSC 的 shell 是 POSIX（mvdan/sh）解释器，不提供该命令。请改用标准 POSIX 命令：%s", cmd, alt)
	}
	return ""
}

// slashErr 把错误字符串里的路径归一为正斜杆（Windows 上 os.* 错误内嵌反斜杆
// 路径，直接回显给模型/用户时与其余正斜杆路径风格不一致）。委托 core.SlashErrText
// 公共实现（与 str_replace_editor 的 error 版归一收敛同一份替换逻辑，见
// core/errslash.go；对齐 AGENTS.md 重复逻辑必须抽取）。
func slashErr(err error) string {
	if err == nil {
		return ""
	}
	return core.SlashErrText(err.Error())
}

// shellExecHandler 是 interp.ExecHandler 的入口：命中内部工具表走进程内实现，
// 否则回退默认外部执行；对 Windows 专属命令失败时附加 POSIX 替代指引。
func shellExecHandler(ctx context.Context, args []string) error {
	if fn, ok := internalCommands[args[0]]; ok {
		hc := interp.HandlerCtx(ctx)
		return fn(ctx, hc, args[1:])
	}
	err := defaultExecHandler(ctx, args)
	if err != nil {
		if hint := posixHint(args[0]); hint != "" {
			return fmt.Errorf("%v%s", err, hint)
		}
	}
	return err
}

// internalCommand 进程内实现一个命令：Write 到 hc.Stdout/hc.Stderr，
// 非零退出用 interp.NewExitStatus 返回。
type internalCommand func(ctx context.Context, hc interp.HandlerContext, args []string) error

var internalCommands = map[string]internalCommand{
	"mkdir": cmdMkdir,
	"ls":    cmdLs,
	"cat":   cmdCat,
	"touch": cmdTouch,
	"rm":    cmdRm,
	"cp":    cmdCp,
	"mv":    cmdMv,
	"grep":  cmdGrep,
	"head":  cmdHead,
	"tail":  cmdTail,
	"wc":    cmdWc,
	"tree":  cmdTree,
}

// ---------- 共享辅助 ----------

// res 把参数里的路径解析为绝对路径（相对路径基于 hc.Dir，即 shell 当前工作目录）。
// Windows 上 /dev/null 特判归一为 NUL 设备：mvdan 的 DefaultOpenHandler 对重定向
// 目标做了同样特判（handler.go），但作为命令参数（如 cat /dev/null）不经该路径，
// 而它又不是合法的文件路径（会被 Join 到工作区下报错），对齐特判使两种用法一致。
func res(hc interp.HandlerContext, p string) string {
	if p == "" {
		return ""
	}
	if runtime.GOOS == "windows" && p == "/dev/null" {
		return "NUL"
	}
	if filepath.IsAbs(p) {
		return p
	}
	return dsc.PJoin(hc.Dir, p)
}

// openContentFiles 打开文件列表（"-" 表示 stdin）读写；返回待 close 句柄。
func openReader(hc interp.HandlerContext, file string) (io.Reader, func(), error) {
	if file == "-" {
		return hc.Stdin, func() {}, nil
	}
	f, err := os.Open(res(hc, file))
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

// ---------- mkdir ----------

func cmdMkdir(ctx context.Context, hc interp.HandlerContext, args []string) error {
	parents := false
	var dirs []string
	for _, a := range args {
		switch a {
		case "-p", "--parents":
			parents = true
		case "--":
		default:
			dirs = append(dirs, a)
		}
	}
	if len(dirs) == 0 {
		fmt.Fprintln(hc.Stderr, "mkdir: missing operand")
		return interp.NewExitStatus(2)
	}
	failed := false
	for _, d := range dirs {
		if d == "" {
			continue
		}
		p := res(hc, d)
		var err error
		if err = ctx.Err(); err != nil {
			return err
		}
		if parents {
			err = os.MkdirAll(p, 0o755)
		} else {
			err = os.Mkdir(p, 0o755)
		}
		if err != nil {
			fmt.Fprintf(hc.Stderr, "mkdir: cannot create directory '%s': %v\n", d, err)
			failed = true
		}
	}
	if failed {
		return interp.NewExitStatus(1)
	}
	return nil
}

// ---------- ls ----------

func cmdLs(ctx context.Context, hc interp.HandlerContext, args []string) error {
	var showAll, long, listSelf, singleCol, classify, byTime, bySize, reverse bool
	var paths []string
	for _, a := range args {
		switch {
		case a == "--":
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, f := range a[1:] {
				switch f {
				case 'a':
					showAll = true
				case 'l':
					long = true
				case 'd':
					listSelf = true
				case 'h':
				case '1':
					singleCol = true
				case 'F':
					// -F 分类符：目录 /、可执行 *、符号链接 @、FIFO |、套接字 =
					classify = true
				case 't':
					// -t 按修改时间排序（新在前）
					byTime = true
				case 'S':
					// -S 按大小排序（大在前）
					bySize = true
				case 'r':
					// -r 逆序
					reverse = true
				default:
					ok = false
				}
			}
			if !ok {
				fmt.Fprintf(hc.Stderr, "ls: unsupported option: %s\n", a)
				return interp.NewExitStatus(2)
			}
		default:
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	multi := len(paths) > 1
	exit := 0
	for idx, p0 := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := res(hc, p0)
		fi, err := os.Lstat(p)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "ls: cannot access '%s': %s\n", p0, slashErr(err))
			exit = 2
			continue
		}
		if multi {
			if idx > 0 {
				fmt.Fprintln(hc.Stdout)
			}
			fmt.Fprintln(hc.Stdout, p0+":")
		}
		if fi.IsDir() && !listSelf {
			if err := lsDir(ctx, hc, p, showAll, long, singleCol, classify, byTime, bySize, reverse); err != nil {
				return err
			}
		} else {
			lsEntry(hc, fi, p0, long, classify)
		}
	}
	if exit != 0 {
		return interp.NewExitStatus(2)
	}
	return nil
}

// lsItem 单个目录项：名字 + stat（-t/-S/-F 均需要 FileInfo，故一次性取齐）。
type lsItem struct {
	name string
	fi   os.FileInfo
}

// sortLsItems 对目录项排序：默认按名升序；-t 按修改时间新在前；-S 按大小大在前；
// -r 整体逆序。Stable 保证同键（如同 mtime）时保持目录序，输出确定。
func sortLsItems(items []lsItem, byTime, bySize, reverse bool) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if reverse {
			a, b = b, a
		}
		switch {
		case byTime:
			return a.fi.ModTime().After(b.fi.ModTime())
		case bySize:
			return a.fi.Size() > b.fi.Size()
		default:
			return a.name < b.name
		}
	})
}

// lsClassifySuffix 返回 -F 分类符（POSIX ls -F 语义）。
func lsClassifySuffix(fi os.FileInfo) string {
	m := fi.Mode()
	switch {
	case m&os.ModeSymlink != 0:
		return "@"
	case m&os.ModeNamedPipe != 0:
		return "|"
	case m&os.ModeSocket != 0:
		return "="
	case m.IsDir():
		return "/"
	case m.Perm()&0o111 != 0:
		return "*"
	}
	return ""
}

// lsName 返回展示名：classify 时追加 -F 分类符。
func lsName(name string, fi os.FileInfo, classify bool) string {
	if classify {
		return name + lsClassifySuffix(fi)
	}
	return name
}

func lsDir(ctx context.Context, hc interp.HandlerContext, p string, showAll, long, singleCol, classify, byTime, bySize, reverse bool) error {
	entries, err := os.ReadDir(p)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "ls: %v\n", err)
		return interp.NewExitStatus(2)
	}
	items := make([]lsItem, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !showAll && strings.HasPrefix(name, ".") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, lsItem{name: name, fi: fi})
	}
	sortLsItems(items, byTime, bySize, reverse)
	if long {
		for _, it := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			lsEntry(hc, it.fi, it.name, true, classify)
		}
	} else if singleCol {
		for _, it := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			fmt.Fprintln(hc.Stdout, lsName(it.name, it.fi, classify))
		}
	} else {
		names := make([]string, 0, len(items))
		for _, it := range items {
			names = append(names, lsName(it.name, it.fi, classify))
		}
		fmt.Fprintln(hc.Stdout, strings.Join(names, "  "))
	}
	return nil
}

func lsEntry(hc interp.HandlerContext, fi os.FileInfo, name string, long, classify bool) {
	if long {
		sz := fi.Size()
		t := fi.ModTime().Format("Jan _2 15:04")
		fmt.Fprintf(hc.Stdout, "%s %8d %s %s\n", fi.Mode().String(), sz, t, lsName(name, fi, classify))
	} else {
		fmt.Fprintln(hc.Stdout, lsName(name, fi, classify))
	}
}

// ---------- cat ----------

func cmdCat(ctx context.Context, hc interp.HandlerContext, args []string) error {
	files := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--" {
			continue
		}
		files = append(files, a)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	exit := 0
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, close, err := openReader(hc, f)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "cat: %s: %v\n", f, err)
			exit = 1
			continue
		}
		if _, err := io.Copy(hc.Stdout, r); err != nil {
			fmt.Fprintf(hc.Stderr, "cat: %s: %v\n", f, err)
			exit = 1
		}
		close()
	}
	if exit != 0 {
		return interp.NewExitStatus(1)
	}
	return nil
}

// ---------- touch ----------

func cmdTouch(ctx context.Context, hc interp.HandlerContext, args []string) error {
	var files []string
	for _, a := range args {
		if a == "--" {
			continue
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			// 简化的 touch 不加 -t/-d/-m 等时间参数支持，但容忍未知旗标继续创建
			continue
		}
		files = append(files, a)
	}
	if len(files) == 0 {
		fmt.Fprintln(hc.Stderr, "touch: missing file operand")
		return interp.NewExitStatus(2)
	}
	failed := false
	now := time.Now()
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := res(hc, f)
		h, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "touch: cannot touch '%s': %v\n", f, err)
			failed = true
			continue
		}
		_ = h.Close()
		if err := os.Chtimes(p, now, now); err != nil && !failed {
			failed = true
		}
	}
	if failed {
		return interp.NewExitStatus(1)
	}
	return nil
}

// ---------- rm ----------

func cmdRm(ctx context.Context, hc interp.HandlerContext, args []string) error {
	recursive, force := false, false
	var files []string
	for _, a := range args {
		switch a {
		case "-r", "-R", "--recursive":
			recursive = true
		case "-f", "--force":
			force = true
		case "--":
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		if !force {
			fmt.Fprintln(hc.Stderr, "rm: missing operand")
			return interp.NewExitStatus(2)
		}
		return nil
	}
	failed := false
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := res(hc, f)
		fi, statErr := os.Lstat(p)
		if statErr != nil {
			if force {
				continue
			}
			fmt.Fprintf(hc.Stderr, "rm: cannot remove '%s': %s\n", f, slashErr(statErr))
			failed = true
			continue
		}
		var err error
		if fi.IsDir() && !recursive {
			fmt.Fprintf(hc.Stderr, "rm: cannot remove '%s': Is a directory\n", f)
			failed = true
			continue
		}
		err = os.RemoveAll(p)
		if err != nil && !force {
			fmt.Fprintf(hc.Stderr, "rm: cannot remove '%s': %v\n", f, err)
			failed = true
		}
	}
	if failed {
		return interp.NewExitStatus(1)
	}
	return nil
}

// ---------- cp / mv ----------

func cmdCp(ctx context.Context, hc interp.HandlerContext, args []string) error {
	recursive := false
	var ops []string
	for _, a := range args {
		switch a {
		case "-r", "-R", "--recursive":
			recursive = true
		case "--":
		default:
			ops = append(ops, a)
		}
	}
	return copyOrMove(ctx, hc, ops, recursive, false)
}

func cmdMv(ctx context.Context, hc interp.HandlerContext, args []string) error {
	var ops []string
	for _, a := range args {
		if a == "--" {
			continue
		}
		ops = append(ops, a)
	}
	return copyOrMove(ctx, hc, ops, true, true)
}

func copyOrMove(ctx context.Context, hc interp.HandlerContext, ops []string, recursive, move bool) error {
	verb := "cp"
	if move {
		verb = "mv"
	}
	if len(ops) < 2 {
		fmt.Fprintf(hc.Stderr, "%s: missing file operand\n", verb)
		return interp.NewExitStatus(2)
	}
	dst := ops[len(ops)-1]
	srcs := ops[:len(ops)-1]
	dstIsDir := false
	if fi, err := os.Stat(res(hc, dst)); err == nil && fi.IsDir() {
		dstIsDir = true
	}
	if len(srcs) > 1 && !dstIsDir {
		fmt.Fprintf(hc.Stderr, "%s: target '%s' is not a directory\n", verb, dst)
		return interp.NewExitStatus(1)
	}
	failed := false
	for _, s := range srcs {
		if err := ctx.Err(); err != nil {
			return err
		}
		src := res(hc, s)
		fi, err := os.Lstat(src)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "%s: %s: %v\n", verb, s, err)
			failed = true
			continue
		}
		// 目标程序：多源/目标为目录→并入该目录。
		dstPath := res(hc, dst)
		if dstIsDir {
			dstPath = dsc.PJoin(dstPath, filepath.Base(src))
		}
		if fi.IsDir() && !recursive {
			fmt.Fprintf(hc.Stderr, "%s: %s: is a directory (not copied); use -r\n", verb, s)
			failed = true
			continue
		}
		var errOp error
		if move {
			errOp = moveEntry(src, dstPath)
		} else {
			errOp = copyEntry(src, dstPath, fi)
		}
		if errOp != nil {
			fmt.Fprintf(hc.Stderr, "%s: %s -> %s: %s\n", verb, s, dst, slashErr(errOp))
			failed = true
		}
	}
	if failed {
		return interp.NewExitStatus(1)
	}
	return nil
}

// moveEntry 移动：优先 os.Rename；跨设备失败时降级为 拷贝+删除。
func moveEntry(src, dst string) error {
	if err := os.MkdirAll(dsc.PDir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyEntry(src, dst, nil); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

// copyEntry 复制文件或目录树（fi 为 src 的 stat，可为 nil 时自取）。
func copyEntry(src, dst string, fi os.FileInfo) error {
	if fi == nil {
		var err error
		fi, err = os.Lstat(src)
		if err != nil {
			return err
		}
	}
	if fi.IsDir() {
		return dsc.PWalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := dsc.PRel(src, p)
			if rel == "." {
				return os.MkdirAll(dst, 0o755)
			}
			dp := dsc.PJoin(dst, rel)
			if d.IsDir() {
				return os.MkdirAll(dp, 0o755)
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if target, e := os.Readlink(p); e == nil {
					if e = os.Symlink(target, dp); e != nil {
						return e
					}
				}
				return nil
			}
			return copyFile(p, dp)
		})
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(dsc.PDir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// ---------- grep ----------

func cmdGrep(ctx context.Context, hc interp.HandlerContext, args []string) error {
	var ignoreCase, invert, lineNum, countOnly bool
	var files []string
	pat := ""
	for _, a := range args {
		switch {
		case a == "--":
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			ok := true
			for _, f := range a[1:] {
				switch f {
				case 'i':
					ignoreCase = true
				case 'v':
					invert = true
				case 'n':
					lineNum = true
				case 'c':
					countOnly = true
				case 'E':
					// -E (extended regex) 对齐 POSIX grep -E
					// 内置实现用 Go regexp（RE2），已支持 extended regex 语法
				case 'e':
					// -e <pattern> 后跟模式参数（对齐 POSIX grep -e）
					// 简化处理：下一个参数作为 pattern
				default:
					ok = false
				}
			}
			if !ok {
				fmt.Fprintf(hc.Stderr, "grep: unsupported option: %s\n", a)
				return interp.NewExitStatus(2)
			}
		default:
			if pat == "" {
				pat = a
			} else {
				files = append(files, a)
			}
		}
	}
	if pat == "" {
		fmt.Fprintln(hc.Stderr, "grep: missing pattern")
		return interp.NewExitStatus(2)
	}
	if ignoreCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "grep: %v\n", err)
		return interp.NewExitStatus(2)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	multi := len(files) > 1
	exit, matchedTotal := 0, 0
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, close, err := openReader(hc, f)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "grep: %s: %v\n", f, err)
			exit = 2
			continue
		}
		count := grepStream(hc, r, re, invert, lineNum, multi, f, countOnly)
		matchedTotal += count
		if countOnly {
			if multi {
				fmt.Fprintf(hc.Stdout, "%s:%d\n", f, count)
			} else {
				fmt.Fprintf(hc.Stdout, "%d\n", count)
			}
		}
		close()
	}
	if exit != 0 {
		return interp.NewExitStatus(uint8(exit))
	}
	if !countOnly && matchedTotal == 0 {
		return interp.NewExitStatus(1)
	}
	return nil
}

// grepStream 把 r 按行匹配；非 countOnly 时输出命中行；返回命中行数。
func grepStream(hc interp.HandlerContext, r io.Reader, re *regexp.Regexp, invert, lineNum, prefix bool, name string, countOnly bool) int {
	sc := bufio.NewScanner(r)
	ln, count := 0, 0
	for sc.Scan() {
		ln++
		hit := re.MatchString(sc.Text())
		if invert {
			hit = !hit
		}
		if !hit {
			continue
		}
		count++
		if countOnly {
			continue
		}
		if prefix {
			fmt.Fprintf(hc.Stdout, "%s:", name)
		}
		if lineNum {
			fmt.Fprintf(hc.Stdout, "%d:", ln)
		}
		fmt.Fprintln(hc.Stdout, sc.Text())
	}
	return count
}

// ---------- head / tail ----------

func cmdHead(ctx context.Context, hc interp.HandlerContext, args []string) error {
	n := int64(10)
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" && i+1 < len(args):
			nl, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil {
				return interp.NewExitStatus(2)
			}
			n = nl
			i++
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			nl, err := strconv.ParseInt(a[2:], 10, 64)
			if err != nil {
				return interp.NewExitStatus(2)
			}
			n = nl
		case a == "--":
		case strings.HasPrefix(a, "-") && a != "-":
			// 其余旗标忽略
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	return headTail(ctx, hc, files, n, false)
}

func cmdTail(ctx context.Context, hc interp.HandlerContext, args []string) error {
	n := int64(10)
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-n" && i+1 < len(args):
			nl, err := strconv.ParseInt(args[i+1], 10, 64)
			if err != nil {
				return interp.NewExitStatus(2)
			}
			n = nl
			i++
		case strings.HasPrefix(a, "-n") && len(a) > 2:
			nl, err := strconv.ParseInt(a[2:], 10, 64)
			if err != nil {
				return interp.NewExitStatus(2)
			}
			n = nl
		case a == "--":
		case strings.HasPrefix(a, "-") && a != "-":
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	return headTail(ctx, hc, files, n, true)
}

func headTail(ctx context.Context, hc interp.HandlerContext, files []string, n int64, tail bool) error {
	multi := len(files) > 1
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, close, err := openReader(hc, f)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "%s: %s: %v\n", map[bool]string{true: "tail", false: "head"}[tail], f, err)
			return interp.NewExitStatus(1)
		}
		if multi {
			fmt.Fprintf(hc.Stdout, "==> %s <==\n", f)
		}
		if tail {
			tailLines(hc.Stdout, r, n)
		} else {
			headLines(hc.Stdout, r, n)
		}
		close()
	}
	return nil
}

func headLines(w io.Writer, r io.Reader, n int64) {
	sc := bufio.NewScanner(r)
	var line int64
	for sc.Scan() {
		if n >= 0 && line >= n {
			return
		}
		line++
		fmt.Fprintln(w, sc.Text())
	}
}

func tailLines(w io.Writer, r io.Reader, n int64) {
	if n < 0 {
		return
	}
	sc := bufio.NewScanner(r)
	ring := make([]string, 0, n)
	for sc.Scan() {
		ring = append(ring, sc.Text())
		if int64(len(ring)) > n {
			ring = ring[1:]
		}
	}
	for _, l := range ring {
		fmt.Fprintln(w, l)
	}
}

// ---------- wc ----------

func cmdWc(ctx context.Context, hc interp.HandlerContext, args []string) error {
	flags := map[byte]bool{}
	var files []string
	for _, a := range args {
		switch {
		case a == "--":
		case strings.HasPrefix(a, "-") && len(a) > 1 && a != "-":
			for _, f := range a[1:] {
				flags[byte(f)] = true
			}
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		files = []string{"-"}
	}
	var tl, tw, tc int64
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, close, err := openReader(hc, f)
		if err != nil {
			fmt.Fprintf(hc.Stderr, "wc: %s: %s\n", f, slashErr(err))
			return interp.NewExitStatus(1)
		}
		l, w, c := countWc(r)
		tl += l
		tw += w
		tc += c
		// 对齐 GNU wc：stdin（"-"）单文件时不回显名字（如 find ... | wc -l 只出计数）
		displayName := f
		if f == "-" && len(files) == 1 {
			displayName = ""
		}
		printWc(hc, flags, l, w, c, displayName)
		close()
	}
	if len(files) > 1 {
		printWc(hc, flags, tl, tw, tc, "total")
	}
	return nil
}

func countWc(r io.Reader) (lines, words, bytes int64) {
	inWord := false
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			b := buf[i]
			bytes++
			if b == '\n' {
				lines++
			}
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
				inWord = false
			} else if !inWord {
				inWord = true
				words++
			}
		}
		if err != nil {
			break
		}
	}
	return
}

func printWc(hc interp.HandlerContext, flags map[byte]bool, l, w, c int64, name string) {
	if len(flags) == 0 {
		if name == "" {
			fmt.Fprintf(hc.Stdout, " %7d %7d %7d\n", l, w, c)
		} else {
			fmt.Fprintf(hc.Stdout, " %7d %7d %7d %s\n", l, w, c, name)
		}
		return
	}
	var parts []string
	if flags['l'] {
		parts = append(parts, strconv.FormatInt(l, 10))
	}
	if flags['w'] {
		parts = append(parts, strconv.FormatInt(w, 10))
	}
	if flags['c'] {
		parts = append(parts, strconv.FormatInt(c, 10))
	}
	if name == "" {
		fmt.Fprintf(hc.Stdout, " %s\n", strings.Join(parts, " "))
	} else {
		fmt.Fprintf(hc.Stdout, " %s %s\n", strings.Join(parts, " "), name)
	}
}

// ---------- tree ----------

// cmdTree 目录树输出（GNU tree 常用子集）：-L n 限制展开层数、-I pattern 排除
// （| 分隔多模式，按 basename 匹配）、-a 含隐藏项、-d 仅目录。进程内实现的原因
// 与其余内建一致：Windows 上 PATH 命中的往往是 C:\Windows\tree.com——不支持
// -L/-I，输出还是系统 OEM 码页编码，且搜索不到文件内容；内建化后跨平台一致。
func cmdTree(ctx context.Context, hc interp.HandlerContext, args []string) error {
	depth := -1 // 无限制
	var excludes []string
	all, dirsOnly := false, false
	rootArg := "."
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-a":
			all = true
		case a == "-d":
			dirsOnly = true
		case a == "--":
		case a == "-L" && i+1 < len(args):
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				fmt.Fprintf(hc.Stderr, "tree: invalid level: %s\n", args[i+1])
				return interp.NewExitStatus(2)
			}
			depth = n
			i++
		case strings.HasPrefix(a, "-L") && len(a) > 2:
			n, err := strconv.Atoi(a[2:])
			if err != nil || n < 1 {
				fmt.Fprintf(hc.Stderr, "tree: invalid level: %s\n", a[2:])
				return interp.NewExitStatus(2)
			}
			depth = n
		case a == "-I" && i+1 < len(args):
			for _, pat := range strings.Split(args[i+1], "|") {
				if pat != "" {
					excludes = append(excludes, pat)
				}
			}
			i++
		case strings.HasPrefix(a, "-") && a != "-":
			fmt.Fprintf(hc.Stderr, "tree: unsupported option: %s\n", a)
			return interp.NewExitStatus(2)
		default:
			rootArg = a
		}
	}
	rootPath := res(hc, rootArg)
	fi, err := os.Stat(rootPath)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "tree: %s: %s\n", rootArg, slashErr(err))
		return interp.NewExitStatus(2)
	}
	if !fi.IsDir() {
		fmt.Fprintf(hc.Stderr, "tree: %s: Not a directory\n", rootArg)
		return interp.NewExitStatus(2)
	}
	fmt.Fprintln(hc.Stdout, rootArg)
	var dirs, files int64
	if err := treeWalk(ctx, hc, rootPath, "", depth, 1, excludes, all, dirsOnly, &dirs, &files); err != nil {
		return err
	}
	fmt.Fprintf(hc.Stdout, "\n%d director%s, %d file%s\n",
		dirs, plural("y", "ies", dirs), files, plural("", "s", files))
	return nil
}

// plural 单复数选词：n==1 取单数词尾，否则复数。
func plural(singular, pluralSuffix string, n int64) string {
	if n == 1 {
		return singular
	}
	return pluralSuffix
}

// treeWalk 递归打印目录树。level 从 1 计（根的直接子项为 1），depth>0 时仅展开
// level<=depth 的目录（更深的目录列出但不展开，对齐 GNU tree -L）。目录在前、
// 按名升序，输出确定。返回 ctx 错误（取消）。
func treeWalk(ctx context.Context, hc interp.HandlerContext, dir, prefix string, depth, level int, excludes []string, all, dirsOnly bool, dirs, files *int64) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(hc.Stdout, "%s[opendir %s]: %s\n", prefix, filepath.ToSlash(dir), slashErr(err))
		return nil
	}
	type treeEntry struct {
		name  string
		isDir bool
	}
	var list []treeEntry
	for _, e := range entries {
		name := e.Name()
		if !all && strings.HasPrefix(name, ".") {
			continue
		}
		if matchAnyPattern(name, excludes) {
			continue
		}
		isDir := e.IsDir()
		if dirsOnly && !isDir {
			continue
		}
		list = append(list, treeEntry{name: name, isDir: isDir})
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].isDir != list[j].isDir {
			return list[i].isDir
		}
		return list[i].name < list[j].name
	})
	for i, it := range list {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := i == len(list)-1
		branch := "├── "
		if last {
			branch = "└── "
		}
		fmt.Fprintln(hc.Stdout, prefix+branch+it.name)
		if !it.isDir {
			*files++
			continue
		}
		*dirs++
		if depth != -1 && level >= depth {
			continue
		}
		child := prefix + "│   "
		if last {
			child = prefix + "    "
		}
		if err := treeWalk(ctx, hc, dsc.PJoin(dir, it.name), child, depth, level+1, excludes, all, dirsOnly, dirs, files); err != nil {
			return err
		}
	}
	return nil
}

// matchAnyPattern basename 是否命中任一排除模式（GNU tree -I 语义，| 已拆分）。
func matchAnyPattern(name string, patterns []string) bool {
	for _, pat := range patterns {
		if ok, _ := filepath.Match(pat, name); ok {
			return true
		}
	}
	return false
}
