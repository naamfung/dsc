# tool-pdf

DSC 插件：为模型提供读取 PDF 文件以及创建 PDF 文件的能力。

## 设计

基于 [pdfcpu](https://github.com/pdfcpu/pdfcpu) 库（Apache 2.0）实现 PDF 结构解析与内容流提取，
自写文本操作符解释器与字体编码解码层（WinAnsi/MacRoman/StandardEncoding + ToUnicode CMap）。

pdfcpu 本身只解析 PDF 结构（XRefTable、字体字典、内容流字节），不提供「文本提取」能力——
其 `api.ExtractContent` 返回的是 PDF 内容流操作符（如 `[(Hello) -100 (World)] TJ`），不是纯文本。
本插件填补这最后一层：把操作符序列解释为带位置的文本片段，经字体字典解码为 Unicode，按视觉行重组。

创建侧把自带的 TrueType 中文字体注册进 pdfcpu 的字体嵌入机制，走 Type0 嵌入子集路径渲染中文，
并按可用行宽做字符级折行（含避头尾 kinsoku 规则）。

### 架构

```
┌─────────────────────────────────────────────────────────────┐
│  tool-pdf 插件（main.go）                                    │
│    ├─ handleReadText   ─┐                                    │
│    ├─ handleInfo         │                                    │
│    ├─ handleOutline      ├─→ loadPDFContext (缓存 Context)    │
│    ├─ handleSearch       │       ↓                            │
│    └─ handleExtractImages┘   pdfcpu ReadValidateAndOptimize │
│                                                              │
│  text_extractor.go                                          │
│    ├─ loadPageFontDecoders ─→ buildDecoderFromFontDict       │
│    │                              ↓                          │
│    │     ┌── ToUnicode CMap (parseToUnicodeCMap) ────┐       │
│    │     ├── Encoding.Differences (parseDifferences) │       │
│    │     └── 基础编码 (WinAnsi/MacRoman/Standard) ───┘       │
│    ├─ tokenizeContentStream (content_stream.go)              │
│    └─ interpretTextOperators → renderLines (位置感知行重组)  │
│                                                              │
│  font_decoder.go                                            │
│    └─ decode(bytes) → Unicode string                        │
│                                                              │
│  pdf_writer.go + font_cjk.go（创建侧）                      │
│    ├─ resolveFontName：标准 14 或自带 CJK 字体解析           │
│    ├─ wrapTextForRender：CJK 按行宽折行（避头尾）            │
│    └─ createPDFFromText / handleAppendText                  │
│        标准 14 字体：直接写字符码                             │
│        CJK 字体：注册 fonts/ 下 .ttf → Embed=GID → 子集嵌入  │
└─────────────────────────────────────────────────────────────┘
```

### 字体解码优先级（读取侧）

1. **ToUnicode CMap**（最高优先级）：直接给出 Unicode 字符，最可靠
2. **Encoding.Differences**：覆盖基础编码的字符映射
3. **BaseEncoding**：WinAnsi / MacRoman / StandardEncoding
4. **默认**：基础 14 字体用 StandardEncoding，其余用 WinAnsi

不可识别的字节回退为 `?`，保证不返回错误——便于模型判断是否值得继续。

## 模型可见工具

### 读取侧（5 个）

| 工具 | 用途 |
|------|------|
| `pdf_read_text` | 提取纯文本（按页或选页，支持 WinAnsi/MacRoman/CJK ToUnicode CMap 字体解码） |
| `pdf_info` | 元数据（页数、版本、页面尺寸、加密状态、标题/作者/主题/关键词） |
| `pdf_outline` | 书签大纲（目录树） |
| `pdf_search` | 全文搜索关键词（返回命中页号与上下文片段） |
| `pdf_extract_images` | 提取嵌入图片到本地目录 |

### 创建侧（3 个）

| 工具 | 用途 |
|------|------|
| `pdf_create_text` | 从纯文本创建 PDF（自动分页与 CJK 折行，标准 14 字体 + 内置 CJK 字体，A4/Letter/Legal 纸张） |
| `pdf_images_to_pdf` | 图片列表转 PDF（每张图一页，支持 JPG/PNG/TIFF/WEBP） |
| `pdf_append_text` | 向已有 PDF 末尾追加文本页（保留原内容，支持 CJK 字体与折行） |

## 创建 PDF 字体支持

### 标准 14 字体（无需嵌入，开箱即用）

- **Times**: Times-Roman, Times-Bold, Times-Italic, Times-BoldItalic
- **Helvetica**: Helvetica, Helvetica-Bold, Helvetica-Oblique, Helvetica-BoldOblique
- **Courier**: Courier, Courier-Bold, Courier-Oblique, Courier-BoldOblique
- **Symbol**: Symbol（希腊字母与数学符号）
- **ZapfDingbats**: ZapfDingbats（装饰符号）

### 内置 CJK 字体（支持中文等字符，自动嵌入）

**字体需自行下载**：字体体积大，不进公开仓库。`fonts/` 目录中的字体文件未随源码跟踪，
需按 `fonts/字体下载.txt` 的地址自行下载后放入 `plugins/tool-pdf/fonts/`（部署时随插件二进制一起，
运行时查找优先级：可执行文件同级 `fonts/` → 工作目录 `fonts/`）。当前推荐 HarmonyOS Sans（简体中文字重）。

`pdf_create_text` / `pdf_append_text` 的 `font` 参数接受这些 `.ttf` 的文件名主干（不含扩展名），
例如简体中文用 `HarmonyOS_Sans_SC_Regular`。

实现方式：把选中的 `.ttf` 安装进 pdfcpu 的「用户字体注册表」（进程级临时目录，不污染用户主页），
经 `EnsureFontDict` 生成 **Identity-H + CIDToGIDMap Identity** 的 Type0 嵌入子集字体；
`WriteMultiLine` 以 `Embed` 模式把每个 Unicode 码点编码为 2 字节 GID 并累计 `UsedGIDs`，
写入前调用 `UpdateUserfonts` 按已用 GID 收尾（子集化、写宽度/CIDSet/ToUnicode）。
CJK 文本按页面可用行宽做字符级折行，复用 pdfcpu 的 `WordWrapFloat`（自动避头尾，
禁止行首/行尾悬挂禁则标点）。

生成的 PDF 嵌入字体子集并携带 ToUnicode CMap，可被本插件 `pdf_read_text` 及第三方阅读器正常提取中文。

## 沙箱

所有文件路径必须在工作空间根（`DSC_WORKSPACE_ROOT` 环境变量）内。
`out_dir` 参数同样受沙箱约束，防止模型写入工作空间外。
对齐 DSC 沙箱策略（与 `tool-filesystem` 同款校验）。

## 配置

在 `config.yaml` 中声明：

```yaml
plugins:
  - name: tool-pdf
    type: tool
    enabled: true
    binary_path: ./plugins/tool-pdf/tool-pdf
```

`fonts/` 目录不进公开仓库，需按「内置 CJK 字体」一节所述自行下载字体文件，并随插件二进制一起部署
（查找优先级：可执行文件同级 `fonts/` → 工作目录 `fonts/`）。若缺失字体，中文创建会返回「bundled font ... not found」错误。

## 限制

- **TJ 字偶间距启发式**：把 TJ 数组的 kerning 按 em 折算成前向间距，只有达到词间隔下限
  （约 0.22 em）才插空格，且**连续 ≥3 个大间距判定为均匀字距（tracking）不插空格**——
  明显减少代码字体、装饰性行距的误插。尽管如此仍属启发式，极端字距组合仍可能失准。
- **CID 字体无 ToUnicode（读取侧）**：缺 ToUnicode 时已提供回退——从内嵌 TrueType
  （FontFile2）的 cmap 解析 GID→Unicode（Identity CIDToGIDMap 场景），扩大中文可读范围；
  仅当既无 ToUnicode、又非嵌入 TrueType 时才输出 `?`（可经 `pdf_extract_images` 走视觉路径）。
- **加密 PDF**：当前不支持密码输入；加密 PDF 的 `pdf_read_text` 会失败。
- **位置感知简化**：仅按 y 坐标聚合行，不处理多列布局、旋转文本、表格等复杂版面。
- **CJK 分页行距**：用字体真实行高（下限 1.2×字号）计算每页行数，替代固定的 1.5×字号；
  折行后的行数计入分页。