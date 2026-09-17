package acp

import (
	"math"
	"sort"
	"strings"
)

// search_blocks（对齐 acp-kernel search/：bm25 + fuzzy 混合检索，默认算法
// hybrid = 0.7×归一化 BM25(stem) + 0.3×归一化 fuzzy 字符 bigram）。
//
// 文档集 = 压缩块摘要（active 与 inactive 全量，bN 引用）+ 历史消息原文
// （mNNNNN 引用，命中后经 ownerOf 知道 decompress 哪个块）。
//
// 分词（对齐 tokenizer.ts）：Latin 按 [a-z][a-z0-9_]* 词切分 + 轻量词干化；
// CJK 无 Intl.Segmenter（Go 无 CLDR 词典）——采用上游文档化的 OOV 兜底策略：
// 连续 CJK 段做重叠 bigram + 单字，召回优先（精度由 BM25 的 IDF 与 hybrid
// 的 fuzzy 通道补偿）。Latin 短词（2-3 字符）多为停用词噪声，fuzzy 门控排除；
// CJK 2 字词是原子词（登录/缓存），门控放宽到 ≥2。

// WBM25 / WFuzzy hybrid 权重（对齐 hybrid.ts，基准 MRR 0.898）。
const (
	WBM25    = 0.7
	WFuzzy   = 0.3
	k1BM25   = 1.2
	bBM25    = 0.75
	minScore = 0.01
)

// stem 轻量英文词干化（对齐 stemmer.ts，Porter 简化版；CJK 不处理）。
func stem(word string) string {
	if len(word) <= 3 {
		return word
	}
	w := word
	switch {
	case strings.HasSuffix(w, "ies"):
		w = w[:len(w)-3] + "y"
	case strings.HasSuffix(w, "ses") || strings.HasSuffix(w, "xes") || strings.HasSuffix(w, "zes"):
		w = w[:len(w)-2]
	case strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes"):
		w = w[:len(w)-2]
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		w = w[:len(w)-1]
	}
	if strings.HasSuffix(w, "ing") && len(w) > 5 {
		w = w[:len(w)-3]
	}
	if strings.HasSuffix(w, "ed") && len(w) > 4 {
		w = w[:len(w)-2]
	}
	switch {
	case strings.HasSuffix(w, "ation") && len(w) > 6:
		w = w[:len(w)-3]
	case strings.HasSuffix(w, "tion") && len(w) > 5:
		w = w[:len(w)-4] + "t"
	case strings.HasSuffix(w, "ion") && len(w) > 4:
		w = w[:len(w)-3]
	}
	if strings.HasSuffix(w, "ment") && len(w) > 6 {
		w = w[:len(w)-4]
	}
	if strings.HasSuffix(w, "ness") && len(w) > 6 {
		w = w[:len(w)-4]
	}
	if strings.HasSuffix(w, "ly") && len(w) > 4 {
		w = w[:len(w)-2]
	}
	return w
}

func isCJKRune(r rune) bool {
	return (r >= 0x3400 && r <= 0x9fff) || (r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0x3040 && r <= 0x30ff) || (r >= 0xac00 && r <= 0xd7af)
}

