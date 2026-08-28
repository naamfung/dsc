package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"dsc-sdk"
	"dsc/core"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	rodproto "github.com/go-rod/rod/lib/proto"
)

// ============================================================
// 瀏覽器會話管理
// ============================================================

// BrowserSession 瀏覽器會話
type BrowserSession struct {
	ID         string
	Browser    *rod.Browser
	Pages      map[string]*rod.Page
	ActivePage string
	CreatedAt  time.Time
	LastUsed   time.Time
	UserMode   bool // 是否命中用户模式（复用默认 Chrome 登录态）
	mu         sync.Mutex
}

// BrowserSessionManager 瀏覽器會話管理器
type BrowserSessionManager struct {
	sessions map[string]*BrowserSession
	mu       sync.RWMutex
	closeMu  sync.Mutex // 用於避免重複關閉
}

var (
	globalBrowserSessionManager *BrowserSessionManager
	browserSessionOnce          sync.Once
)

// GetBrowserSessionManager 獲取全局瀏覽器會話管理器
func GetBrowserSessionManager() *BrowserSessionManager {
	browserSessionOnce.Do(func() {
		globalBrowserSessionManager = &BrowserSessionManager{
			sessions: make(map[string]*BrowserSession),
		}
		// 啟動空閒會話清理協程
		go globalBrowserSessionManager.cleanupIdleSessions()
	})
	return globalBrowserSessionManager
}

// getExeDir 獲取可執行文件所在目錄
func getExeDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

// cleanupOldBrowserData 清理24小時前的臨時瀏覽器數據目錄
func cleanupOldBrowserData(exeDir string) error {
	browserDataRoot := filepath.Join(exeDir, "temp", "browser-data")
	// 確保根目錄存在
	if err := os.MkdirAll(browserDataRoot, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(browserDataRoot)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(browserDataRoot, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// 檢查修改時間是否超過24小時
		if now.Sub(info.ModTime()) > 24*time.Hour {
			log.Printf("[BrowserSessionManager] removing old browser data directory: %s", fullPath)
			if err := os.RemoveAll(fullPath); err != nil {
				log.Printf("[BrowserSessionManager] error removing old browser data directory %s: %v", fullPath, err)
			}
		}
	}
	return nil
}

// defaultUserDataDir 返回默认 Chrome 用户数据目录（内含用户登录态）。
// Windows 下为 %LOCALAPPDATA%\Google\Chrome\User Data；不存在或未知平台返回空串。
func defaultUserDataDir() string {
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			p := filepath.Join(la, "Google", "Chrome", "User Data")
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				return p
			}
		}
	}
	return ""
}

// tryLaunch 用给定 user-data-dir 启动浏览器并连接，返回是否成功。
// 用 recover 捕获启动/连接异常（如 profile 被锁），供用户模式失败降级。
func tryLaunch(userDataDir string) (b *rod.Browser, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			b = nil
		}
	}()
	l := launcher.New().UserDataDir(userDataDir)
	b = rod.New().ControlURL(l.MustLaunch()).MustConnect()
	return b, true
}

// launchBrowserRod 启动浏览器实例。userMode 为真时优先复用默认 Chrome 用户数据
// 目录（携带登录态）；该目录不可用（如用户 Chrome 已在运行、profile 被锁）时
// 自动降级到按 session 隔离的临时目录。返回浏览器与是否命中了用户模式。
func launchBrowserRod(sessionID string, userMode bool) (*rod.Browser, bool, error) {
	// 用户模式：复用默认用户数据目录以保留登录态；失败则降级
	if userMode {
		if dir := defaultUserDataDir(); dir != "" {
			if b, ok := tryLaunch(dir); ok {
				log.Printf("[BrowserSessionManager] user-mode session %s using default Chrome profile %s", sessionID, dir)
				return b, true, nil
			}
			log.Printf("[BrowserSessionManager] user-mode profile unavailable for %s (likely locked), falling back to isolated temp profile", sessionID)
		}
	}

	// 获取可执行文件所在目录
	exeDir, err := getExeDir()
	if err != nil {
		return nil, false, fmt.Errorf("failed to get executable dir: %w", err)
	}

	// 浏览器数据目录：程序可执行路径下的 temp/browser-data/<sessionID>
	// 避免与 workspace 混雜，並通過 sessionID 區分不同實例
	browserDataDir := filepath.Join(exeDir, "temp", "browser-data", sessionID)
	if err := os.MkdirAll(browserDataDir, 0755); err != nil {
		return nil, false, fmt.Errorf("failed to create browser data directory: %w", err)
	}

	// 清理24小時前的臨時瀏覽器數據
	if err := cleanupOldBrowserData(exeDir); err != nil {
		log.Printf("[BrowserSessionManager] warning: failed to cleanup old browser data: %v", err)
	}

	b, ok := tryLaunch(browserDataDir)
	if !ok {
		return nil, false, fmt.Errorf("failed to launch browser for session %s", sessionID)
	}
	return b, false, nil
}

