// Package dsp 定义并实现 .dsp（dsc's plugin）插件文件格式。
//
// 一个 .dsp 文件是一个 **SQLite 数据库**：插件的元数据、代码与运行期状态同处一库，
// 即「程序即数据」。加载它的宿主是 tool-sql-host（Go 进程），因此不需要可执行位、
// 也不需要内核 binfmt_misc 之类的机制——执行主体是宿主进程，文件只承载内容。
//
// 强制不变量（Open 时逐条校验，任一不满足即拒绝加载）：
//
//  1. 路径后缀必须为 Ext（.dsp）。
//  2. 文件头 16 字节必须是 SQLite 3 魔数。
//  3. SQLite application_id 必须是 ApplicationID（ASCII "DSP1"）。
//  4. 必须存在 dsp_meta 表，且含 name（插件名）与 language（载体语言）。
//
// 库内保留表（统一 dsp_ 前缀，避免与插件作者自建表冲突）：
//
//	dsp_meta   插件元数据（键值对：name/language/entry/description/version/schema…）
//	dsp_blobs  代码与静态内容（kind: script|asset）
//	dsp_state  插件自持状态（KV，插件运行期读写自身）
//	dsp_log    插件日志（可选，审计用）
//
// 除保留表外，插件作者可在同一库内自由建表（由打包期 schema.sql 落库），
// 并经 dsc.sql.* 读写——这正是「插件读写自身作为数据存取」的落点。
//
// 连接策略：**不长期持有句柄**，每次操作开一次短连接即用即关。三个理由：
//
//  1. 状态就写在载体文件里，长期持有会让文件在 Windows 上无法被替换（重打包/热更新）；
//  2. 释放句柄后，作者可以用 sqlite3 CLI 等外部工具直接检查/修改这个文件；
//  3. 单次操作的开销（毫秒级）远小于一次工具调用的量级。
//
// 变更检测用 ArtifactHash（只覆盖元数据与代码 blob，不含状态），因此插件写自己的
// 状态不会被误判为「载体变了」——这也是状态能安全落在同一个文件里的前提。
package dsp

import (
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	dsc "dsc-sdk"
	_ "modernc.org/sqlite" // 纯 Go SQLite 驱动（无 CGO，可交叉编译）
)

const (
	// Ext 是 .dsp 插件文件的强制后缀（dsc's plugin）。
	Ext = ".dsp"

	// ApplicationID 是 .dsp 文件的 SQLite application_id，取值 ASCII "DSP1"。
	// 由 SelfDB 的 SELF 魔数（0x53454C46）思路而来：靠库头自证身份，而非只靠后缀。
	ApplicationID = 0x44535031

	// SchemaVersion 是当前 dsp 库结构版本，写入 PRAGMA user_version。
	SchemaVersion = 1

	// sqliteMagic 是 SQLite 3 文件头的前 16 字节。
	sqliteMagic = "SQLite format 3\x00"
	// sqliteHeaderSize 是 SQLite 库头长度（application_id 落在其中）。
	sqliteHeaderSize = 100
	// appIDOffset 是库头内 application_id 字段的字节偏移（大端 uint32）。
	appIDOffset = 68

	// 保留表名。
	TableMeta  = "dsp_meta"
	TableBlobs = "dsp_blobs"
	TableState = "dsp_state"
	TableLog   = "dsp_log"

	// 元数据键。
	MetaName        = "name"
	MetaLanguage    = "language"
	MetaEntry       = "entry"
	MetaDescription = "description"
	MetaVersion     = "version"

	// LanguageLua 是目前唯一受支持的载体语言；其代码以 dsp_blobs.kind='script' 存放。
	LanguageLua = "lua"

	// KindScript 是可执行脚本 blob；KindAsset 是可读静态内容 blob。
	KindScript = "script"
	KindAsset  = "asset"
)

// nameRE 约束插件名（用作工具前缀与文件基名的一部分）。
var nameRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// busyTimeoutMS 是每次连接申请写锁的等待上限（同进程内串行写入，仅防外部工具占用）。
const busyTimeoutMS = 5000

// ErrNotDsp 表示文件不是合法的 .dsp 插件（后缀、魔数、application_id 或元数据不符）。
var ErrNotDsp = errors.New("not a dsp plugin")

