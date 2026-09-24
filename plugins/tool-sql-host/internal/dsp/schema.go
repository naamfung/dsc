package dsp

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// schemaDDL 是 .dsp 库的保留表结构（顺序即创建顺序）。
// dsp_meta/dsp_blobs 由打包工具写入、加载器读取；dsp_state/dsp_log 由加载器自愈补齐。
var schemaDDL = []string{
	`CREATE TABLE IF NOT EXISTS ` + TableMeta + ` (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS ` + TableBlobs + ` (
		id       INTEGER PRIMARY KEY,
		name     TEXT NOT NULL UNIQUE,
		kind     TEXT NOT NULL,
		language TEXT NOT NULL DEFAULT '',
		content  BLOB
	)`,
	`CREATE TABLE IF NOT EXISTS ` + TableState + ` (
		key        TEXT PRIMARY KEY,
		value      BLOB,
		updated_at TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS ` + TableLog + ` (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		level      TEXT NOT NULL,
		message    TEXT NOT NULL,
		created_at TEXT NOT NULL
	)`,
}

// SchemaDDL 返回保留表的建表语句（供打包器与文档复用）。
func SchemaDDL() []string {
	out := make([]string, len(schemaDDL))
	copy(out, schemaDDL)
	return out
}

// openDB 打开（必要时创建）给定路径的 SQLite 库，返回可直接操作的句柄。
// 仅供打包器建库使用；读取路径一律走 Plugin.withDB 的短连接。
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout=%d", busyTimeoutMS)); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// ensureRuntimeTables 建立缺失的保留表并落库 schema 版本（仅在可写库上调用）。
func ensureRuntimeTables(db *sql.DB) error {
	for _, ddl := range schemaDDL {
		if _, err := db.Exec(ddl); err != nil {
			return err
		}
	}
	var ver int
	if err := db.QueryRow("PRAGMA user_version").Scan(&ver); err != nil {
		return err
	}
	if ver == 0 {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", SchemaVersion)); err != nil {
			return err
		}
	}
	return nil
}

// writable 探测库是否真的可写，做法是「真写一次再回滚」：
//
//	BEGIN IMMEDIATE → CREATE TABLE __dsp_write_probe(x) → ROLLBACK
//
// 为什么不能只看 BEGIN IMMEDIATE：驱动在只读文件上会静默退化为只读连接，而
// BEGIN IMMEDIATE 只是取锁（Windows 上对只读句柄也能取），并不真正落盘；必须先
// 触发一次真实写（DDL 需写日志）才会暴露 "attempt to write a readonly database"。
// 回滚保证探测本身不留任何痕迹（实测文件逐字节不变）。
func writable(db *sql.DB) bool {
	if _, err := db.Exec("BEGIN IMMEDIATE"); err != nil {
		return false
	}
	if _, err := db.Exec("CREATE TABLE " + writeProbeTable + "(x)"); err != nil {
		_, _ = db.Exec("ROLLBACK")
		return false
	}
	_, err := db.Exec("ROLLBACK")
	return err == nil
}

// writeProbeTable 是可写性探测用的临时表名（始终在回滚事务里建了即弃，不会留存）。
const writeProbeTable = "__dsp_write_probe"

// writableOrErr 在只读插件上给出明确错误（而非静默丢弃写入）。
func (p *Plugin) writableOrErr() error {
	if p.ReadOnly {
		return fmt.Errorf("插件 %s 的 .dsp 为只读（状态写入不可用）：%s", p.Name, p.Path)
	}
	return nil
}

// now 返回统一的时间戳文本（UTC RFC3339，纳秒精度，跨平台一致）。
func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// encodeValue 把 Go 值编码为入库字节（JSON 文本）。
func encodeValue(v any) ([]byte, error) {
	return json.Marshal(v)
}

// decodeValue 把入库字节解码为 Go 值。
func decodeValue(raw []byte) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// normalizeSQLValue 把驱动返回的列值规范化为可安全投喂 Lua / JSON 的类型：
// []byte 退化为 string（避免 Lua 侧拿到字节切片），时间统一为 RFC3339 文本。
func normalizeSQLValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339Nano)
	default:
		return v
	}
}