// CreateSession 創建新的瀏覽器會話
// 如果會話已存在，直接返回現有會話並更新 LastUsed
func (m *BrowserSessionManager) CreateSession(sessionID string, userMode bool) (*BrowserSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果會話已存在，直接返回（在 sess.mu 下更新 LastUsed）
	if sess, ok := m.sessions[sessionID]; ok {
		sess.mu.Lock()
		sess.LastUsed = time.Now()
		sess.mu.Unlock()
		return sess, nil
	}

	// 啟動瀏覽器
	browser, usedUserMode, err := launchBrowserRod(sessionID, userMode)
	if err != nil {
		return nil, fmt.Errorf("啟動瀏覽器失敗: %w", err)
	}

	now := time.Now()
	sess := &BrowserSession{
		ID:        sessionID,
		Browser:   browser,
		Pages:     make(map[string]*rod.Page),
		CreatedAt: now,
		LastUsed:  now,
		UserMode:  usedUserMode,
	}

	m.sessions[sessionID] = sess
	log.Printf("[BrowserSessionManager] Created session %s (userMode=%v)", sessionID, usedUserMode)
	return sess, nil
}

// GetSession 獲取會話（不創建）
func (m *BrowserSessionManager) GetSession(sessionID string) (*BrowserSession, bool) {
	m.mu.RLock()
	sess, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if ok {
		// 更新 LastUsed 必須在 sess.mu 下進行，避免與 cleanupIdleSessions 等讀取競爭
		sess.mu.Lock()
		sess.LastUsed = time.Now()
		sess.mu.Unlock()
	}
	return sess, ok
}

// CloseSession 關閉並移除指定的瀏覽器會話
// 這是防止資源泄漏的關鍵方法，調用方應在 GlobalSession 停止時調用此方法
func (m *BrowserSessionManager) CloseSession(sessionID string) error {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()

	m.mu.Lock()
	sess, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return nil
	}
	delete(m.sessions, sessionID)
	m.mu.Unlock()

	if sess.Browser != nil {
		log.Printf("[BrowserSessionManager] Closing browser session %s", sessionID)
		sess.Browser.Close()
	}
	return nil
}

// CloseAllSessions 關閉所有瀏覽器會話（用於程序退出時清理）
func (m *BrowserSessionManager) CloseAllSessions() {
	// 使用寫鎖拍快照並清空 map，防止 CloseAllSessions 和 CreateSession 之間的窗口泄漏
	m.mu.Lock()
	sessions := make([]*BrowserSession, 0, len(m.sessions))
	for _, sess := range m.sessions {
		sessions = append(sessions, sess)
	}
	m.sessions = make(map[string]*BrowserSession)
	m.mu.Unlock()

	for _, sess := range sessions {
		if sess.Browser != nil {
			sess.Browser.Close()
		}
	}
	log.Printf("[BrowserSessionManager] All browser sessions closed")
}

// cleanupIdleSessions 定期清理空閒超時的會話
// 空閒超時時間默認 30 分鐘
func (m *BrowserSessionManager) cleanupIdleSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupIdleSessionsOnce()
	}
}

