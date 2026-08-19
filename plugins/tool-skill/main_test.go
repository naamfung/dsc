package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanAndInstall(t *testing.T) {
	src := t.TempDir()
	// 目录布局技能（含资源文件）
	writeTestFile(t, filepath.Join(src, "pkg1", "SKILL.md"), "---\nname: pkg-one\ndescription: 目录布局技能\n---\n正文 A\n")
	writeTestFile(t, filepath.Join(src, "pkg1", "assets", "ref.txt"), "resource")
	// 扁平技能
	writeTestFile(t, filepath.Join(src, "flat.md"), "---\nname: flat-skill\ndescription: 扁平技能\n---\n正文 B\n")
	// 深度 3 的嵌套目录包
	writeTestFile(t, filepath.Join(src, "outer", "inner", "pkg2", "SKILL.md"), "---\nname: pkg-two\ndescription: 嵌套技能\n---\n正文 C\n")
	// 缺 description，应被跳过
	writeTestFile(t, filepath.Join(src, "nodef.md"), "---\nname: no-desc\n---\n正文 D\n")
	// 深度超限，应被跳过
	writeTestFile(t, filepath.Join(src, "deep", "a", "b", "c", "tooldeep.md"), "---\nname: too-deep\ndescription: x\n---\n正文\n")

	builtinDir := t.TempDir()
	installedDir := t.TempDir()
	store := NewSkillStore(builtinDir, installedDir)
	tool := &InstallSkillTool{store: store, installedDir: installedDir}

	cands := scanSkillDir(src)
	if len(cands) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %+v", len(cands), cands)
	}

	for _, c := range cands {
		if err := tool.installCandidate(c); err != nil {
			t.Fatalf("install %s: %v", c.Name, err)
		}
	}

	// 校验拷贝结果：目录布局整包拷贝、扁平转目录布局（均在 installed 目录下）
	for _, want := range []string{
		filepath.Join(installedDir, "pkg-one", "SKILL.md"),
		filepath.Join(installedDir, "pkg-one", "assets", "ref.txt"),
		filepath.Join(installedDir, "flat-skill", "SKILL.md"),
		filepath.Join(installedDir, "pkg-two", "SKILL.md"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("missing installed file %s: %v", want, err)
		}
	}
	// 缺 description 的不应被安装
	if _, err := os.Stat(filepath.Join(installedDir, "no-desc")); err == nil {
		t.Fatal("no-desc skill should not be installed")
	}

	// 刷新存储后 read_skill 可读取，且作用域为外置
	store.load()
	if sk, ok := store.get("flat-skill"); !ok || sk.Body != "正文 B" || sk.Scope != ScopeInstalled {
		t.Fatalf("read_skill lookup failed: ok=%v body=%q scope=%q", ok, sk.Body, sk.Scope)
	}
}

func TestUninstall(t *testing.T) {
	builtinDir := t.TempDir()
	installedDir := t.TempDir()
	// 内置技能（不可卸载）
	writeTestFile(t, filepath.Join(builtinDir, "git-commit", "SKILL.md"), "---\nname: git-commit\ndescription: 内置技能\n---\n正文\n")
	// 外置技能（可卸载）
	writeTestFile(t, filepath.Join(installedDir, "flat-skill", "SKILL.md"), "---\nname: flat-skill\ndescription: 外置技能\n---\n正文 B\n")

	store := NewSkillStore(builtinDir, installedDir)
	if len(store.skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(store.skills))
	}
	if sk, _ := store.get("git-commit"); sk.Scope != ScopeBuiltin {
		t.Fatalf("git-commit should be builtin, got %q", sk.Scope)
	}

	// 内置技能不可卸载
	if err := store.removeInstalled("git-commit"); err == nil {
		t.Fatal("builtin skill should not be uninstallable")
	}
	// 外置技能可卸载
	if err := store.removeInstalled("flat-skill"); err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	store.load()
	if _, ok := store.get("flat-skill"); ok {
		t.Fatal("flat-skill should be removed from store")
	}
	if _, err := os.Stat(filepath.Join(installedDir, "flat-skill")); err == nil {
		t.Fatal("installed skill dir should be removed")
	}
	// 内置技能不受影响
	if _, ok := store.get("git-commit"); !ok {
		t.Fatal("builtin git-commit should remain")
	}
}

func TestCandidateFromFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "foo", "SKILL.md"), "---\nname: bar\ndescription: d\n---\nbody\n")
	writeTestFile(t, filepath.Join(dir, "single.md"), "---\nname: s1\ndescription: d\n---\nbody\n")

	c, ok := candidateFromFile(filepath.Join(dir, "foo", "SKILL.md"))
	if !ok || c.Name != "bar" {
		t.Fatalf("SKILL.md candidate wrong: %+v ok=%v", c, ok)
	}
	c, ok = candidateFromFile(filepath.Join(dir, "single.md"))
	if !ok || c.Name != "s1" {
		t.Fatalf("flat candidate wrong: %+v ok=%v", c, ok)
	}
}