// tokenize 混合分词：Latin 词 + 词干化；CJK 连续段 → 重叠 bigram + 单字。
func tokenize(text string, doStem bool) []string {
	lower := strings.ToLower(text)
	var tokens []string

	// Latin：连续 [a-z0-9_] 段，去掉首尾下划线，≥2 才保留
	var cur strings.Builder
	flush := func() {
		w := strings.Trim(cur.String(), "_")
		cur.Reset()
		if len(w) >= 2 {
			if doStem {
				w = stem(w)
			}
			tokens = append(tokens, w)
		}
	}
	hasCJK := false
	for _, r := range lower {
		if isCJKRune(r) {
			hasCJK = true
			flush()
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if !hasCJK {
		return tokens
	}

	// CJK 连续段 → bigram + 单字（OOV 兜底策略）
	var run []rune
	drain := func() {
		if len(run) == 0 {
			return
		}
		for i := 0; i+1 < len(run); i++ {
			tokens = append(tokens, string(run[i:i+2]))
		}
		for _, r := range run {
			tokens = append(tokens, string(r))
		}
		run = run[:0]
	}
	for _, r := range lower {
		if isCJKRune(r) {
			run = append(run, r)
			continue
		}
		drain()
	}
	drain()
	return tokens
}

// charBigrams 字符 bigram 集（跳过含空白的组合，对齐 charBigrams）。
func charBigrams(text string) map[string]bool {
	runes := []rune(text)
	out := map[string]bool{}
	for i := 0; i+1 < len(runes); i++ {
		pair := string(runes[i : i+2])
		if strings.TrimSpace(pair) == pair {
			out[pair] = true
		}
	}
	return out
}

// SearchDoc 检索文档（块摘要或历史消息）。
type SearchDoc struct {
	Kind    string // "block" | "message"
	Ref     string // "b3" | "m00350"
	Text    string // 打分文本（块=topic+summary；消息=原文）
	Title   string
	Role    string // 消息角色（块为空）
	BlockID string // 归属块（消息命中时指向 decompress 目标）
	Tier    int
	Tokens  int
}

// ScoredDoc 打分结果。
type ScoredDoc struct {
	Ref   string  `json:"ref"`
	Score float64 `json:"score"`
}

// tfMap 词频表。
func tfMap(text string, doStem bool) map[string]int {
	m := map[string]int{}
	for _, t := range tokenize(text, doStem) {
		m[t]++
	}
	return m
}

// bm25Score BM25（k1=1.2, b=0.75，IDF 下压跨语料常见词）。
func bm25Score(docs []SearchDoc, query string) []ScoredDoc {
	n := len(docs)
	parsed := make([]struct {
		id string
		tf map[string]int
		l  int
	}, 0, n)
	totalLen := 0
	for _, d := range docs {
		tf := tfMap(d.Text, true)
		l := len(tokenize(d.Text, true))
		parsed = append(parsed, struct {
			id string
			tf map[string]int
			l  int
		}{d.Ref, tf, l})
		totalLen += l
	}
	avgdl := float64(totalLen) / float64(n)
	if avgdl <= 0 {
		avgdl = 1
	}

	qTerms := tokenize(query, true)
	if len(qTerms) == 0 {
		return nil
	}
	idf := map[string]float64{}
	seen := map[string]bool{}
	for _, t := range qTerms {
		if seen[t] {
			continue
		}
		seen[t] = true
		df := 0
		for _, d := range parsed {
			if d.tf[t] > 0 {
				df++
			}
		}
		idf[t] = log1p(1 + (float64(n-df)+0.5)/(float64(df)+0.5))
	}
	out := make([]ScoredDoc, 0, n)
	for _, d := range parsed {
		score := 0.0
		for _, t := range qTerms {
			f := float64(d.tf[t])
			if f == 0 {
				continue
			}
			score += idf[t] * (f * (k1BM25 + 1)) / (f + k1BM25*(1-bBM25+(bBM25*float64(d.l))/avgdl))
		}
		out = append(out, ScoredDoc{Ref: d.id, Score: score})
	}
	return out
}

// fuzzyScore 字符 bigram Jaccard 式重叠（容错/跨脚本召回通道）。
func fuzzyScore(docs []SearchDoc, query string) []ScoredDoc {
	// 门控：Latin ≥4；CJK ≥2（2 字 CJK 是原子词）
	var qTokens []string
	for _, t := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return r == ' ' || r == ',' }) {
		hasCJK := strings.IndexFunc(t, isCJKRune) >= 0
		if len([]rune(t)) >= 4 || (len([]rune(t)) >= 2 && hasCJK) {
			qTokens = append(qTokens, t)
		}
	}
	qGrams := map[string]bool{}
	for _, t := range qTokens {
		for g := range charBigrams(t) {
			qGrams[g] = true
		}
	}
	out := make([]ScoredDoc, 0, len(docs))
	for _, d := range docs {
		docGrams := charBigrams(d.Text)
		hits := 0
		for g := range qGrams {
			if docGrams[g] {
				hits++
			}
		}
		out = append(out, ScoredDoc{Ref: d.Ref, Score: float64(hits) / float64(len(qGrams)|1)})
	}
	return out
}

// hybridScore 混合打分：各自 max 归一化后 0.7/0.3 加权。
func hybridScore(docs []SearchDoc, query string) []ScoredDoc {
	bm := bm25Score(docs, query)
	fz := fuzzyScore(docs, query)
	maxOf := func(xs []ScoredDoc) float64 {
		m := 1e-9
		for _, x := range xs {
			if x.Score > m {
				m = x.Score
			}
		}
		return m
	}
	maxBm, maxFz := maxOf(bm), maxOf(fz)
	bmMap, fzMap := map[string]float64{}, map[string]float64{}
	for _, x := range bm {
		bmMap[x.Ref] = x.Score / maxBm
	}
	for _, x := range fz {
		fzMap[x.Ref] = x.Score / maxFz
	}
	out := make([]ScoredDoc, 0, len(docs))
	for _, d := range docs {
		out = append(out, ScoredDoc{Ref: d.Ref, Score: WBM25*(bmMap[d.Ref]) + WFuzzy*(fzMap[d.Ref])})
	}
	return out
}

// RoleWeights 角色加权（用户意图 > 助手推理 > 工具噪声；块 1.0）。
type RoleWeights struct {
	User      float64
	Assistant float64
	Tool      float64
	Block     float64
}

// DefaultRoleWeights 默认角色权重（对齐 DEFAULT_ROLE_WEIGHTS）。
var DefaultRoleWeights = RoleWeights{User: 1.5, Assistant: 1.0, Tool: 0.6, Block: 1.0}