func (m *BrowserSessionManager) cleanupIdleSessionsOnce() {
	idleThreshold := 30 * time.Minute
	now := time.Now()
	toClose := make([]string, 0)

	m.mu.RLock()
	for id, sess := range m.sessions {
		sess.mu.Lock()
		lastUsed := sess.LastUsed
		sess.mu.Unlock()
		if now.Sub(lastUsed) > idleThreshold {
			toClose = append(toClose, id)
		}
	}
	m.mu.RUnlock()

	for _, id := range toClose {
		if err := m.CloseSession(id); err != nil {
			log.Printf("[BrowserSessionManager] Failed to close idle session %s: %v", id, err)
		} else {
			log.Printf("[BrowserSessionManager] Closed idle session %s", id)
		}
	}
}

// CreatePage 在會話中創建新頁面
func (s *BrowserSession) CreatePage(pageID string, url string) (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 創建新頁面
	page, err := s.Browser.Page(rodproto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, fmt.Errorf("創建頁面失敗: %w", err)
	}

	s.Pages[pageID] = page
	s.ActivePage = pageID
	s.LastUsed = time.Now()

	return page, nil
}

// GetPage 獲取頁面
func (s *BrowserSession) GetPage(pageID string) (*rod.Page, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	page, ok := s.Pages[pageID]
	if ok {
		s.LastUsed = time.Now()
	}
	return page, ok
}

// GetActivePage 獲取當前活動頁面
func (s *BrowserSession) GetActivePage() (*rod.Page, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ActivePage == "" {
		return nil, false
	}
	page, ok := s.Pages[s.ActivePage]
	s.LastUsed = time.Now()
	return page, ok
}

// SetActivePage 設置活動頁面
func (s *BrowserSession) SetActivePage(pageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.Pages[pageID]; !ok {
		return fmt.Errorf("頁面 %s 不存在", pageID)
	}
	s.ActivePage = pageID
	s.LastUsed = time.Now()
	return nil
}

// ClosePage 關閉頁面
func (s *BrowserSession) ClosePage(pageID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	page, ok := s.Pages[pageID]
	if !ok {
		return nil
	}

	if page != nil {
		page.Close()
	}
	delete(s.Pages, pageID)

	// 如果關閉的是活動頁面，切換到其他頁面
	if s.ActivePage == pageID {
		s.ActivePage = ""
		for id := range s.Pages {
			s.ActivePage = id
			break
		}
	}
	s.LastUsed = time.Now()
	return nil
}

// PageInfo 頁面信息
type PageInfo struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

// ListPages 列出所有頁面
func (s *BrowserSession) ListPages() []PageInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	var pages []PageInfo
	for id, page := range s.Pages {
		info, _ := page.Info()
		pi := PageInfo{
			ID:     id,
			URL:    info.URL,
			Title:  info.Title,
			Active: id == s.ActivePage,
		}
		pages = append(pages, pi)
	}
	return pages
}

// ============================================================
// 瀏覽器工具實現
// ============================================================

// getOrCreatePage 獲取或創建瀏覽器頁面（使用會話管理器）
// userMode 仅在首次创建会话（启动浏览器）时生效；会话已存在则忽略。
func getOrCreatePage(sessionID, pageID, url string, userMode bool) (*rod.Page, *BrowserSession, error) {
	mgr := GetBrowserSessionManager()
	sess, ok := mgr.GetSession(sessionID)
	if !ok {
		var err error
		sess, err = mgr.CreateSession(sessionID, userMode)
		if err != nil {
			return nil, nil, err
		}
	}

	page, ok := sess.GetPage(pageID)
	if !ok || page == nil {
		var err error
		page, err = sess.CreatePage(pageID, url)
		if err != nil {
			return nil, nil, err
		}
	} else if url != "" {
		if err := page.Navigate(url); err != nil {
			return nil, nil, err
		}
	}

	// 設置超時：不在此 cancel，交由 page.Context 內部管理。
	timeout := 30 // 默認 30 秒超時
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	_ = cancel // 保留變量避免編譯錯誤，但不在此取消
	page = page.Context(ctx)

	return page, sess, nil
}

