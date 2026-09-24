# tool-pdf

DSC 插件：為模型提供讀取 PDF 文件以及創建 PDF 文件的能力。

## 設計

基於 [pdfcpu](https://github.com/pdfcpu/pdfcpu) 庫（Apache 2.0）實現 PDF 結構解析與內容流提取，
自寫文本操作符解釋器與字體編碼解碼層（WinAnsi/MacRoman/StandardEncoding + ToUnicode CMap）。

pdfcpu 本身只解析 PDF 結構（XRefTable、字體字典、內容流字節），不提供「文本提取」能力——
其 `api.ExtractContent` 返回的是 PDF 內容流操作符（如 `[(Hello) -100 (World)] TJ`），不是純文本。
本插件填補這最後一層：把操作符序列解釋為帶位置的文本片段，經字體字典解碼為 Unicode，按視覺行重組。

創建側把自帶的 TrueType 中文字體註冊進 pdfcpu 的字體嵌入機制，走 Type0 嵌入子集路徑渲染中文，
並按可用行寬做字符級折行（含避頭尾 kinsoku 規則）。

### 架構

```
┌─────────────────────────────────────────────────────────────┐
│  tool-pdf 插件（main.go）                                    │
│    ├─ handleReadText   ─┐                                    │
│    ├─ handleInfo         │                                    │
│    ├─ handleOutline      ├─→ loadPDFContext (緩存 Context)    │
│    ├─ handleSearch       │       ↓                            │
│    └─ handleExtractImages┘   pdfcpu ReadValidateAndOptimize │
│                                                              │
│  text_extractor.go                                          │
│    ├─ loadPageFontDecoders ─→ buildDecoderFromFontDict       │
│    │                              ↓                          │
│    │     ┌── ToUnicode CMap (parseToUnicodeCMap) ────┐       │
│    │     ├── Encoding.Differences (parseDifferences) │       │
│    │     └── 基礎編碼 (WinAnsi/MacRoman/Standard) ───┘       │
│    ├─ tokenizeContentStream (content_stream.go)              │
│    └─ interpretTextOperators → renderLines (位置感知行重組)  │
│                                                              │
│  font_decoder.go                                            │
│    └─ decode(bytes) → Unicode string                        │
│                                                              │
│  pdf_writer.go + font_cjk.go（創建側）                      │
│    ├─ resolveFontName：標準 14 或自帶 CJK 字體解析           │
│    ├─ wrapTextForRender：CJK 按行寬折行（避頭尾）            │
│    └─ createPDFFromText / handleAppendText                  │
│        標準 14 字體：直接寫字符碼                             │
│        CJK 字體：註冊 fonts/ 下 .ttf → Embed=GID → 子集嵌入  │
└─────────────────────────────────────────────────────────────┘
```

### 字體解碼優先級（讀取側）

1. **ToUnicode CMap**（最高優先級）：直接給出 Unicode 字符，最可靠
2. **Encoding.Differences**：覆蓋基礎編碼的字符映射
3. **BaseEncoding**：WinAnsi / MacRoman / StandardEncoding
4. **默認**：基礎 14 字體用 StandardEncoding，其餘用 WinAnsi

不可識別的字節回退為 `?`，保證不返回錯誤——便於模型判斷是否值得繼續。

## 模型可見工具

### 讀取側（6 個）

| 工具 | 用途 |
|------|------|
| `pdf_read_text` | 提取純文本（按頁或選頁，支持 WinAnsi/MacRoman/CJK ToUnicode CMap 字體解碼） |
| `pdf_extract_tables` | 檢測並輸出表格結構（列對齊網格，列間以豎線分隔） |
| `pdf_info` | 元數據（頁數、版本、頁面尺寸、加密狀態、標題/作者/主題/關鍵詞） |
| `pdf_outline` | 書籤大綱（目錄樹） |
| `pdf_search` | 全文搜索關鍵詞（返回命中頁號與上下文片段） |
| `pdf_extract_images` | 提取嵌入圖片到本地目錄 |

### 創建與整理側（6 個）

| 工具 | 用途 |
|------|------|
| `pdf_create_text` | 從純文本創建 PDF（自動分頁與 CJK 折行，標準 14 字體 + 內置 CJK 字體，A4/Letter/Legal 紙張） |
| `pdf_images_to_pdf` | 圖片列表轉 PDF（每張圖一頁，支持 JPG/PNG/TIFF/WEBP） |
| `pdf_append_text` | 向已有 PDF 末尾追加文本頁（保留原內容，支持 CJK 字體與折行） |
| `pdf_merge_pdfs` | 合併多個 PDF 為一個（保序，可選分隔頁） |
| `pdf_split_pdfs` | 按頁拆分為多個 PDF（默認每頁一段，可指定 span） |
| `pdf_extract_pages` | 從 PDF 抽選頁生成新 PDF（如 `"1,3,5-7"`） |

> 頁面轉圖（`pdf_to_images`）**當前臨時禁用**：其依賴外部渲染器（mutool / pdftoppm / Ghostscript），
> 未真機驗證。實現保留在 `pdf_render.go`，恢復時取消 `main.go` 中註冊塊註釋即可。

## 創建 PDF 字體支持

### 標準 14 字體（無需嵌入，開箱即用）

- **Times**: Times-Roman, Times-Bold, Times-Italic, Times-BoldItalic
- **Helvetica**: Helvetica, Helvetica-Bold, Helvetica-Oblique, Helvetica-BoldOblique
- **Courier**: Courier, Courier-Bold, Courier-Oblique, Courier-BoldOblique
- **Symbol**: Symbol（希臘字母與數學符號）
- **ZapfDingbats**: ZapfDingbats（裝飾符號）

### 內置 CJK 字體（支持中文等字符，自動嵌入）

**字體需自行下載（啓動強制校驗）**：字體體積大，不進公開倉庫，`fonts/` 目錄中的字體文件未隨源碼跟蹤。
插件啓動時檢查 fonts 目錄，未檢測到任何 `.ttf` 字體（中文渲染必需）則直接 PANIC 拒絕啓動並打印下載指引——
使用本插件必須先下載字體。需按 `fonts/字体下载.txt` 的地址自行下載後放入 `plugins/tool-pdf/fonts/`
（部署時隨插件二進制一起，運行時查找優先級：環境變量 `TOOL_PDF_FONTS_DIR` → 可執行文件同級 `fonts/` →
工作目錄 `fonts/`）。當前推薦 HarmonyOS Sans（簡體中文字重）。

`pdf_create_text` / `pdf_append_text` 的 `font` 參數接受這些 `.ttf` 的文件名主幹（不含擴展名），
例如簡體中文用 `HarmonyOS_Sans_SC_Regular`。

實現方式：把選中的 `.ttf` 安裝進 pdfcpu 的「用户字體註冊表」（進程級臨時目錄，不污染用户主頁），
經 `EnsureFontDict` 生成 **Identity-H + CIDToGIDMap Identity** 的 Type0 嵌入子集字體；
`WriteMultiLine` 以 `Embed` 模式把每個 Unicode 碼點編碼為 2 字節 GID 並累計 `UsedGIDs`，
寫入前調用 `UpdateUserfonts` 按已用 GID 收尾（子集化、寫寬度/CIDSet/ToUnicode）。
CJK 文本按頁面可用行寬做字符級折行，複用 pdfcpu 的 `WordWrapFloat`（自動避頭尾，
禁止行首/行尾懸掛禁則標點）。

生成的 PDF 嵌入字體子集並攜帶 ToUnicode CMap，可被本插件 `pdf_read_text` 及第三方閲讀器正常提取中文。

## 沙箱

所有文件路徑必須在工作空間根（`DSC_WORKSPACE_ROOT` 環境變量）內。
`out_dir` 參數同樣受沙箱約束，防止模型寫入工作空間外。
對齊 DSC 沙箱策略（與 `tool-filesystem` 同款校驗）。

## 配置

在 `config.yaml` 中聲明：

```yaml
plugins:
  - name: tool-pdf
    type: tool
    enabled: true
    binary_path: ./plugins/tool-pdf/tool-pdf
```

`fonts/` 目錄不進公開倉庫，需按「內置 CJK 字體」一節所述自行下載字體文件，並隨插件二進制一起部署
（查找優先級：可執行文件同級 `fonts/` → 工作目錄 `fonts/`）。若缺失字體，中文創建會返回「bundled font ... not found」錯誤。

## 限制

- **TJ 字偶間距啓發式**：把 TJ 數組的 kerning 按 em 折算成前向間距，只有達到詞間隔下限
  （約 0.22 em）才插空格，且**連續 ≥3 個大間距判定為均勻字距（tracking）不插空格**——
  明顯減少代碼字體、裝飾性行距的誤插。儘管如此仍屬啓發式，極端字距組合仍可能失準。
- **CID 字體無 ToUnicode（讀取側）**：缺 ToUnicode 時已提供回退——從內嵌 TrueType
  （FontFile2）的 cmap 解析 GID→Unicode（Identity CIDToGIDMap 場景），擴大中文可讀範圍；
  僅當既無 ToUnicode、又非嵌入 TrueType 時才輸出 `?`（可經 `pdf_extract_images` 走視覺路徑）。
- **加密 PDF**：當前不支持密碼輸入；加密 PDF 的 `pdf_read_text` 會失敗。
- **位置感知**：按 y 聚合行；行內按橫向間隙識別多欄並以製表符分隔；`Tm` 的旋轉角
  非零文本單獨分區輸出（標註 `〔rotate N°〕`），避免混入正常行。不恢復斜排/鏡像等複雜版面。
- **表格提取為啓發式**：`pdf_extract_tables` 基於整頁 x 對齊聚類，僅覆蓋常規報表/發票/
  日程等對齊列布局，不恢復表格邊框、合併單元格。
- **頁面轉圖暫時禁用**：`pdf_to_images` 依賴外部渲染器且未真機驗證，當前未註冊；恢復方式見上文。
- **CJK 分頁行距**：用字體真實行高（下限 1.2×字號）計算每頁行數，替代固定的 1.5×字號；
  折行後的行數計入分頁。