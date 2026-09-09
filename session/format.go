package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// 会话格式版本化与迁移（对齐 DSH session-format + session-format-catalog + 迁移 codec）。
//
// DSH 有 4 个包：session-format（版本定义）、session-format-catalog（注册表）、
// session-format-v0-to-v1、session-format-v1-to-v2（迁移 codec）。
// 每个版本有 validate / migration，格式版本号写在文件头部。
//
// DSC 的适配：当前只有一种格式（无版本头）。本模块引入版本化：
//   - SessionFormatVersion：当前格式版本号
//   - 迁移注册表：按版本号链式迁移
//   - 文件格式：首行 {"format_version": N} 后跟 JSONL 事件
//   - 向后兼容：无版本头的旧文件被视为 v0，自动迁移

// SessionFormatVersion 当前会话格式版本。
const SessionFormatVersion = 2

// FormatHeader 文件头（JSON 编码，独占文件首行）。
type FormatHeader struct {
	FormatVersion int `json:"format_version"`
}

// SessionMigration 从旧版本迁移到新版本的函数。
type SessionMigration func(events []*Event) ([]*Event, error)

// migrationRegistry 迁移注册表：migrations[from] = 把 v(from) 迁移到 v(from+1) 的函数。
var migrationRegistry = map[int]SessionMigration{
	0: migrateV0ToV1,
	1: migrateV1ToV2,
}

// migrateV0ToV1 从 v0（无版本头）迁移到 v1（加版本头）。
// v0 的事件格式与 v1 兼容——只是没有文件头。
func migrateV0ToV1(events []*Event) ([]*Event, error) {
	// v0 → v1 无需变更事件内容，只是加了文件头
	return events, nil
}

// migrateV1ToV2 从 v1 迁移到 v2。
// v2 新增了 compaction 事件类型（CompactionSummary 等）——旧文件中没有这类事件，
// 无需特殊处理；同时 v2 确保所有事件都有 Seq 字段（v1 可能有缺失）。
func migrateV1ToV2(events []*Event) ([]*Event, error) {
	for i, ev := range events {
		if ev.Seq == 0 {
			ev.Seq = i + 1
		}
	}
	return events, nil
}

// MigrateToCurrent 把事件列表从指定版本迁移到当前版本。
// 若 fromVersion >= SessionFormatVersion，直接返回（无需迁移）。
func MigrateToCurrent(fromVersion int, events []*Event) ([]*Event, error) {
	if fromVersion >= SessionFormatVersion {
		return events, nil
	}
	for v := fromVersion; v < SessionFormatVersion; v++ {
		migration, ok := migrationRegistry[v]
		if !ok {
			return nil, fmt.Errorf("session format: no migration from v%d to v%d", v, v+1)
		}
		var err error
		events, err = migration(events)
		if err != nil {
			return nil, fmt.Errorf("session format: migration v%d→v%d failed: %w", v, v+1, err)
		}
	}
	return events, nil
}

// DetectFormatVersion 从文件路径检测会话格式版本。
//   - 有首行 JSON 含 format_version → 返回该版本号
//   - 无首行 JSON 或解析失败 → 返回 0（旧格式）
func DetectFormatVersion(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	// 尝试解析首行为 FormatHeader
	firstLineEnd := 0
	for i, b := range data {
		if b == '\n' {
			firstLineEnd = i
			break
		}
	}
	if firstLineEnd == 0 {
		return 0, nil // 空文件或单行
	}
	firstLine := data[:firstLineEnd]
	// 尝试 JSON 解析
	var header FormatHeader
	if err := json.Unmarshal(firstLine, &header); err != nil {
		return 0, nil // 首行不是 JSON → 旧格式 v0
	}
	if header.FormatVersion > 0 {
		return header.FormatVersion, nil
	}
	return 0, nil
}

// SaveWithFormat 把事件列表以版本化格式写入文件。
// 文件格式：首行 FormatHeader JSON，后跟每行一个事件的 JSONL。
func SaveWithFormat(path string, events []*Event) error {
	header := FormatHeader{FormatVersion: SessionFormatVersion}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("session format: marshal header: %w", err)
	}
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// 写版本头
	if _, err := f.Write(append(headerJSON, '\n')); err != nil {
		return err
	}
	// 写事件（JSONL）
	enc := json.NewEncoder(f)
	for _, ev := range events {
		if err := enc.Encode(ev); err != nil {
			return fmt.Errorf("session format: encode event: %w", err)
		}
	}
	return nil
}

// LoadWithFormat 从文件加载事件列表，自动检测版本并迁移到当前版本。
func LoadWithFormat(path string) ([]*Event, error) {
	version, err := DetectFormatVersion(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// 跳过首行（版本头）
	startOffset := 0
	for i, b := range data {
		if b == '\n' {
			startOffset = i + 1
			break
		}
	}
	if version == 0 {
		// v0 无版本头，不跳过首行
		startOffset = 0
	}
	// 解析 JSONL 事件
	var events []*Event
	lines := splitJSONL(data[startOffset:])
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // 跳过损坏行
		}
		events = append(events, &ev)
	}
	// 迁移到当前版本
	return MigrateToCurrent(version, events)
}

// splitJSONL 把 JSONL 字节按行拆分为每行的字节切片。
func splitJSONL(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
