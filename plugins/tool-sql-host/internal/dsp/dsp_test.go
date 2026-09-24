package dsp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

// writeSource 在临时目录写一份最小插件源（dsp.yaml + main.lua）。
func writeSource(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		ManifestFile: "name: demo\nlanguage: lua\nentry: main.lua\ndescription: 测试\n",
		"main.lua":   `dsc.register_tool("x", { description = "x" }, function() return "x" end)`,
		"lib.lua":    "return {}\n",
		"data.txt":   "静态内容\n",
		AuthorSchemaFile: `CREATE TABLE note (
    id   INTEGER PRIMARY KEY,
    text TEXT NOT NULL
);
`,
	}
	for name, content := range files {
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestValidatePath 覆盖 .dsp 后缀强制校验（格式第一道门禁）。
func TestValidatePath(t *testing.T) {
	ok := []string{"a.dsp", "plugins/x/y.dsp", "tool-1.dsp", "A.DSP", "demo_1.dsp"}
	bad := []string{"", "a", "a.sqlite", "a.db", "a.dsp.txt", "a.txt", "a.dsp/", "bad name.dsp", "a+b.dsp", "有中文.dsp"}
	for _, p := range ok {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) 应通过，得到 %v", p, err)
		}
	}
	for _, p := range bad {
		err := ValidatePath(p)
		if err == nil {
			t.Errorf("ValidatePath(%q) 应拒绝", p)
			continue
		}
		if !errors.Is(err, ErrNotDsp) {
			t.Errorf("ValidatePath(%q) 错误未包裹 ErrNotDsp: %v", p, err)
		}
	}
}