// FetchUrlResult 獲取網頁內容結果
type FetchUrlResult struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// fetchUrlImpl 抓取网页内容。plainText 为真时返回剥离结构的纯文本（默认，
// 便于模型直接阅读、避免对大量 HTML 产生抗拒），为假时返回原始 HTML 供需要
// 结构化内容的场景使用。结果用 json.Marshal 序列化，避免手拼字符串破坏转义。
func fetchUrlImpl(sessionID, url string, plainText, userMode bool) (string, error) {
	page, _, err := getOrCreatePage(sessionID, "default", url, userMode)
	if err != nil {
		e := FetchUrlResult{Success: false, URL: url, Error: err.Error()}
		b, _ := json.Marshal(e)
		return string(b), nil
	}

	// 等待页面加载
	if err := page.WaitLoad(); err != nil {
		log.Printf("頁面加載警告: %v", err)
	}

	content := ""
	if plainText {
		content = page.MustEval("() => document.body.innerText").Str()
	} else {
		content = page.MustEval("() => document.documentElement.outerHTML").Str()
	}

	e := FetchUrlResult{Success: true, URL: url, Content: content}
	b, _ := json.Marshal(e)
	return string(b), nil
}

// WebSearchResult 聚合搜索结果
type WebSearchResult struct {
	Success bool               `json:"success"`
	Query   string             `json:"query"`
	Results []SearchResultItem `json:"results"`
	Errors  []string           `json:"errors,omitempty"`
	Sources []string           `json:"sources,omitempty"`
}

type SearchResultItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"`
}

// searchEngine 单一搜索引擎的取值定义：怎样构造搜索 URL、等待哪个结果容器、
// 以及如何从 DOM 提取结果。
type searchEngine struct {
	name      string
	urlFmt    string
	waitSel   string
	extractJS string
}

// searchEngines 支持的多搜索引擎。各引擎的提取脚本独立（DOM 结构差异大），
// 单个引擎失败仅导致该引擎无结果，不影响整体聚合。
var searchEngines = map[string]searchEngine{
	"duckduckgo": {
		name:    "duckduckgo",
		urlFmt:  "https://html.duckduckgo.com/html/?q=%s",
		waitSel: ".result, .results",
		extractJS: `() => JSON.stringify(Array.from(document.querySelectorAll('.result')).map(r => {
			const a = r.querySelector('a.result__a');
			const s = r.querySelector('.result__snippet, .result__snippet_no_offset');
			return { title: a ? a.innerText.trim() : '', url: a ? a.href : '', description: s ? s.innerText.trim() : '' };
		}).filter(r => r.url && r.url.startsWith('http')))`,
	},
	"google": {
		name:    "google",
		urlFmt:  "https://www.google.com/search?q=%s",
		waitSel: "a[href]",
		extractJS: `() => JSON.stringify(Array.from(document.querySelectorAll('a[href^="http"]')).map(r => {
			const a = r.querySelector('h3') || r;
			return { title: a ? a.innerText.trim() : '', url: r.href || '', description: '' };
		}).filter(r => r.url && r.url.startsWith('http')))`,
	},
	"baidu": {
		name:    "baidu",
		urlFmt:  "https://www.baidu.com/s?wd=%s",
		waitSel: "#content_left",
		extractJS: `() => JSON.stringify(Array.from(document.querySelectorAll('#content_left .result, #content_left .c-container')).map(r => {
			const a = r.querySelector('h3 a');
			const s = r.querySelector('.c-abstract, .content-right');
			return { title: a ? a.innerText.trim() : '', url: a ? a.href : '', description: s ? s.innerText.trim() : '' };
		}).filter(r => r.url && r.url.startsWith('http')))`,
	},
	"so360": {
		name:    "so360",
		urlFmt:  "https://www.so.com/s?q=%s",
		waitSel: ".res-list",
		extractJS: `() => JSON.stringify(Array.from(document.querySelectorAll('.res-list')).map(r => {
			const a = r.querySelector('h3 a');
			const s = r.querySelector('.res-desc');
			return { title: a ? a.innerText.trim() : '', url: a ? a.href : '', description: s ? s.innerText.trim() : '' };
		}).filter(r => r.url && r.url.startsWith('http')))`,
	},
}