// ValidatePath 强制校验插件文件后缀必须是 .dsp（大小写不敏感）。
// 这是 .dsp 格式的第一道门禁：所有打开/打包入口都先经此校验。
func ValidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: 插件路径为空", ErrNotDsp)
	}
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, Ext) {
		return fmt.Errorf("%w: 插件文件后缀必须为 %s，得到 %q（%s）", ErrNotDsp, Ext, ext, path)
	}
	base := strings.TrimSuffix(filepath.Base(path), ext)
	if !nameRE.MatchString(base) {
		return fmt.Errorf("%w: 插件文件基名 %q 非法（仅允许 [A-Za-z0-9_-]，长度 1-64）", ErrNotDsp, base)
	}
	return nil
}

// ValidateName 校验插件名合法性（取自 dsp_meta.name 或文件基名）。
func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("%w: 插件名 %q 非法（仅允许 [A-Za-z0-9_-]，长度 1-64）", ErrNotDsp, name)
	}
	return nil
}

// Plugin 是一个已校验的 .dsp 插件的描述（不含长连接；每次操作开短连接）。
type Plugin struct {
	Path     string // 规范化后的绝对路径
	Name     string // 插件名（dsp_meta.name，缺省取文件基名）
	Language string // 载体语言（dsp_meta.language）
	Entry    string // 入口 blob 名（dsp_meta.entry）
	ReadOnly bool   // 库不可写（如文件只读）时为 true
	Artifact string // 产物哈希（覆盖元数据与全部 blob，不含状态；见 artifactHash）

	meta map[string]string // 打开时的元数据快照
}

// Open 校验并读取一个 .dsp 插件的元数据与产物哈希（用完即关句柄，不长期持有）。
//
// 校验顺序：后缀 → 文件存在且为普通文件 → SQLite 魔数 → application_id →
// 必需元数据表。任一步失败返回包裹 ErrNotDsp 的错误，调用方据此跳过该文件。
func Open(path string) (*Plugin, error) {
	if err := ValidatePath(path); err != nil {
		return nil, err
	}
	abs, err := dsc.PAbs(path)
	if err != nil {
		return nil, fmt.Errorf("%w: 解析绝对路径失败: %v", ErrNotDsp, err)
	}
	if err := verifyHeader(abs); err != nil {
		return nil, err
	}
	p := &Plugin{Path: abs}
	err = p.withDB(func(db *sql.DB) error {
		meta, err := readMeta(db)
		if err != nil {
			return err
		}
		p.meta = meta
		if err := p.applyMeta(); err != nil {
			return err
		}
		p.ReadOnly = !writable(db)
		if p.ReadOnly {
			return nil
		}
		if err := ensureRuntimeTables(db); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 产物哈希单独一次读取：需要读全部 blob，且判定「载体是否变化」与可写性无关。
	hash, err := p.artifactHash()
	if err != nil {
		return nil, err
	}
	p.Artifact = hash
	return p, nil
}

// withDB 以短连接执行 fn：开库 → 设 PRAGMA → fn → 关库。
func (p *Plugin) withDB(fn func(*sql.DB) error) error {
	db, err := sql.Open("sqlite", p.Path)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)); err != nil {
		return err
	}
	return fn(db)
}

// verifyHeader 校验 SQLite 魔数与 application_id（不打开库，纯字节检查）。
func verifyHeader(abs string) error {
	f, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("%w: 打开失败: %v", ErrNotDsp, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("%w: 取文件信息失败: %v", ErrNotDsp, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: 不是普通文件", ErrNotDsp)
	}
	if info.Size() < sqliteHeaderSize {
		return fmt.Errorf("%w: 文件过小（%d 字节），不是 SQLite 库", ErrNotDsp, info.Size())
	}
	head := make([]byte, sqliteHeaderSize)
	if _, err := f.Read(head); err != nil {
		return fmt.Errorf("%w: 读取文件头失败: %v", ErrNotDsp, err)
	}
	if string(head[:16]) != sqliteMagic {
		return fmt.Errorf("%w: 文件头不是 SQLite 3 魔数（.dsp 实质须为 SQLite 数据库）", ErrNotDsp)
	}
	if got := binary.BigEndian.Uint32(head[appIDOffset : appIDOffset+4]); got != ApplicationID {
		return fmt.Errorf("%w: application_id = 0x%08X，须为 0x%08X（ASCII %q）",
			ErrNotDsp, got, ApplicationID, "DSP1")
	}
	return nil
}

// applyMeta 由元数据快照推导 Name/Language/Entry；language 缺失即视为非法 .dsp。
func (p *Plugin) applyMeta() error {
	name := p.meta[MetaName]
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(p.Path), Ext)
	}
	if err := ValidateName(name); err != nil {
		return err
	}
	lang := p.meta[MetaLanguage]
	if lang == "" {
		return fmt.Errorf("%w: dsp_meta 缺 %s（载体语言）", ErrNotDsp, MetaLanguage)
	}
	p.Name = name
	p.Language = lang
	p.Entry = p.meta[MetaEntry]
	if p.Entry == "" {
		p.Entry = "main.lua"
	}
	return nil
}

