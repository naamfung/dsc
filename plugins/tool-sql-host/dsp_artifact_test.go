package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"tool-sql-host/internal/dsp"
)

// shippedExampleDsp 是随插件分发的示例载体（examples/hello 的打包产物）。
var shippedExampleDsp = filepath.Join("dsp", "hello.dsp")

// TestShippedExampleDspMatchesSource 钉死「随包示例载体 == 示例源目录的打包结果」：
// .dsp 是二进制产物，改了 examples/hello 却忘了重新打包，用户拿到的就是过期的载体，
// 而且这种不一致在源码 diff 里看不出来。打包是字节确定的（同名同源 → 同字节），
// 故这里直接逐字节比对，把漂移挡在提交前。
//
// 重新打包：go run ./cmd/dsp-pack -o dsp/hello.dsp ./examples/hello
func TestShippedExampleDspMatchesSource(t *testing.T) {
	fresh := filepath.Join(t.TempDir(), "hello.dsp")
	if _, err := dsp.Pack(filepath.Join("examples", "hello"), fresh, nil); err != nil {
		t.Fatalf("打包示例源目录失败: %v", err)
	}
	want, err := os.ReadFile(fresh)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(shippedExampleDsp)
	if err != nil {
		t.Fatalf("读取随包示例载体失败（%s）: %v", shippedExampleDsp, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("%s 与 examples/hello 的打包结果不一致：源码改了但载体没重打包。\n"+
			"请执行：go run ./cmd/dsp-pack -o %s ./examples/hello", shippedExampleDsp, shippedExampleDsp)
	}
}

// TestShippedExampleDspIsValid 覆盖随包示例载体的格式自证：后缀、SQLite 魔数、
// application_id 与元数据都必须成立（用户开箱即用的那份必须是合法载体）。
func TestShippedExampleDspIsValid(t *testing.T) {
	p, err := dsp.Open(shippedExampleDsp)
	if err != nil {
		t.Fatalf("随包示例载体不合法: %v", err)
	}
	if p.Name != "hello" || p.Language != dsp.LanguageLua || p.ReadOnly {
		t.Fatalf("随包示例载体元数据 = %+v", p)
	}
	if blobs, err := p.Blobs(); err != nil || len(blobs) != 2 {
		t.Fatalf("随包示例载体 blob = %v/%v", blobs, err)
	}
}
