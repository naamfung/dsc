package dsp

import (
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dsc "dsc-sdk"
	"gopkg.in/yaml.v3"
)

// ManifestFile 是打包源目录内的清单文件名。
const ManifestFile = "dsp.yaml"

// AuthorSchemaFile 是打包源目录内可选的作者建表脚本（插件自有表结构）。
// 运行期禁止 DDL（见 GuardSQL），故自有表一律在打包期落库。
const AuthorSchemaFile = "schema.sql"

// MetaSchema 是 dsp_meta 中记录作者建表脚本的键（留档以便审计与重建）。
const MetaSchema = "schema"

// Manifest 描述一个待打包插件（源目录内 dsp.yaml）。
// 全部字段可选：缺省时 name 取目录基名、language 取 lua、entry 取 main.lua。
type Manifest struct {
	Name        string `yaml:"name"`
	Language    string `yaml:"language"`
	Entry       string `yaml:"entry"`
	Description string `yaml:"description"`
	Version     string `yaml:"version"`
}

// PackInfo 是一次打包的结果摘要。
type PackInfo struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Language string     `json:"language"`
	Entry    string     `json:"entry"`
	Blobs    []BlobInfo `json:"blobs"`
	Size     int64      `json:"size"`
}

// Pack 把一个源目录打包为 .dsp 文件（目录 → SQLite 库）：
//
//	<src>/dsp.yaml    清单（可选）
//	<src>/main.lua    入口脚本（blob 名 = manifest.entry，kind=script）
//	<src>/**.lua      其余 Lua 文件（kind=script，可经 dsc.dsp.require 加载）
//	<src>/其他文件    静态内容（kind=asset，可经 dsc.dsp.asset 读取）
//
// override 可覆盖清单中的非空字段（供调用方在不改源目录的前提下指定插件名等）；
// 传 nil 表示完全以 dsp.yaml 与默认值为准。
//
// 产物先写同目录临时文件再原子改名，避免半成品 .dsp 被加载器看到。
func Pack(srcDir, outPath string, override *Manifest) (*PackInfo, error) {
	if err := ValidatePath(outPath); err != nil {
		return nil, err
	}
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("插件源目录不可用: %s", srcDir)
	}

	mf, err := readManifest(srcDir)
	if err != nil {
		return nil, err
	}
	applyOverride(mf, override)
	if err := ValidateName(mf.Name); err != nil {
		return nil, err
	}
	entryPath := dsc.PJoin(srcDir, mf.Entry)
	entryData, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, fmt.Errorf("读取入口脚本 %s 失败: %w", mf.Entry, err)
	}

	meta := map[string]string{
		MetaName:        mf.Name,
		MetaLanguage:    mf.Language,
		MetaEntry:       mf.Entry,
		MetaDescription: mf.Description,
		MetaVersion:     mf.Version,
	}
	if strings.EqualFold(mf.Language, LanguageLua) && !strings.HasSuffix(strings.ToLower(mf.Entry), ".lua") {
		return nil, fmt.Errorf("lua 插件的入口须为 .lua 文件，得到 %q", mf.Entry)
	}

	absOut, err := dsc.PAbs(outPath)
	if err != nil {
		return nil, err
	}
	outDir := dsc.PDir(absOut)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(outDir, "."+filepath.Base(absOut)+".tmp-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath) // 失败路径清理；成功路径已被 Rename 移走

	db, err := openDB(dsc.PClean(tmpPath))
	if err != nil {
		return nil, err
	}
	if err := writeDatabase(db, meta, mf, entryData, srcDir, absOut); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	// 原子替换：先删目标再改名的窗口极小，且加载器按内容哈希识别，短暂缺失不会误判。
	if err := os.Rename(tmpPath, absOut); err != nil {
		return nil, fmt.Errorf("写出 %s 失败: %w", absOut, err)
	}

	stat, err := os.Stat(absOut)
	if err != nil {
		return nil, err
	}
	p, err := Open(absOut)
	if err != nil {
		return nil, err
	}
	blobs, err := p.Blobs()
	if err != nil {
		return nil, err
	}
	return &PackInfo{
		Name: mf.Name, Path: absOut, Language: mf.Language,
		Entry: mf.Entry, Blobs: blobs, Size: stat.Size(),
	}, nil
}

// applyOverride 用非空字段覆盖清单（调用方显式指定优先于 dsp.yaml 与默认值）。
func applyOverride(mf, override *Manifest) {
	if override == nil {
		return
	}
	if override.Name != "" {
		mf.Name = override.Name
	}
	if override.Language != "" {
		mf.Language = override.Language
	}
	if override.Entry != "" {
		mf.Entry = dsc.PClean(filepath.ToSlash(override.Entry))
	}
	if override.Description != "" {
		mf.Description = override.Description
	}
	if override.Version != "" {
		mf.Version = override.Version
	}
}