// parseEngines 解析 engines 参数（逗号分隔），去重并跳过非法项；空或全部非法时
// 返回默认引擎集。
func parseEngines(engines string) []string {
	if strings.TrimSpace(engines) == "" {
		return []string{"google", "duckduckgo", "baidu", "so360"}
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range strings.Split(engines, ",") {
		e = strings.TrimSpace(e)
		if _, ok := searchEngines[e]; ok && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return []string{"google", "duckduckgo", "baidu", "so360"}
	}
	return out
}

// extractSearchPageElements 从已就绪的搜索结果页提取结果项。
func extractSearchPageElements(page *rod.Page, spec searchEngine) ([]SearchResultItem, error) {
	linksJSON := page.MustEval(spec.extractJS).Str()
	return unmarshalSearchItems(linksJSON)
}

// unmarshalSearchItems 解析引擎提取脚本产出的结果 JSON 数组。
func unmarshalSearchItems(raw string) ([]SearchResultItem, error) {
	var items []SearchResultItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// mergeEngineResults 把单个引擎的结果按 URL 去重并入调用方的 seen 集合，
// 并给每条结果标注来源引擎。返回新增（未重复）的结果与是否带来了新结果。
func mergeEngineResults(items []SearchResultItem, engine string, seen map[string]bool) (added []SearchResultItem, hasNew bool) {
	for i := range items {
		it := items[i]
		it.Source = engine
		if seen[it.URL] {
			continue
		}
		seen[it.URL] = true
		added = append(added, it)
		hasNew = true
	}
	return added, hasNew
}

// webSearchImpl 按指定引擎集搜索并聚合。逐个引擎访问其搜索结果页、提取结果，
// 按 URL 去重后统一返回；单引擎失败（DOM 变化 / 验证码 / 网络）仅记录错误，
// 不阻断其它引擎，满足"多源同时搜索再聚合"的诉求。
func webSearchImpl(sessionID, query string, engines []string, userMode bool) (string, error) {
	aggregated := make([]SearchResultItem, 0, 16)
	seen := make(map[string]bool)
	sources := make([]string, 0)
	var errors []string

	for _, name := range engines {
		spec, ok := searchEngines[name]
		if !ok {
			errors = append(errors, fmt.Sprintf("engine %q not supported", name))
			continue
		}
		searchURL := fmt.Sprintf(spec.urlFmt, url.QueryEscape(query))
		page, _, err := getOrCreatePage(sessionID, "default", searchURL, userMode)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if err := page.WaitLoad(); err != nil {
			log.Printf("頁面加載警告(%s): %v", name, err)
		}
		// 等待结果容器出现（最多 10 秒）
		waitCtx := page.Timeout(10 * time.Second)
		if _, err := waitCtx.Element(spec.waitSel); err != nil {
			errors = append(errors, fmt.Sprintf("%s: results not rendered", name))
			continue
		}
		items, err := extractSearchPageElements(page, spec)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		added, hasNew := mergeEngineResults(items, name, seen)
		aggregated = append(aggregated, added...)
		if hasNew {
			sources = append(sources, name)
		}
	}

	res := WebSearchResult{
		Success: len(aggregated) > 0,
		Query:   query,
		Results: aggregated,
		Errors:  errors,
		Sources: sources,
	}
	b, _ := json.Marshal(res)
	return string(b), nil
}

// BrowserClickResult 點擊結果
type BrowserClickResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
}

func browserClickImpl(sessionID, url, selector string) (string, error) {
	page, _, err := getOrCreatePage(sessionID, "default", url, true)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error()), nil
	}

	element, err := page.Element(selector)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"未找到元素 '%s': %s"}`, selector, err.Error()), nil
	}

	if err := element.ScrollIntoView(); err != nil {
		log.Printf("滾動到元素失敗: %v", err)
	}

	element.MustClick()
	time.Sleep(500 * time.Millisecond)

	info, _ := page.Info()
	result := BrowserClickResult{
		Success: true,
		Message: fmt.Sprintf("成功點擊元素: %s", selector),
		URL:     info.URL,
	}

	jsonData, _ := json.Marshal(result)
	return string(jsonData), nil
}

// BrowserTypeResult 輸入結果
type BrowserTypeResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

func browserTypeImpl(sessionID, url, selector, text string, submit bool) (string, error) {
	page, _, err := getOrCreatePage(sessionID, "default", url, true)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error()), nil
	}

	element, err := page.Element(selector)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"未找到輸入框 '%s': %s"}`, selector, err.Error()), nil
	}

	element.MustClick()
	element.SelectAllText()
	element.Input(text)

	if submit {
		page.Keyboard.Press(input.Enter)
		time.Sleep(500 * time.Millisecond)
	}

	result := BrowserTypeResult{
		Success: true,
		Message: fmt.Sprintf("成功輸入文本到: %s", selector),
		Value:   text,
	}

	jsonData, _ := json.Marshal(result)
	return string(jsonData), nil
}

