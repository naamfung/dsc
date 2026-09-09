package core

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/go-hclog"
	"gopkg.in/yaml.v3"
)

// Profile HMR（配置热重载，对齐 DSH profile HMR）。
//
// DSH 的 profile HMR 监听配置文件变化，变更时自动重载插件集——
// 用户修改 config.yaml 或 preset YAML 后无需重启 DSC，变更即时生效。
//
// DSC 的适配：ProfileWatcher 用 fsnotify 监听 config.yaml 与 preset YAML 文件，
// 变更时触发回调（ReloadCallback），由 Manager 重新加载插件集。
//
// 与插件热重载（hot_reload_watch.go）的区别：
//   - 插件热重载监听二进制文件变化（版本化 .exe 文件）
//   - Profile HMR 监听配置文件变化（.yaml 文件）
//   - 两者正交，可同时启用

// ReloadCallback 配置文件变更时触发的回调。
// 返回新的合并配置供 Manager 重新加载。
type ReloadCallback func() (*Config, error)

// ProfileWatcher 配置文件热重载监视器。
type ProfileWatcher struct {
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	callback ReloadCallback
	manager  *Manager
	logger   hclog.Logger
	// debounce 防抖：文件可能在短时间内被多次写入（编辑器保存触发多次事件）
	lastReload time.Time
	// minInterval 两次重载间最小间隔
	minInterval time.Duration
}

// NewProfileWatcher 创建配置文件热重载监视器。
// callback 在配置文件变更时被调用，返回新配置供 Manager 重新加载。
func NewProfileWatcher(mgr *Manager, callback ReloadCallback, logger hclog.Logger) (*ProfileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &ProfileWatcher{
		watcher:     watcher,
		stopCh:      make(chan struct{}),
		callback:    callback,
		manager:     mgr,
		logger:      logger,
		minInterval: 2 * time.Second, // 防抖：至少 2 秒间隔
	}, nil
}

// Watch 添加要监视的配置文件路径并启动监视循环。
func (pw *ProfileWatcher) Watch(paths ...string) error {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	for _, p := range paths {
		// 确保父目录被监视（fsnotify 监视目录，文件创建/删除事件需要目录级监视）
		dir := filepath.Dir(p)
		if err := pw.watcher.Add(dir); err != nil {
			pw.logger.Warn("profile watcher: failed to watch dir", "dir", dir, "error", err)
		}
		// 同时添加文件本身（如果已存在）
		if _, err := os.Stat(p); err == nil {
			if err := pw.watcher.Add(p); err != nil {
				pw.logger.Warn("profile watcher: failed to watch file", "file", p, "error", err)
			}
		}
	}
	go pw.watchLoop()
	return nil
}

// watchLoop 监视文件变更事件。
func (pw *ProfileWatcher) watchLoop() {
	for {
		select {
		case <-pw.stopCh:
			return
		case event, ok := <-pw.watcher.Events:
			if !ok {
				return
			}
			// 只关心写入/创建/重命名事件
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			// 防抖：检查距上次重载是否足够久
			pw.mu.Lock()
			if time.Since(pw.lastReload) < pw.minInterval {
				pw.mu.Unlock()
				continue
			}
			pw.lastReload = time.Now()
			pw.mu.Unlock()

			// 延迟 500ms 等待写入完成（编辑器可能先写临时文件再 rename）
			time.Sleep(500 * time.Millisecond)

			pw.logger.Info("profile watcher: config file changed, reloading", "file", event.Name)
			pw.reload()

		case err, ok := <-pw.watcher.Errors:
			if !ok {
				return
			}
			pw.logger.Warn("profile watcher: error", "error", err)
		}
	}
}

// reload 执行配置重载。
func (pw *ProfileWatcher) reload() {
	if pw.callback == nil {
		return
	}
	cfg, err := pw.callback()
	if err != nil {
		pw.logger.Error("profile watcher: reload callback failed", "error", err)
		return
	}
	if cfg == nil {
		return
	}
	// 经 Manager 重新加载插件集
	if err := pw.manager.ReloadFromConfig(cfg); err != nil {
		pw.logger.Error("profile watcher: reload from config failed", "error", err)
		return
	}
	pw.logger.Info("profile watcher: reload completed successfully")
}

// Stop 停止监视。
func (pw *ProfileWatcher) Stop() {
	close(pw.stopCh)
	pw.watcher.Close()
}

// ReloadFromConfig 重新加载插件集（由 ProfileWatcher 调用）。
// 先 Shutdown 当前插件，再用新配置 LoadFromConfig。
func (m *Manager) ReloadFromConfig(cfg *Config) error {
	// 先停止热重载监视器（避免 reload 期间触发再次 reload）
	m.StopHotReloadWatcher()

	// Shutdown 当前所有插件
	m.Shutdown()

	// 用新配置重新加载
	return m.LoadFromConfig(cfg)
}

// ParseConfigYAML 从 YAML 数据解析配置。
func ParseConfigYAML(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