// TestOpenRejectsForeignFiles 覆盖「实质须为 SQLite 库 + 自证身份」的校验：
// 非 SQLite 文件、SQLite 但 application_id 不符、无 dsp_meta 者一律拒绝。
func TestOpenRejectsForeignFiles(t *testing.T) {
	dir := t.TempDir()

	// 1. 纯文本伪装 .dsp
	fake := dir + "/fake.dsp"
	if err := os.WriteFile(fake, []byte(strings.Repeat("hello", 40)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(fake); !errors.Is(err, ErrNotDsp) {
		t.Fatalf("非 SQLite 文件应被拒绝（ErrNotDsp），得到 %v", err)
	}

	// 2. 真 SQLite 但未认领 application_id
	plain := dir + "/plain.dsp"
	db, err := openDB(plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE t(a TEXT)"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(plain); !errors.Is(err, ErrNotDsp) {
		t.Fatalf("application_id 不符应被拒绝，得到 %v", err)
	}

	// 3. 认领了 application_id 但缺 dsp_meta
	meta, err := openDB(plain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meta.Exec("PRAGMA application_id = " + strconv.Itoa(int(ApplicationID))); err != nil {
		t.Fatal(err)
	}
	meta.Close()
	if _, err := Open(plain); !errors.Is(err, ErrNotDsp) {
		t.Fatalf("缺 dsp_meta 应被拒绝，得到 %v", err)
	}

	// 4. 后缀不符（即使内容合法）
	if err := os.WriteFile(dir+"/ok.sqlite", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir + "/ok.sqlite"); !errors.Is(err, ErrNotDsp) {
		t.Fatalf("非 .dsp 后缀应被拒绝，得到 %v", err)
	}
}

// TestPackOpenRoundTrip 覆盖「目录 → .dsp → 打开」的字段保真往返：
// 元数据、入口脚本、其余脚本与静态内容都必须逐字段存活。
func TestPackOpenRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	out := dir + "/demo.dsp"

	info, err := Pack(src, out, nil)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if info.Name != "demo" || info.Entry != "main.lua" || info.Language != "lua" {
		t.Fatalf("PackInfo = %+v", info)
	}

	p, err := Open(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if p.Name != "demo" || p.Language != "lua" || p.Entry != "main.lua" {
		t.Fatalf("打开后的元数据 = %+v", p)
	}
	if p.ReadOnly {
		t.Fatal("新打包的 .dsp 不应只读")
	}
	if p.MetaValue(MetaDescription) != "测试" {
		t.Fatalf("description 未保真: %q", p.MetaValue(MetaDescription))
	}

	// 入口脚本内容逐字节保真
	raw, ok, err := p.Blob("main.lua", KindScript)
	if err != nil || !ok {
		t.Fatalf("入口 blob: ok=%v err=%v", ok, err)
	}
	orig, _ := os.ReadFile(src + "/main.lua")
	if string(raw) != string(orig) {
		t.Fatalf("入口脚本内容未保真:\n%q\n%q", raw, orig)
	}

	// 其余脚本/静态内容按 kind 分类存在
	if _, ok, _ := p.Blob("lib.lua", KindScript); !ok {
		t.Fatal("lib.lua 应以 script 收录")
	}
	if _, ok, _ := p.Blob("data.txt", KindAsset); !ok {
		t.Fatal("data.txt 应以 asset 收录")
	}
	if _, ok, _ := p.Blob(AuthorSchemaFile, KindAsset); ok {
		t.Fatal("schema.sql 已转写进 dsp_meta，不应再作为 asset 收录")
	}
	if p.MetaValue(MetaSchema) == "" {
		t.Fatal("作者建表脚本应留档在 dsp_meta.schema")
	}

	// 作者自有表在打包期已落库
	tables, err := p.Tables()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(tables, "note") {
		t.Fatalf("作者 schema.sql 未生效，表 = %v", tables)
	}
}

// TestStatePersistsAcrossOpen 覆盖插件自持状态随文件落盘：写入后关闭再打开仍在。
func TestStatePersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	out := dir + "/demo.dsp"
	if _, err := Pack(src, out, nil); err != nil {
		t.Fatal(err)
	}

	p, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.StateSet("counter", 41); err != nil {
		t.Fatalf("StateSet: %v", err)
	}
	if err := p.StateSet("obj", map[string]any{"a": 1.0}); err != nil {
		t.Fatalf("StateSet obj: %v", err)
	}
	if err := p.StateSet("gone", "bye"); err != nil {
		t.Fatal(err)
	}
	if err := p.StateDelete("gone"); err != nil {
		t.Fatal(err)
	}
	keys, _ := p.StateKeys()
	if p.CountState() != 2 || !contains(keys, "counter") || contains(keys, "gone") {
		t.Fatalf("状态键 = %v（count=%d）", keys, p.CountState())
	}
	reopened, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	v, ok, err := reopened.StateGet("counter")
	if err != nil || !ok {
		t.Fatalf("StateGet counter: ok=%v err=%v", ok, err)
	}
	if n, ok := v.(float64); !ok || n != 41 {
		t.Fatalf("counter = %#v, want 41", v)
	}
	obj, ok, _ := reopened.StateGet("obj")
	m, isMap := obj.(map[string]any)
	if !ok || !isMap || m["a"] != 1.0 {
		t.Fatalf("obj = %#v", obj)
	}
	if _, ok, _ := reopened.StateGet("gone"); ok {
		t.Fatal("已删除的键不应存在")
	}
}

// TestPackIsReproducible 覆盖「同一份源目录 → 字节一致的 .dsp」：
// 仓库内提交的示例载体因此可被测试钉死，避免源码改了而 .dsp 忘了重打包。
func TestPackIsReproducible(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	a, b := dir+"/a.dsp", dir+"/b.dsp"
	if _, err := Pack(src, a, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(src, b, nil); err != nil {
		t.Fatal(err)
	}
	ha, err := fileSHA256(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, _ := fileSHA256(b)
	if ha != hb {
		t.Fatalf("两次打包结果不一致（%s vs %s）", ha, hb)
	}
}

// TestPackRequiresDspExtension 覆盖打包入口的后缀强制校验。
func TestPackRequiresDspExtension(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	if _, err := Pack(src, dir+"/demo.sqlite", nil); !errors.Is(err, ErrNotDsp) {
		t.Fatalf("非 .dsp 输出应被拒绝，得到 %v", err)
	}
}

// TestPackLuaEntryMustBeLua 覆盖载体语言的打包期取舍：lua 插件的入口必须是 .lua，
// 否则产出的载体无法被加载器执行。
func TestPackLuaEntryMustBeLua(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	if _, err := Pack(src, dir+"/demo.dsp", &Manifest{Name: "demo", Language: "lua", Entry: "main.txt"}); err == nil {
		t.Fatal("lua 插件入口非 .lua 应被拒绝")
	}
}

// TestQueryExecAndGuard 覆盖脚本侧 SQL：能读写自有表，且被守卫拦下的语句一律拒绝。
func TestQueryExecAndGuard(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	out := dir + "/demo.dsp"
	if _, err := Pack(src, out, nil); err != nil {
		t.Fatal(err)
	}
	p, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec("INSERT INTO note(id, text) VALUES(?, ?)", []any{1, "第一条"}); err != nil {
		t.Fatalf("插入自有表失败: %v", err)
	}
	rows, err := p.Query("SELECT id, text FROM note WHERE id = ?", []any{1})
	if err != nil {
		t.Fatalf("查询自有表失败: %v", err)
	}
	if len(rows) != 1 || rows[0]["text"] != "第一条" {
		t.Fatalf("查询结果 = %#v", rows)
	}

	// 保留表：读可以，写不行
	if _, err := p.Query("SELECT key, value FROM dsp_meta", nil); err != nil {
		t.Fatalf("读保留表应允许: %v", err)
	}
	if _, err := p.Exec("UPDATE dsp_meta SET value = 'x' WHERE key = 'entry'", nil); err == nil {
		t.Fatal("写 dsp_meta 应被拒绝")
	}
}

// fileSHA256 返回文件字节的 sha256（与 dsp 产物哈希无关：此处校验的是打包的字节确定性）。
func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// TestArtifactHashTracksCodeNotState 是「状态能安全落在同一个文件里」的前提：
// 产物哈希只覆盖元数据与代码 blob，故插件写自己的状态不会改变它（不然宿主每 2s
// 都会把状态写入误判为重打包，反复卸载重载自己的插件）。
func TestArtifactHashTracksCodeNotState(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	out := dir + "/demo.dsp"
	if _, err := Pack(src, out, nil); err != nil {
		t.Fatal(err)
	}

	before, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Artifact) != 64 {
		t.Fatalf("产物哈希 = %q", before.Artifact)
	}
	if err := before.StateSet("counter", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := before.Exec("INSERT INTO note(id, text) VALUES(1, 'x')", nil); err != nil {
		t.Fatal(err)
	}
	after, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if after.Artifact != before.Artifact {
		t.Fatalf("写状态/自有表改变了产物哈希：%s → %s", before.Artifact, after.Artifact)
	}

	// 改写代码后产物哈希必须变化（宿主据此重载）
	if err := os.WriteFile(src+"/main.lua", []byte(`dsc.register_tool("y", {}, function() return "y" end)`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(src, out, nil); err != nil {
		t.Fatal(err)
	}
	changed, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Artifact == before.Artifact {
		t.Fatal("改写代码后产物哈希未变化，宿主将无法感知重打包")
	}
}

// TestReadOnlyPluginDegradesWrites 覆盖只读载体的优雅降级：读操作照常，
// 写状态给出明确错误（而不是静默丢弃或让插件在中途崩掉）。
func TestReadOnlyPluginDegradesWrites(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/demo"
	writeSource(t, src)
	out := dir + "/demo.dsp"
	if _, err := Pack(src, out, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(out, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(out, 0o644) })

	p, err := Open(out)
	if err != nil {
		t.Fatalf("只读载体应仍可打开: %v", err)
	}
	if !p.ReadOnly {
		t.Fatal("只读文件的 .dsp 应被标记为 ReadOnly")
	}
	if _, err := p.Query("SELECT COUNT(*) FROM note", nil); err != nil {
		t.Fatalf("只读载体应可查询: %v", err)
	}
	if err := p.StateSet("k", "v"); err == nil {
		t.Fatal("只读载体写状态应报错")
	}
	if _, err := p.Exec("INSERT INTO note(id, text) VALUES(1, 'x')", nil); err == nil {
		t.Fatal("只读载体写自有表应报错")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
