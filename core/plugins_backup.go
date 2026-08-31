package core

import (
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
)

// 插件目录自愈（P3）：二进制插件体积较大，不宜像配置那样大量存档，故只维护
// 「上次正常」的一份快照（默认命名为 plugins-backup，位于 plugins 同级）。
// 成功启动时若发现插件目录有新内容则刷新快照；启动因「插件目录与配置无法对齐」
// （如插件二进制缺失/损坏）而失败时，从快照回拷合并多正常插件目录再续启——
// 回拷为非破坏性合并（补缺失/覆盖损坏），不会删除其他正常插件。

// pluginsBackupDir 返回 plugins 的同级快照目录（<pluginsDir>-backup）。
func pluginsBackupDir(pluginsDir string) string {
	return pluginsDir + "-backup"
}

// maxModTime 返回目录树内最新修改时间（用于判断是否需要刷新快照）。
func maxModTime(dir string) (time.Time, error) {
	var latest time.Time
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无法访问的条目
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
		return nil
	})
	if err != nil {
		return latest, err
	}
	return latest, nil
}

// BackupPluginsDir 维护 plugins 的「上次正常」快照：仅当 plugins 有新于快照的内容
// （或快照缺失）时，才整目录合并拷贝刷新。返回是否发生了刷新。
func BackupPluginsDir(pluginsDir, backupDir string, _ hclog.Logger) (bool, error) {
	if st, err := os.Stat(pluginsDir); err != nil || !st.IsDir() {
		return false, nil // plugins 目录不可用则不快照
	}
	srcMax, err := maxModTime(pluginsDir)
	if err != nil {
		return false, err
	}
	var dstMax time.Time
	if st, err := os.Stat(backupDir); err == nil && st.IsDir() {
		if m, err2 := maxModTime(backupDir); err2 == nil {
			dstMax = m
		}
	} else {
		dstMax = time.Time{}
	}
	if !srcMax.After(dstMax) {
		return false, nil // 无新内容，跳过刷新（避免二进制大文件每次启动都重拷）
	}
	if err := copyDir(pluginsDir, backupDir); err != nil {
		return false, err
	}
	return true, nil
}

// RestorePluginsDir 从快照回拷到 plugins：非破坏性合并（补缺失、覆盖损坏）。
// 使用容错拷贝：运行中的插件二进制（Windows 下 .exe 被进程锁住）无法覆盖时
// 跳过该文件继续——那些本就是正常运行的，真正缺失/损坏（未运行）的会被回拷。
func RestorePluginsDir(backupDir, pluginsDir string, logger hclog.Logger) error {
	if st, err := os.Stat(backupDir); err != nil || !st.IsDir() {
		return err // 无快照则无法恢复
	}
	if err := copyDirTolerant(backupDir, pluginsDir); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("restored plugins dir from snapshot", "backup", backupDir, "target", pluginsDir)
	}
	return nil
}

// copyDirTolerant 递归拷贝目录树，但单个文件因被占用/共享冲突而无法写时跳过该文件
// 而非整体失败（供插件目录恢复用：运行中进程会锁住自身 .exe，但无需覆盖）。
func copyDirTolerant(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 无法访问的条目跳过
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if err := copyFile(path, target); err != nil {
			return nil // 被占用等写失败时跳过该文件，不中断恢复
		}
		return nil
	})
}
