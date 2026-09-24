# dsc-billion-context

DSC 上下文管理插件：以 [acp-kernel](https://github.com/ranxianglei/acp-kernel) 算法接管 DSC 默認的壓縮機制。

## 設計

- **Go 原生重實現 acp-kernel 核心**（對齊 MIT 協議，獨立重寫非移植）
- **經 SDK 通用事件鈎子接管**：`Hook.OnEvent` 攔截 `agent/pre-step` 事件改寫消息列表；`Hook.ContextFn` 貢獻 ACP system prompt（對齊 DSH `ctx.systemPrompt.section`）
- **模型驅動壓縮**：模型決定 *何時* 壓、*壓什麼範圍*，插件編排其他一切
- **3 層 LSM-tree**：T1 標準壓縮、T2 蒸餾（T1→T2）、T3 凝結（T2→T3）
- **前綴緩存穩定**：summary 原位嵌入（不堆到頭部）、不注入 `<acp>` 標籤到消息文本、nudge 作為尾部 user 消息
- **孤兒配對清理**：壓縮邊界切斷 tool-call ↔ tool-result / reasoning ↔ assistant 時自動清理
- **state 持久化**：每個 session 一個 JSON 文件（`ExecDir/billion-context/<session>.json`）

## 4 個模型可見工具

| 工具 | 用途 |
|------|------|
| `compress` | 把消息範圍替換為模型生成的摘要（T1）；用 `bN` 塊引用觸發蒸餾（T2/T3） |
| `decompress` | 恢復壓縮塊內容 |
| `search_context` | 在壓縮塊摘要中搜索關鍵詞（子串匹配） |
| `acp_status` | 查看上下文使用率與可壓縮範圍 |

## 接管流程

1. 用户在 `config.yaml` 中聲明 `compaction: dsc-billion-context` 顯式選擇後端
2. 宿主驗證插件聲明瞭 `Provides: {"compaction": "true"}` 能力（對齊 DSH preset + 能力驗證）
3. 插件經 `Hook.ContextFn` 貢獻 ACP system prompt（壓縮哲學、何時壓縮、工具使用説明、蒸餾規則）
4. agent 每輪 `buildSystemPrompt` 經 `ListContext` 拉取 ACP 指導（對齊 DSH `ctx.systemPrompt.section`）
5. agent 發起 LLM 請求前，宿主 emit `agent/pre-step` 事件
6. 插件收到事件，經 `acp.ProcessTurn` 跑 pipeline：
   - 分配 `mNNNNN` ref 給每條消息（FNV-1a 內容 hash 生成穩定 ID）
   - 應用 prune（被壓縮塊覆蓋的消息替換為 summary，原位嵌入保持前綴穩定）
   - 清理孤兒 tool-call/result/reasoning 配對
   - 決策是否注入 nudge 提示（45% 閾值 + 增長門控 + T2/T3 蒸餾觸發）
7. 返回改寫後的消息列表給宿主，宿主透傳給 LLM
8. 模型調 `compress` 工具時，插件從緩存的 `lastMessages` 解析 `mNNNNN` 邊界執行壓縮
9. 上下文溢出時，插件經 `agent/request-error` 事件觸發緊急壓縮後返回 `{"retry": true}`，宿主重新走 pre-step + 重新調 provider

## 前綴緩存穩定性

| 設計 | 説明 |
|------|------|
| summary 原位嵌入 | 每個 summary 放在它替換的原始範圍的最早消息位置（`insertAt`），不堆到列表頭部 |
| summary 位置穩定 | 經 `acp_summary_<blockId>` 前綴識別"已渲染的 summary"，下次 prune 時保持位置不變 |
| 不注入 `<acp>` 標籤 | `renderTags=false`——標籤的 tokens 屬性每輪估算會變化，破壞前綴緩存 |
| nudge 尾部追加 | nudge 文本每次不同（含 usage%、ranges），作為尾部 user 消息，不影響前綴 |
| ACP system prompt 經 ListContext | 經 `Hook.ContextFn` 貢獻，agent 的 `buildSystemPrompt` 經 `ListContext` 拉取，位置固定 |

## 多層蒸餾

| 層級 | 觸發條件 | 輸入 | 輸出 |
|------|---------|------|------|
| T1 標準壓縮 | 上下文用量 ≥ 45% + nudge | `mNNNNN` 消息範圍 | ~2K token 結構化摘要 |
| T2 蒸餾 | active T1 塊數 ≥ 5（`tier2Trigger`） | `bN` 塊引用（如 `b0..b4`） | ~1K token 高階聚合摘要 |
| T3 凝結 | active T2 塊數 ≥ 10（`tier3Trigger`） | `bN` 塊引用 | 30-60 token/塊 超濃縮事實索引 |

模型在 `compress` 調用中用 `bN` 形式作為 `startId`/`endId` 觸發蒸餾路徑。

## 與 agent 內聯壓縮的關係

agent-react-loop 的內聯 `compactHistory`（80% 閾值）作為**兜底安全網**保留：
- billion-context 在 45% 閾值主動 nudge 模型壓縮，使 80% 正常不被觸發
- agent 經 `ListContext` 響應中的 `[DSC_COMPACTION_BACKEND_ACTIVE]` 標記檢測後端存在
- 後端卸載後標記消失，agent 自動恢復內聯壓縮
- 不依賴 env 或跨進程狀態同步——`ListContext` 每輪實時反映插件生命週期

## 配置

在 `config.yaml` 中聲明：
```yaml
compaction: dsc-billion-context
plugins:
  - name: dsc-billion-context
    type: dsc
    enabled: true
    binary_path: ./plugins/dsc-billion-context/dsc-billion-context
```

環境變量：
- `DSC_CONTEXT_WINDOW`：模型上下文窗口大小（默認 131072）
- `DSC_EXEC_DIR`：state 存儲目錄（默認當前目錄）
- `DSC_SESSION_ID`：當前會話 ID（缺省按工作區路徑派生的項目級鍵，對齊宿主 session
  存儲的 `SessionKeyForProject`——同一項目同名、不同項目隔離；如
  `C:\Users\...\DeepClean` → `C--Users-...-DeepClean.json`）

## 已實現

- T1/T2/T3 完整多層壓縮與蒸餾
- 前綴緩存穩定（原位嵌入 + 不注入標籤 + nudge 尾部追加）
- 孤兒 tool-call/result/reasoning 配對清理
- nudge 決策（閾值門 + 增長門 + baseline 重置 + T2/T3 計數觸發）
- ACP system prompt 經 `Hook.ContextFn` 貢獻（對齊 DSH `ctx.systemPrompt.section`）
- 穩定消息 ID（FNV-1a 內容 hash，不含視圖 index）
- request-error 重試閉環（溢出緊急壓縮 + `{"retry": true}`）
- state JSON 文件持久化
- 4 個工具 handler
- 18 個單元測試

## 與 acp-kernel v0.0.75 的對齊（0.3.0 完工）

「留待後續」六項全部落地（對照上游算法實現，非逐行移植）：

- **absorb 即時工具結果吸收**：`absorb` 第五工具（`DSC_ACP_ABSORB=1` 啓用）——
  達標大工具結果每輪追加 `[ACP absorb]` 強制提示；模型蒸餾後消息對在下一輪
  pre-step 隱藏，摘要成為唯一持久記錄（對齊 absorb.ts 全流程：候選判定/
  寬鬆參數解析/膨脹警告/隱藏兩半）
- **fork/恢復 state 重建**：消息列表即事件日誌——重建按首次見到順序重排 ref
  （原始歷史完整時逐位一致），依序重放 compress 調用鏈（args 範圍 + 結果
  blocksCreated 校驗），塊與 callId 全量還原；state 空白而歷史有壓縮痕跡時
  自動觸發
- **BM25 / fuzzy 混合檢索**：search_context 升級為 hybrid（0.7×歸一化
  BM25(stem) + 0.3×fuzzy 字符 bigram，對齊上游默認算法與基準權重）；文檔集
  = active 塊摘要 + 歷史消息原文（命中消息攜帶歸屬塊 → 直接 decompress）；
  CJK 走上游文檔化 OOV 兜底（bigram+單字，Go 無 CLDR 詞典的有意取捨）
- **hide-compress-calls**：被塊消費的 compress 調用對隱藏；孤兒調用保留最新
  兩對（失敗可觀察 + 殘留封頂，對齊上游 #9 風暴修復）；存活調用 args 內
  重複 summary 壓為 200 字符存根（上游 #336 實測僅重複即 ~22K token）；
  塊的 compressCallId 由 pre-step 依範圍精確匹配從歷史回填
- **emergency-truncate**：佔用 ≥95% 時截斷超大工具輸出為「前綴+標記+後綴」
  （降到目標線即停、近端 5 條保護、文本消息為最後手段、渲染 summary 永不
  觸碰、rune 邊界安全——對齊 #816 surrogate 中毒修復的 Go 等價）；terminal
  escape 信號（#300）同步落地：連續 3 輪近滿無解零收益 → 單次逃逸信號
- **merge-blocks（批量合併 old 塊）**：上游現行版已以「模型驅動 bN 蒸餾 +
  recommend 門控合併」取代自動合併節點——本插件經 T2/T3 nudge 觸發蒸餾
  （既有）+ `MergeRangesToThreshold`（新增，#309 尾部門控：按真實字符量
  批量合併、尾部併入前批、整段低於門不出推薦）承載同一語義

同步落地的上游修復：prune 錨點防裂（v0.0.71 2f60bfb：assistant 連續 run 與
並行工具突發內部不落錨）、sync-blocks 節點（消費/展開/缺席停用 + summary 在場
保持活性）、adapter 工具調用與圖像引用保真往返（改寫輸出不再丟 ToolCalls/
Images——此前會破壞 provider 端 tool-call 協議）。

仍未引入（有意）：cache-report（acp_cache 工具，屬流式宿主記賬面）、wire
codec / transform-channel / filter（dsc 無對應配置面）、語義向量檢索（無嵌入
通路）、per-tier nudge cadence（現用 baseline+增長門控，行為等價已夠）。
