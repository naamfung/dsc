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
	"sort"
	"strconv"
	"strings"
	"time"

	"mvdan.cc/sh/v3/interp"
)

// defaultExecHandler 保持「未命中内部工具」时原有的 PATH 外部程序执行语义
// （mvdan/sh 默认 kill timeout 为 2s，对齐未设置 ExecHandler 前的库默认行为）。
var defaultExecHandler = interp.DefaultExecHandler(2 * time.Second)

// shellExecHandler 是 interp.ExecHandler 的入口：命中内部工具表走进程内实现，
// 否则回退默认外部执行。
func shellExecHandler(ctx context.Context, args []string) error {
	if fn, ok := internalCommands[args[0]]; ok {
		hc := interp.HandlerCtx(ctx)
		return fn(ctx, hc, args[1:])
	}
	return defaultExecHandler(ctx, args)
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
}

// ---------- 共享辅助 ----------

// res 把参数里的路径解析为绝对路径（相对路径基于 hc.Dir，即 shell 当前工作目录）。
func res(hc interp.HandlerContext, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(hc.Dir, p)
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
	var showAll, long, listSelf bool
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
			fmt.Fprintf(hc.Stderr, "ls: cannot access '%s': %v\n", p0, err)
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
			if err := lsDir(ctx, hc, p, showAll, long); err != nil {
				return err
			}
		} else {
			lsEntry(hc, fi, p0, long)
		}
	}
	if exit != 0 {
		return interp.NewExitStatus(2)
	}
	return nil
}

func lsDir(ctx context.Context, hc interp.HandlerContext, p string, showAll, long bool) error {
	entries, err := os.ReadDir(p)
	if err != nil {
		fmt.Fprintf(hc.Stderr, "ls: %v\n", err)
		return interp.NewExitStatus(2)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !showAll && strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if long {
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			fi, err := os.Lstat(filepath.Join(p, name))
			if err != nil {
				continue
			}
			lsEntry(hc, fi, name, true)
		}
	} else {
		fmt.Fprintln(hc.Stdout, strings.Join(names, "  "))
	}
	return nil
}

func lsEntry(hc interp.HandlerContext, fi os.FileInfo, name string, long bool) {
	if long {
		sz := fi.Size()
		t := fi.ModTime().Format("Jan _2 15:04")
		fmt.Fprintf(hc.Stdout, "%s %8d %s %s\n", fi.Mode().String(), sz, t, name)
	} else {
		fmt.Fprintln(hc.Stdout, name)
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
			fmt.Fprintf(hc.Stderr, "rm: cannot remove '%s': %v\n", f, statErr)
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
			dstPath = filepath.Join(dstPath, filepath.Base(src))
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
			fmt.Fprintf(hc.Stderr, "%s: %s -> %s: %v\n", verb, s, dst, errOp)
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
		return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(src, p)
			if rel == "." {
				return os.MkdirAll(dst, 0o755)
			}
			dp := filepath.Join(dst, rel)
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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
			fmt.Fprintf(hc.Stderr, "wc: %s: %v\n", f, err)
			return interp.NewExitStatus(1)
		}
		l, w, c := countWc(r)
		tl += l
		tw += w
		tc += c
		printWc(hc, flags, l, w, c, f)
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
		fmt.Fprintf(hc.Stdout, " %7d %7d %7d %s\n", l, w, c, name)
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
	fmt.Fprintf(hc.Stdout, " %s %s\n", strings.Join(parts, " "), name)
}