// readManifest 读取清单，缺省字段按目录名与 Lua 约定补齐。
func readManifest(srcDir string) (*Manifest, error) {
	mf := &Manifest{}
	raw, err := os.ReadFile(dsc.PJoin(srcDir, ManifestFile))
	switch {
	case err == nil:
		if err := yaml.Unmarshal(raw, mf); err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", ManifestFile, err)
		}
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("读取 %s 失败: %w", ManifestFile, err)
	}
	if mf.Name == "" {
		mf.Name = filepath.Base(strings.TrimRight(srcDir, `\/`))
	}
	if err := ValidateName(mf.Name); err != nil {
		return nil, err
	}
	if mf.Language == "" {
		mf.Language = LanguageLua
	}
	if mf.Entry == "" {
		mf.Entry = "main.lua"
	}
	mf.Entry = dsc.PClean(filepath.ToSlash(mf.Entry))
	return mf, nil
}

// writeDatabase 建表并写入元数据、入口与其他 blob。
func writeDatabase(db *sql.DB, meta map[string]string, mf *Manifest, entry []byte, srcDir, absOut string) error {
	for _, ddl := range schemaDDL {
		if _, err := db.Exec(ddl); err != nil {
			return err
		}
	}
	authorSchema, err := applyAuthorSchema(db, srcDir)
	if err != nil {
		return err
	}
	if authorSchema != "" {
		meta[MetaSchema] = authorSchema
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, k := range sortedKeys(meta) {
		if _, err := tx.Exec("INSERT INTO "+TableMeta+"(key, value) VALUES(?, ?)", k, meta[k]); err != nil {
			return err
		}
	}
	insert := "INSERT INTO " + TableBlobs + "(name, kind, language, content) VALUES(?, ?, ?, ?)"
	if _, err := tx.Exec(insert, mf.Entry, KindScript, mf.Language, entry); err != nil {
		return err
	}

	extra, err := collectBlobs(srcDir, absOut, mf.Entry)
	if err != nil {
		return err
	}
	for _, b := range extra {
		if _, err := tx.Exec(insert, b.name, b.kind, b.language, b.content); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// 自证身份：后缀之外再靠库头 application_id 认领「我是 .dsp」。
	if _, err := db.Exec(fmt.Sprintf("PRAGMA application_id = %d", ApplicationID)); err != nil {
		return err
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
		return err
	}
	return nil
}

type pendingBlob struct {
	name     string
	kind     string
	language string
	content  []byte
}

// applyAuthorSchema 执行作者 schema.sql（若存在）并返回其原文。
// 预留表已先建好，作者脚本可安全引用；逐条执行（见 SplitStatements）。
func applyAuthorSchema(db *sql.DB, srcDir string) (string, error) {
	raw, err := os.ReadFile(dsc.PJoin(srcDir, AuthorSchemaFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	text := string(raw)
	for _, stmt := range SplitStatements(text) {
		if _, err := db.Exec(stmt); err != nil {
			return "", fmt.Errorf("执行 %s 失败（%s）: %w", AuthorSchemaFile, firstLine(stmt), err)
		}
	}
	return text, nil
}

// firstLine 取语句首行（错误信息里定位到具体那一条）。
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// collectBlobs 收集入口之外的源文件：Lua 文件按脚本收录（可 require），其余按静态内容收录。
// 顺序按相对路径升序，保证同一份源目录产出字节稳定的 .dsp。
// 打包产物自身若落在源目录内，按相对路径排除（否则会把上一次的 .dsp 也打进去）。
func collectBlobs(srcDir, absOut, entryName string) ([]pendingBlob, error) {
	outAbs, _ := dsc.PRel(srcDir, absOut)
	var out []pendingBlob
	err := dsc.PWalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := dsc.PRel(srcDir, path)
		if err != nil {
			return err
		}
		name := rel
		if name == ManifestFile || name == AuthorSchemaFile || name == entryName || (outAbs != "" && name == outAbs) {
			return nil
		}
		if strings.EqualFold(filepath.Ext(name), Ext) {
			return nil // 不把嵌套 .dsp 当内容打包
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		kind, lang := KindAsset, ""
		if strings.EqualFold(filepath.Ext(name), ".lua") {
			kind, lang = KindScript, LanguageLua
		}
		out = append(out, pendingBlob{name: name, kind: kind, language: lang, content: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
