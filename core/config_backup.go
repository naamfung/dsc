package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-hclog"
)

// 配置自愈：config.yaml 与 preset <mode>.yaml 都是会被改写的配置文件。
// 每次成功启动后，把已生效的 config.yaml 与当前 mode 的 preset 各自独立备份到
// config-backups/（旋转保留最近 N 份，互为独立、互不串扰）。当某份文件损坏、
// 丢失或因坏插件导致启动失败时，宿主「分别还原」该文件自己的最近正常备份，
// 再继续启动——不做跨文件合并式恢复，避免 A 坏了却用 B 的备份去套。

// backupKeep 旋转保留的最近正常配置份数。
const backupKeep = 10

// backupConfigDir / backupPresetDir：config.yaml 与 preset 各自独立的备份子目录名。
const (
	backupConfigDir = "config-backups"
	backupPresetDir = "preset-backups"
)

// backupDir 返回某配置文件所在目录下的备份目录：config.yaml 用 config-backups/，
// preset（standard/minimal/... 等模式文件）用 preset-backups/，两者独立、互不串扰。
func backupDir(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	sub := backupConfigDir
	if base != "config" {
		sub = backupPresetDir
	}
	return filepath.Join(filepath.Dir(file), sub)
}

// backupName 由源文件名.prefix 派生备份文件名：<base>.<ts>.yaml。
// 不同来源文件用各自 base（如 config、standard）作前缀，互不覆盖、各自旋转。
func backupName(file string) string {
	base := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	ts := time.Now().Format("20060102-150405.000")
	return fmt.Sprintf("%s.%s.yaml", base, ts)
}

// BackupGoodFile 把一份已生效的配置文件（config.yaml 或 preset）备份进备份目录，
// 并修剪到最近 N 份（按各自文件名前缀区分、独立旋转）。返回备份路径。
func BackupGoodFile(file string, logger hclog.Logger) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("backup good file: read %s: %w", file, err)
	}
	dir := backupDir(file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("backup good file: mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, backupName(file))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("backup good file: write %s: %w", path, err)
	}
	trimBackups(dir, backupPrefix(file))
	return path, nil
}

// backupPrefix 返回本文件备份文件名的公共前缀（<base>.），用于区分不同来源的备份。
func backupPrefix(file string) string {
	return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file)) + "."
}

// trimBackups 修剪备份目录，仅保留某前缀下最近 N 份（文件名按时间排倒序）。
func trimBackups(dir, prefix string) {
	entries, _ := os.ReadDir(dir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := backupKeep; i < len(names); i++ {
		_ = os.Remove(filepath.Join(dir, names[i]))
	}
}

// LatestGoodBackup 返回某文件在备份目录中最近一份正常备份路径；无则返回空串。
func LatestGoodBackup(file string) (string, error) {
	entries, err := os.ReadDir(backupDir(file))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	prefix := backupPrefix(file)
	best := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if e.Name() > best {
			best = e.Name()
		}
	}
	if best == "" {
		return "", nil
	}
	return filepath.Join(backupDir(file), best), nil
}

// RestoreGoodFile 用一份备份覆盖源文件（保证启动可回读正常版本）。
func RestoreGoodFile(file, backup string, logger hclog.Logger) error {
	data, err := os.ReadFile(backup)
	if err != nil {
		return fmt.Errorf("restore file: read backup %s: %w", backup, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(file, data, 0644); err != nil {
		return fmt.Errorf("restore file: write %s: %w", file, err)
	}
	if logger != nil {
		logger.Info("restored file from good backup", "file", file, "backup", backup)
	}
	return nil
}

// BackupGoodConfig 为方便 Manager 内部使用提供的封装：备份 persistConfigPath()。
func (m *Manager) BackupGoodConfig() (string, error) {
	return BackupGoodFile(m.persistConfigPath(), m.logger)
}
