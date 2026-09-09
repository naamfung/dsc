package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// 会话全文检索（对齐 DSH session-query + session-query-sqlite）。
//
// DSH 用 SQLite FTS5 做跨会话全文检索，经 tool-session-query 暴露给模型。
// DSC 的适配：用内存倒排索引实现（避免引入 cgo sqlite 依赖），索引按需构建。
// 检索结果按相关度（命中次数）+ 时间衰减排序。
//
// 落点：ExecDir/sessions/ 目录下所有 .jsonl 文件。

// SessionSearchResult 一条会话检索结果。
type SessionSearchResult struct {
	SessionID    string `json:"session_id"`
	Snippet      string `json:"snippet"` // 命中上下文片段
	Score        int    `json:"score"`   // 命中次数
	LastModified int64  `json:"last_modified"`
}

// SessionSearcher 会话全文检索器。
type SessionSearcher struct {
	mu    sync.RWMutex
	dir   string
	index map[string][]*searchEntry // sessionID → entries
	built bool
}

type searchEntry struct {
	text string
	seq  int
}

// NewSessionSearcher 创建会话检索器。dir 为会话文件目录。
func NewSessionSearcher(dir string) *SessionSearcher {
	return &SessionSearcher{
		dir:   dir,
		index: make(map[string][]*searchEntry),
	}
}

// Rebuild 重建索引。扫描 dir 下所有 .jsonl 文件，提取 user/assistant 消息文本。
func (s *SessionSearcher) Rebuild() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.index = make(map[string][]*searchEntry)

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil // 目录不存在不报错
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		path := filepath.Join(s.dir, entry.Name())
		texts, err := extractTexts(path)
		if err != nil {
			continue
		}
		s.index[sessionID] = texts
	}
	s.built = true
	return nil
}

// Search 执行全文检索。返回按相关度+时间衰减排序的结果列表。
func (s *SessionSearcher) Search(ctx context.Context, query string, limit int) ([]SessionSearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if !s.built {
		if err := s.Rebuild(); err != nil {
			return nil, err
		}
	}

	keywords := strings.Fields(strings.ToLower(query))
	if len(keywords) == 0 {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []SessionSearchResult
	for sessionID, entries := range s.index {
		score := 0
		var bestSnippet string
		for _, entry := range entries {
			lowerText := strings.ToLower(entry.text)
			matches := 0
			for _, kw := range keywords {
				if strings.Contains(lowerText, kw) {
					matches++
				}
			}
			if matches > score {
				score = matches
				bestSnippet = extractSnippet(entry.text, keywords, 100)
			}
		}
		if score > 0 {
			info, _ := os.Stat(filepath.Join(s.dir, sessionID+".jsonl"))
			var lastMod int64
			if info != nil {
				lastMod = info.ModTime().UnixMilli()
			}
			results = append(results, SessionSearchResult{
				SessionID:    sessionID,
				Snippet:      bestSnippet,
				Score:        score,
				LastModified: lastMod,
			})
		}
	}

	// 按 score 降序，相同 score 按 lastModified 降序
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].LastModified > results[j].LastModified
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// extractTexts 从会话 JSONL 文件提取所有 user/assistant 消息文本。
func extractTexts(path string) ([]*searchEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []*searchEntry
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 简单提取：找 "content" 字段
		content := extractJSONField(line, "content")
		if content != "" {
			entries = append(entries, &searchEntry{text: content})
		}
	}
	return entries, nil
}

// extractJSONField 从单行 JSON 中简单提取字段值（避免完整 JSON 解析开销）。
func extractJSONField(line, field string) string {
	key := `"` + field + `":`
	idx := strings.Index(line, key)
	if idx < 0 {
		return ""
	}
	start := idx + len(key)
	// 跳过空白
	for start < len(line) && (line[start] == ' ' || line[start] == '\t') {
		start++
	}
	if start >= len(line) {
		return ""
	}
	if line[start] == '"' {
		// 字符串值
		start++
		end := start
		for end < len(line) {
			if line[end] == '\\' && end+1 < len(line) {
				end += 2
				continue
			}
			if line[end] == '"' {
				break
			}
			end++
		}
		if end > len(line) {
			end = len(line)
		}
		return unescapeJSON(line[start:end])
	}
	return ""
}

// extractSnippet 提取命中关键词周围的上下文片段。
func extractSnippet(text string, keywords []string, maxLen int) string {
	lower := strings.ToLower(text)
	bestPos := 0
	bestMatches := 0
	for _, kw := range keywords {
		pos := strings.Index(lower, kw)
		if pos >= 0 {
			bestPos = pos
			bestMatches++
			break
		}
	}
	_ = bestMatches
	start := bestPos - maxLen/2
	if start < 0 {
		start = 0
	}
	end := start + maxLen
	if end > len(text) {
		end = len(text)
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return text
	}
	runeStart := 0
	bytePos := 0
	for bytePos < start && bytePos < len(text) {
		_, size := decodeRune(text, bytePos)
		bytePos += size
		runeStart++
	}
	runeEnd := runeStart
	for bytePos < end && bytePos < len(text) {
		_, size := decodeRune(text, bytePos)
		bytePos += size
		runeEnd++
	}
	if runeEnd > len(runes) {
		runeEnd = len(runes)
	}
	if runeStart >= len(runes) {
		runeStart = 0
		runeEnd = 0
	}
	if runeStart > 0 {
		return "…" + string(runes[runeStart:runeEnd])
	}
	return string(runes[runeStart:runeEnd])
}

func unescapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\"`, `"`)
	s = strings.ReplaceAll(s, `\\`, `\`)
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}

func decodeRune(s string, pos int) (rune, int) {
	if pos >= len(s) {
		return 0, 0
	}
	b := s[pos]
	if b < 0x80 {
		return rune(b), 1
	}
	// 简化 UTF-8 解码
	for i := 2; i <= 4; i++ {
		if pos+i <= len(s) {
			r := []byte(s[pos : pos+i])
			if r[0]&0xC0 != 0x80 {
				return rune(r[0]), 1
			}
		}
	}
	return rune(b), 1
}

// FormatSearchResults 格式化检索结果为模型可读文本。
func FormatSearchResults(results []SessionSearchResult) string {
	if len(results) == 0 {
		return "未找到匹配的会话。"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("找到 %d 个匹配会话：\n\n", len(results)))
	for i, r := range results {
		t := time.UnixMilli(r.LastModified).Format("2006-01-02 15:04")
		sb.WriteString(fmt.Sprintf("%d. [%s] %s (命中 %d 次)\n   %s\n\n",
			i+1, t, r.SessionID, r.Score, r.Snippet))
	}
	return sb.String()
}