// BrowserScreenshotResult 截圖結果
type BrowserScreenshotResult struct {
	URL       string `json:"url"`
	Success   bool   `json:"success"`
	SavedFile string `json:"savedFile,omitempty"`
	FullPage  bool   `json:"fullPage"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Size      int64  `json:"size"`
}

func browserScreenshotImpl(sessionID, url string, fullPage bool) (string, error) {
	page, _, err := getOrCreatePage(sessionID, "default", url, true)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"%s"}`, err.Error()), nil
	}

	time.Sleep(1 * time.Second)
	width := page.MustEval("() => window.innerWidth").Int()
	height := page.MustEval("() => document.body.scrollHeight").Int()

	var screenshot []byte
	if fullPage {
		screenshot = page.MustScreenshotFullPage()
	} else {
		screenshot = page.MustScreenshot()
	}

	// 保存截圖到工作區目錄（沙箱限制寫入系統臨時目錄，如 %TEMP%，故存到 workspace 內）
	downloadDir := filepath.Join(core.WorkspaceRoot, "screenshots")
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Sprintf(`{"success":false,"error":"創建下載目錄失敗: %s"}`, err.Error()), nil
	}

	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("screenshot_%s.png", timestamp)
	filePath := filepath.Join(downloadDir, fileName)

	if err := os.WriteFile(filePath, screenshot, 0644); err != nil {
		return fmt.Sprintf(`{"success":false,"error":"保存截圖失敗: %s"}`, err.Error()), nil
	}

	result := BrowserScreenshotResult{
		URL:       url,
		Success:   true,
		SavedFile: filePath,
		FullPage:  fullPage,
		Width:     width,
		Height:    height,
		Size:      int64(len(screenshot)),
	}

	jsonData, _ := json.Marshal(result)
	return string(jsonData), nil
}

