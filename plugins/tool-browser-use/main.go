package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

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

// launchBrowserRod 啟動瀏覽器實例
func launchBrowserRod() (*rod.Browser, error) {
	// 使用項目目錄內的用戶數據目錄（而非 rod 默認的 %APPDATA%\rod），
	// 避免受沙箱「受限目錄不可寫」限制，同時保證多次啟動共用同一配置
	userDataDir := filepath.Join(plugin.WorkspaceRoot, "browser-data")
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create browser data directory: %w", err)
	}

	l := launcher.New().UserDataDir(userDataDir)
	browser := rod.New().
		ControlURL(l.MustLaunch()).
		MustConnect()

	return browser, nil
}

// CreateSession 創建新的瀏覽器會話
// 如果會話已存在，直接返回現有會話並更新 LastUsed
func (m *BrowserSessionManager) CreateSession(sessionID string) (*BrowserSession, error) {
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
	browser, err := launchBrowserRod()
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
	}

	m.sessions[sessionID] = sess
	log.Printf("[BrowserSessionManager] Created session %s", sessionID)
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
func getOrCreatePage(sessionID, pageID, url string) (*rod.Page, *BrowserSession, error) {
	mgr := GetBrowserSessionManager()
	sess, ok := mgr.GetSession(sessionID)
	if !ok {
		var err error
		sess, err = mgr.CreateSession(sessionID)
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

// BrowserTool 瀏覽器工具實現
type BrowserTool struct {
	name        string
	description string
	schema      json.RawMessage
	handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

func (b *BrowserTool) Name() string {
	return b.name
}

func (b *BrowserTool) Description() string {
	return b.description
}

func (b *BrowserTool) ParametersSchema() json.RawMessage {
	return b.schema
}

func (b *BrowserTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return b.handler(ctx, args)
}

// FetchUrlResult 獲取網頁內容結果
type FetchUrlResult struct {
	Success bool   `json:"success"`
	URL     string `json:"url"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

func fetchUrlImpl(sessionID, url string) (string, error) {
	page, _, err := getOrCreatePage(sessionID, "default", url)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"url":"%s","error":"%s"}`, url, err.Error()), nil
	}

	// 等待頁面加載
	if err := page.WaitLoad(); err != nil {
		log.Printf("頁面加載警告: %v", err)
	}

	// 獲取頁面內容
	contentHTML := page.MustEval("() => document.documentElement.outerHTML").Str()

	return fmt.Sprintf(`{"success":true,"url":"%s","content":"%s"}`, url, contentHTML), nil
}

// WebSearchResult 搜索結果
type WebSearchResult struct {
	Success bool               `json:"success"`
	Query   string             `json:"query"`
	Results []SearchResultItem `json:"results"`
}

type SearchResultItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

func webSearchImpl(sessionID, query string) (string, error) {
	// 先嘗試 DuckDuckGo（HTML 版對自動化友好、無需 JS 渲染；Google 對 headless 會返回 reCAPTCHA）
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	page, _, err := getOrCreatePage(sessionID, "default", searchURL)
	if err != nil {
		return fmt.Sprintf(`{"success":false,"query":"%s","error":"%s"}`, query, err.Error()), nil
	}

	// 等待頁面加載
	if err := page.WaitLoad(); err != nil {
		log.Printf("頁面加載警告: %v", err)
	}

	// 等待搜索結果渲染（DDG 的結果容器為 .result；最多等 10 秒）
	waitCtx := page.Timeout(10 * time.Second)
	_, err = waitCtx.Element(`.result, .results, form[action*="search"]`)
	if err != nil {
		// 結果容器未出現：返回當前標題與 URL，便於調試
		info, _ := page.Info()
		return fmt.Sprintf(`{"success":false,"query":"%s","error":"results not rendered","pageTitle":"%s","pageUrl":"%s"}`, query, info.Title, info.URL), nil
	}
	// 等待至少一個結果元素
	_ = waitCtx.WaitElementsMoreThan(`.result`, 0)

	// 提取搜索結果：DDG HTML 版每個結果塊為 .result，含 .result__a 標題鏈接與 .result__snippet 摘要
	linksJSON := page.MustEval(`() => {
		return JSON.stringify(Array.from(document.querySelectorAll('.result')).map(r => {
			const a = r.querySelector('a.result__a');
			const snippet = r.querySelector('.result__snippet, .result__snippet_no_offset');
			return {
				title: a ? a.innerText.trim() : '',
				url: a ? a.href : '',
				description: snippet ? snippet.innerText.trim() : ''
			};
		}).filter(r => r.url && r.url.startsWith('http')));
	}`).Str()

	return fmt.Sprintf(`{"success":true,"query":"%s","results_json":"%s"}`, query, linksJSON), nil
}

// BrowserClickResult 點擊結果
type BrowserClickResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
}

