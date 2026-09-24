// dsp-pack 把插件源目录打包为 .dsp 文件（目录 → SQLite 库）。
//
// 角色对齐 SelfDB 的 elf2self：把散落的源码/清单固化成单一自持载体；
// 打包产物可直接放进 tool-sql-host 的插件目录（./dsp）被自动加载。
//
// 用法：
//
//	dsp-pack -o ./dsp/hello.dsp ./examples/hello
//	dsp-pack -name my-tool -o ./dsp/my-tool.dsp ./examples/hello
package main

import (
	"flag"
	"fmt"
	"os"

	"tool-sql-host/internal/dsp"
)

func main() {
	out := flag.String("o", "", "输出 .dsp 文件路径（后缀必须为 .dsp）")
	name := flag.String("name", "", "覆盖插件名（缺省取 dsp.yaml 或源目录名）")
	language := flag.String("language", "", "覆盖载体语言（缺省 lua）")
	entry := flag.String("entry", "", "覆盖入口脚本（缺省 main.lua）")
	description := flag.String("description", "", "覆盖插件描述")
	version := flag.String("version", "", "覆盖插件版本")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"用法: %s -o <输出.dsp> [选项] <插件源目录>\n\n源目录布局：%s（清单，可选）+ 入口脚本（默认 main.lua）"+
				"+ 其他 *.lua（可 require 的脚本 blob）+ 其余文件（可读静态内容）+ %s（自有表建表，可选）\n\n选项：\n",
			os.Args[0], dsp.ManifestFile, dsp.AuthorSchemaFile)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *out == "" || flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	info, err := dsp.Pack(flag.Arg(0), *out, &dsp.Manifest{
		Name: *name, Language: *language, Entry: *entry,
		Description: *description, Version: *version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "dsp-pack: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已打包 %s（%s，%d 字节，%d 个 blob）\n",
		info.Path, info.Name, info.Size, len(info.Blobs))
	for _, b := range info.Blobs {
		fmt.Printf("  - %-16s kind=%-6s %6d 字节\n", b.Name, b.Kind, b.Size)
	}
}