// MetaSnapshot 返回打开时读取的元数据快照（副本；不需再访问库）。
func (p *Plugin) MetaSnapshot() map[string]string {
	out := make(map[string]string, len(p.meta))
	for k, v := range p.meta {
		out[k] = v
	}
	return out
}

// MetaValue 取单个元数据值（快照）。
func (p *Plugin) MetaValue(key string) string { return p.meta[key] }

// readMeta 读取 dsp_meta 全表；表缺失即视为非法 .dsp。
func readMeta(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM " + TableMeta)
	if err != nil {
		return nil, fmt.Errorf("%w: 读取 %s 失败（表缺失或结构不符）: %v", ErrNotDsp, TableMeta, err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("%w: 解析 %s 失败: %v", ErrNotDsp, TableMeta, err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: 遍历 %s 失败: %v", ErrNotDsp, TableMeta, err)
	}
	return out, nil
}

// artifactHash 返回插件「产物」的内容哈希：覆盖元数据与全部 blob（代码/静态内容），
// **不含** dsp_state/dsp_log 与作者自有表——插件运行期写自己的状态不该被当成载体变更，
// 这正是状态能安全与代码同居一库的前提。宿主据此判定 .dsp 是否被重打包以决定热重载。
func (p *Plugin) artifactHash() (string, error) {
	h := sha256.New()
	err := p.withDB(func(db *sql.DB) error {
		meta, err := readMeta(db)
		if err != nil {
			return err
		}
		for _, k := range sortedKeysOfMap(meta) {
			fmt.Fprintf(h, "meta\x00%s\x00%s\x00", k, meta[k])
		}
		rows, err := db.Query("SELECT name, kind, language, content FROM " + TableBlobs + " ORDER BY name")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, kind, lang string
			var content []byte
			if err := rows.Scan(&name, &kind, &lang, &content); err != nil {
				return err
			}
			fmt.Fprintf(h, "blob\x00%s\x00%s\x00%s\x00", name, kind, lang)
			h.Write(content)
			h.Write([]byte{0})
		}
		return rows.Err()
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sortedKeysOfMap(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// 状态（dsp_state）：插件自持 KV，随文件一起落盘，重启不丢。
// ---------------------------------------------------------------------------

// StateGet 读取插件自持状态（值为 JSON 解码后的 Go 值）。
func (p *Plugin) StateGet(key string) (any, bool, error) {
	var raw []byte
	err := p.withDB(func(db *sql.DB) error {
		return db.QueryRow("SELECT value FROM "+TableState+" WHERE key = ?", key).Scan(&raw)
	})
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("读取状态 %q 失败: %w", key, err)
	}
	v, err := decodeValue(raw)
	if err != nil {
		return nil, false, fmt.Errorf("状态 %q 解码失败: %w", key, err)
	}
	return v, true, nil
}

// StateSet 写入插件自持状态。
func (p *Plugin) StateSet(key string, v any) error {
	if err := p.writableOrErr(); err != nil {
		return err
	}
	raw, err := encodeValue(v)
	if err != nil {
		return fmt.Errorf("状态 %q 编码失败: %w", key, err)
	}
	return p.withDB(func(db *sql.DB) error {
		_, err := db.Exec(
			"INSERT INTO "+TableState+"(key, value, updated_at) VALUES(?, ?, ?) "+
				"ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at",
			key, raw, now())
		if err != nil {
			return fmt.Errorf("写入状态 %q 失败: %w", key, err)
		}
		return nil
	})
}

// StateDelete 删除插件自持状态。
func (p *Plugin) StateDelete(key string) error {
	if err := p.writableOrErr(); err != nil {
		return err
	}
	return p.withDB(func(db *sql.DB) error {
		if _, err := db.Exec("DELETE FROM "+TableState+" WHERE key = ?", key); err != nil {
			return fmt.Errorf("删除状态 %q 失败: %w", key, err)
		}
		return nil
	})
}

// StateKeys 列出全部状态键（升序，保证确定性输出）。
func (p *Plugin) StateKeys() ([]string, error) {
	var out []string
	err := p.withDB(func(db *sql.DB) error {
		rows, err := db.Query("SELECT key FROM " + TableState + " ORDER BY key")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return err
			}
			out = append(out, k)
		}
		return rows.Err()
	})
	return out, err
}

// CountState 返回状态键数量（供 list_sql_plugins 概览）。
func (p *Plugin) CountState() int {
	var n int
	err := p.withDB(func(db *sql.DB) error {
		return db.QueryRow("SELECT COUNT(*) FROM " + TableState).Scan(&n)
	})
	if err != nil {
		return 0
	}
	return n
}

// ---------------------------------------------------------------------------
// 代码与静态内容（dsp_blobs）
// ---------------------------------------------------------------------------

// Blob 按名读取 blob 内容；kind 为空表示不限。
func (p *Plugin) Blob(name, kind string) ([]byte, bool, error) {
	q := "SELECT content FROM " + TableBlobs + " WHERE name = ?"
	args := []any{name}
	if kind != "" {
		q += " AND kind = ?"
		args = append(args, kind)
	}
	var raw []byte
	var found bool
	err := p.withDB(func(db *sql.DB) error {
		err := db.QueryRow(q, args...).Scan(&raw)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return raw, found, nil
}

// BlobInfo 描述一个 blob（不含内容）。
type BlobInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Language string `json:"language,omitempty"`
	Size     int    `json:"size"`
}

// Blobs 列出全部 blob（按 name 升序，保证确定性输出）。
func (p *Plugin) Blobs() ([]BlobInfo, error) {
	var out []BlobInfo
	err := p.withDB(func(db *sql.DB) error {
		rows, err := db.Query(
			"SELECT name, kind, language, COALESCE(LENGTH(content), 0) FROM " + TableBlobs + " ORDER BY name")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var b BlobInfo
			if err := rows.Scan(&b.Name, &b.Kind, &b.Language, &b.Size); err != nil {
				return err
			}
			out = append(out, b)
		}
		return rows.Err()
	})
	return out, err
}

// ---------------------------------------------------------------------------
// 脚本侧 SQL（经 GuardSQL 守卫，仅作用于自身库）
// ---------------------------------------------------------------------------

// Query 执行查询并物化全部行（列名 → 值；SQL NULL → nil）。
func (p *Plugin) Query(stmt string, params []any) ([]map[string]any, error) {
	if err := GuardSQL(stmt); err != nil {
		return nil, err
	}
	var out []map[string]any
	err := p.withDB(func(db *sql.DB) error {
		rows, err := db.Query(stmt, params...)
		if err != nil {
			return err
		}
		defer rows.Close()
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		out = make([]map[string]any, 0, 8)
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			row := make(map[string]any, len(cols))
			for i, c := range cols {
				row[c] = normalizeSQLValue(vals[i])
			}
			out = append(out, row)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Exec 执行写语句，返回受影响行数。
func (p *Plugin) Exec(stmt string, params []any) (int64, error) {
	if err := GuardSQL(stmt); err != nil {
		return 0, err
	}
	if err := p.writableOrErr(); err != nil {
		return 0, err
	}
	var n int64
	err := p.withDB(func(db *sql.DB) error {
		res, err := db.Exec(stmt, params...)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected() // 少数语句（DDL 已被守卫拦下）不报告行数
		return nil
	})
	return n, err
}

// Tables 列出插件库内的表与视图（排除 SQLite 内部表）。
func (p *Plugin) Tables() ([]string, error) {
	var out []string
	err := p.withDB(func(db *sql.DB) error {
		rows, err := db.Query(
			"SELECT name FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			out = append(out, n)
		}
		return rows.Err()
	})
	return out, err
}