// SearchOptions 检索选项。
type SearchOptions struct {
	Limit         int // 默认 10
	PreviewLength int // 默认 200
}

// SearchResultItem 检索结果项（模型可见形态）。
type SearchResultItem struct {
	Kind    string  `json:"kind"`
	Ref     string  `json:"ref"`
	BlockID string  `json:"blockId,omitempty"`
	Tier    int     `json:"tier,omitempty"`
	Score   float64 `json:"score"`
	Title   string  `json:"title"`
	Preview string  `json:"preview"`
	Role    string  `json:"role,omitempty"`
	Tokens  int     `json:"tokens,omitempty"`
}

// SearchBlocks 混合检索入口（对齐 searchBlocks：blockDocs + messageDocs 统一打分）。
func SearchBlocks(docs []SearchDoc, query string, opts SearchOptions) []SearchResultItem {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	previewLen := opts.PreviewLength
	if previewLen <= 0 {
		previewLen = 200
	}
	if len(docs) == 0 {
		return nil
	}
	scored := hybridScore(docs, query)
	byRef := map[string]SearchDoc{}
	for _, d := range docs {
		byRef[d.Ref] = d
	}
	rw := DefaultRoleWeights
	var out []SearchResultItem
	for _, s := range scored {
		d, ok := byRef[s.Ref]
		if !ok || s.Score < minScore {
			continue
		}
		w := rw.Block
		switch d.Kind {
		case "message":
			switch d.Role {
			case "user":
				w = rw.User
			case "assistant":
				w = rw.Assistant
			default:
				w = rw.Tool
			}
		}
		out = append(out, SearchResultItem{
			Kind:    d.Kind,
			Ref:     d.Ref,
			BlockID: d.BlockID,
			Tier:    d.Tier,
			Score:   s.Score * w,
			Title:   d.Title,
			Preview: makePreview(d.Text, query, previewLen),
			Role:    d.Role,
			Tokens:  d.Tokens,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// makePreview 围绕首个命中的定位预览（对齐 makePreview 语义：命中窗口截取）。
func makePreview(text, query string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	pos := -1
	lowerText := strings.ToLower(text)
	for _, t := range tokenize(query, false) {
		if i := strings.Index(lowerText, strings.ToLower(t)); i >= 0 {
			// 字节索引 → rune 索引
			pos = runeLen(lowerText[:i])
			break
		}
	}
	if pos < 0 {
		return clampPrefix(text, maxLen)
	}
	start := pos - maxLen/4
	if start < 0 {
		start = 0
	}
	window := clampWindow(text, start, start+maxLen)
	if start > 0 {
		window = "…" + window
	}
	if start+maxLen < len(runes) {
		window += "…"
	}
	return window
}

// log1p 自然对数 ln(1+x)（math.Log，IR 打分用）。
func log1p(x float64) float64 { return math.Log(1 + x) }

// BlockDocs 块摘要文档集（active 块；本插件检索面向可 decompress 的活跃块，
// 与既有 Search 语义一致——上游含 inactive 属历史可选差异）。
func BlockDocs(state *CompressionState) []SearchDoc {
	docs := make([]SearchDoc, 0, len(state.Blocks))
	for _, b := range state.Blocks {
		if !b.Active {
			continue
		}
		docs = append(docs, SearchDoc{
			Kind:    "block",
			Ref:     b.BlockID,
			Text:    b.Topic + " " + b.Summary,
			Title:   b.Topic,
			BlockID: b.BlockID,
			Tier:    int(b.Tier),
			Tokens:  b.CompressedTokens,
		})
	}
	return docs
}

// MessageDocs 历史消息文档集：每条消息原文为一文档（ref=mNNNNN），
// 归属块 = 覆盖它的 active 块（命中消息即知道 decompress 目标）。
func MessageDocs(messages []CoreMessage, state *CompressionState) []SearchDoc {
	owner := map[string]string{}
	tier := map[string]int{}
	for _, b := range state.ActiveBlocks() {
		for _, id := range b.EffectiveMessageIDs {
			owner[id] = b.BlockID
			tier[id] = int(b.Tier)
		}
	}
	docs := make([]SearchDoc, 0, len(messages))
	for _, m := range messages {
		if m.Text == "" {
			continue
		}
		ref := RefForRaw(m.ID, state)
		if ref == "" {
			continue
		}
		title := string(m.Role) + ": " + clampPrefix(m.Text, 60)
		docs = append(docs, SearchDoc{
			Kind:    "message",
			Ref:     ref,
			Text:    m.Text,
			Title:   title,
			Role:    string(m.Role),
			BlockID: owner[m.ID],
			Tier:    tier[m.ID],
			Tokens:  estimateTokensForText(m.Text),
		})
	}
	return docs
}