// ============================================================
// 以公共 SDK（dsc-sdk）声明式启动：SDK 自动提供 ToolService /
// PluginMetadata / PluginHookService 与 go-core 组装（重写自旧的
// ToolServiceServer/MetadataServer/ToolMetadataGRPCPlugin 样板）。
// ============================================================
func main() {
	// 定義 fetch_url 工具
	fetchUrlSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "Browser session ID for persistent state"
			},
			"url": {
				"type": "string",
				"description": "URL to fetch"
			},
			"return_html": {
				"type": "boolean",
				"description": "Whether to return raw HTML instead of plain text. Defaults to false (plain text)."
			},
			"user_mode": {
				"type": "boolean",
				"description": "Whether to reuse the default Chrome profile (logged-in state) when creating the session. Defaults to true."
			}
		},
		"required": ["url"]
	}`)
	fetchUrlHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			SessionID  string `json:"session_id"`
			URL        string `json:"url"`
			ReturnHTML bool   `json:"return_html"`
			UserMode   *bool  `json:"user_mode"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if params.URL == "" {
			return "", fmt.Errorf("url is required")
		}
		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = "default-browser-session"
		}
		userMode := true // 默认用户模式
		if params.UserMode != nil {
			userMode = *params.UserMode
		}
		return fetchUrlImpl(sessionID, params.URL, !params.ReturnHTML, userMode)
	}

	// 定義 web_search 工具
	webSearchSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "Browser session ID for persistent state"
			},
			"query": {
				"type": "string",
				"description": "Search query"
			},
			"engines": {
				"type": "string",
				"description": "Comma-separated search engines to aggregate, e.g. \"google,baidu,so360\". Supported: google, duckduckgo, baidu, so360. Defaults to all."
			},
			"user_mode": {
				"type": "boolean",
				"description": "Whether to reuse the default Chrome profile (logged-in state) when creating the session. Defaults to true."
			}
		},
		"required": ["query"]
	}`)
	webSearchHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			SessionID string `json:"session_id"`
			Query     string `json:"query"`
			Engines   string `json:"engines"`
			UserMode  *bool  `json:"user_mode"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if strings.TrimSpace(params.Query) == "" {
			return "", fmt.Errorf("query is required")
		}
		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = "default-browser-session"
		}
		userMode := true // 默认用户模式
		if params.UserMode != nil {
			userMode = *params.UserMode
		}
		return webSearchImpl(sessionID, params.Query, parseEngines(params.Engines), userMode)
	}

	// 定義 browser_click 工具
	browserClickSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "Browser session ID for persistent state"
			},
			"url": {
				"type": "string",
				"description": "Current page URL"
			},
			"selector": {
				"type": "string",
				"description": "CSS selector of the element to click"
			}
		},
		"required": ["url", "selector"]
	}`)
	browserClickHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			SessionID string `json:"session_id"`
			URL       string `json:"url"`
			Selector  string `json:"selector"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if params.URL == "" || params.Selector == "" {
			return "", fmt.Errorf("url and selector are required")
		}
		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = "default-browser-session"
		}
		return browserClickImpl(sessionID, params.URL, params.Selector)
	}

	// 定義 browser_type 工具
	browserTypeSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "Browser session ID for persistent state"
			},
			"url": {
				"type": "string",
				"description": "Current page URL"
			},
			"selector": {
				"type": "string",
				"description": "CSS selector of the input field"
			},
			"text": {
				"type": "string",
				"description": "Text to type"
			},
			"submit": {
				"type": "boolean",
				"description": "Whether to submit the form after typing"
			}
		},
		"required": ["url", "selector", "text"]
	}`)
	browserTypeHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			SessionID string `json:"session_id"`
			URL       string `json:"url"`
			Selector  string `json:"selector"`
			Text      string `json:"text"`
			Submit    bool   `json:"submit"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if params.URL == "" || params.Selector == "" || params.Text == "" {
			return "", fmt.Errorf("url, selector, and text are required")
		}
		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = "default-browser-session"
		}
		return browserTypeImpl(sessionID, params.URL, params.Selector, params.Text, params.Submit)
	}

	// 定義 browser_screenshot 工具
	browserScreenshotSchema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"session_id": {
				"type": "string",
				"description": "Browser session ID for persistent state"
			},
			"url": {
				"type": "string",
				"description": "Current page URL"
			},
			"full_page": {
				"type": "boolean",
				"description": "Whether to take a full page screenshot"
			}
		},
		"required": ["url"]
	}`)
	browserScreenshotHandler := func(ctx context.Context, args json.RawMessage) (string, error) {
		var params struct {
			SessionID string `json:"session_id"`
			URL       string `json:"url"`
			FullPage  bool   `json:"full_page"`
		}
		if err := json.Unmarshal(args, &params); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
		if params.URL == "" {
			return "", fmt.Errorf("url is required")
		}
		sessionID := params.SessionID
		if sessionID == "" {
			sessionID = "default-browser-session"
		}
		return browserScreenshotImpl(sessionID, params.URL, params.FullPage)
	}

	sdk := dsc.New(dsc.Config{Name: "browser-use", Version: "1.0.0", Type: dsc.TypeTool})
	sdk.Tool(dsc.Tool{Name: "fetch_url", Description: "Fetch the content of a URL using a headless browser, supporting JavaScript rendering", Schema: fetchUrlSchema, Handler: fetchUrlHandler})
	sdk.Tool(dsc.Tool{Name: "web_search", Description: "Perform a web search using a headless browser and extract search results", Schema: webSearchSchema, Handler: webSearchHandler})
	sdk.Tool(dsc.Tool{Name: "browser_click", Description: "Click an element on the page by selector", Schema: browserClickSchema, Handler: browserClickHandler})
	sdk.Tool(dsc.Tool{Name: "browser_type", Description: "Type text into an input field", Schema: browserTypeSchema, Handler: browserTypeHandler})
	sdk.Tool(dsc.Tool{Name: "browser_screenshot", Description: "Take a screenshot of the current page or full page", Schema: browserScreenshotSchema, Handler: browserScreenshotHandler})
	sdk.Serve()
}