func browserClickImpl(sessionID, url, selector string) (string, error) {
	page, _, err := getOrCreatePage(sessionID, "default", url)
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
	page, _, err := getOrCreatePage(sessionID, "default", url)
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
	page, _, err := getOrCreatePage(sessionID, "default", url)
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
	downloadDir := filepath.Join(plugin.WorkspaceRoot, "screenshots")
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
// 插件服務實現
// ============================================================

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	tools []*BrowserTool
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	for _, t := range s.tools {
		if t.Name() == req.ToolName {
			res, err := t.Execute(ctx, json.RawMessage(req.ArgumentsJson))
			if err != nil {
				return &proto.ExecuteToolResponse{Error: err.Error()}, nil
			}
			return &proto.ExecuteToolResponse{Content: res}, nil
		}
	}
	return &proto.ExecuteToolResponse{Error: "tool not found"}, nil
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	var tools []*proto.Tool
	for _, t := range s.tools {
		tools = append(tools, &proto.Tool{
			Name:           t.Name(),
			Description:    t.Description(),
			ParametersJson: string(t.ParametersSchema()),
		})
	}
	return &proto.ListToolsResponse{Tools: tools}, nil
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "tool",
		Name:       "browser-use",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

func main() {
	// 定義 fetch_url 工具
	fetchUrlTool := &BrowserTool{
		name:        "fetch_url",
		description: "Fetch the content of a URL using a headless browser, supporting JavaScript rendering",
		schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "Browser session ID for persistent state"
				},
				"url": {
					"type": "string",
					"description": "URL to fetch"
				}
			},
			"required": ["url"]
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				SessionID string `json:"session_id"`
				URL       string `json:"url"`
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
			return fetchUrlImpl(sessionID, params.URL)
		},
	}

	// 定義 web_search 工具
	webSearchTool := &BrowserTool{
		name:        "web_search",
		description: "Perform a web search using a headless browser and extract search results",
		schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"session_id": {
					"type": "string",
					"description": "Browser session ID for persistent state"
				},
				"query": {
					"type": "string",
					"description": "Search query"
				}
			},
			"required": ["query"]
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				SessionID string `json:"session_id"`
				Query     string `json:"query"`
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
			return webSearchImpl(sessionID, params.Query)
		},
	}

	// 定義 browser_click 工具
	browserClickTool := &BrowserTool{
		name:        "browser_click",
		description: "Click an element on the page by selector",
		schema: json.RawMessage(`{
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
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
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
		},
	}

	// 定義 browser_type 工具
	browserTypeTool := &BrowserTool{
		name:        "browser_type",
		description: "Type text into an input field",
		schema: json.RawMessage(`{
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
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
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
		},
	}

	// 定義 browser_screenshot 工具
	browserScreenshotTool := &BrowserTool{
		name:        "browser_screenshot",
		description: "Take a screenshot of the current page or full page",
		schema: json.RawMessage(`{
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
		}`),
		handler: func(ctx context.Context, args json.RawMessage) (string, error) {
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
		},
	}

	// 創建工具服務服務端
	toolServer := &ToolServiceServer{
		tools: []*BrowserTool{fetchUrlTool, webSearchTool, browserClickTool, browserTypeTool, browserScreenshotTool},
	}

	// 創建元數據服務服務端
	metadataServer := &MetadataServer{}

	// 啟動插件服務
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolMetadataGRPCPlugin{
				ToolImpl:     toolServer,
				MetadataImpl: metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}

// ToolMetadataGRPCPlugin 是 gRPC 插件的實現
type ToolMetadataGRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	ToolImpl     proto.ToolServiceServer
	MetadataImpl metadata.PluginMetadataServer
}

func (p *ToolMetadataGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterToolServiceServer(s, p.ToolImpl)
	metadata.RegisterPluginMetadataServer(s, p.MetadataImpl)
	return nil
}

func (p *ToolMetadataGRPCPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &ToolMetadataGRPCClient{
		ToolClient:     proto.NewToolServiceClient(c),
		MetadataClient: metadata.NewPluginMetadataClient(c),
	}, nil
}

type ToolMetadataGRPCClient struct {
	ToolClient     proto.ToolServiceClient
	MetadataClient metadata.PluginMetadataClient
}

func (c *ToolMetadataGRPCClient) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest, opts ...grpc.CallOption) (*proto.ExecuteToolResponse, error) {
	return c.ToolClient.ExecuteTool(ctx, req, opts...)
}

func (c *ToolMetadataGRPCClient) ListTools(ctx context.Context, req *proto.ListToolsRequest, opts ...grpc.CallOption) (*proto.ListToolsResponse, error) {
	return c.ToolClient.ListTools(ctx, req, opts...)
}

func (c *ToolMetadataGRPCClient) GetInfo(ctx context.Context, req *metadata.Empty, opts ...grpc.CallOption) (*metadata.PluginInfo, error) {
	return c.MetadataClient.GetInfo(ctx, req, opts...)
}
