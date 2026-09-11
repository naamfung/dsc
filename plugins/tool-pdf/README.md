# tool-pdf

DSC 插件：为模型提供读取 PDF 文件以获取内容信息的能力。

## 设计

基于 [pdfcpu](https://github.com/pdfcpu/pdfcpu) 库（Apache 2.0）实现 PDF 结构解析与内容流提取，
自写文本操作符解释器与字体编码解码层（WinAnsi/MacRoman/StandardEncoding + ToUnicode CMap）。

pdfcpu 本身只解析 PDF 结构（XRefTable、字体字典、内容流字节），不提供「文本提取」能力——
其 `api.ExtractContent` 返回的是 PDF 内容流操作符（如 `[(Hello) -100 (World)] TJ`），不是纯文本。
本插件填补这最后一层：把操作符序列解释为带位置的文本片段，经字体字典解码为 Unicode，按视觉行重组。

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
└─────────────────────────────────────────────────────────────┘
```

### 字体解码优先级

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
| `pdf_create_text` | 从纯文本创建 PDF（自动分页，标准 14 字体，A4/Letter/Legal 纸张） |
| `pdf_images_to_pdf` | 图片列表转 PDF（每张图一页，支持 JPG/PNG/TIFF/WEBP） |
| `pdf_append_text` | 向已有 PDF 末尾追加文本页（保留原内容） |

## 创建 PDF 字体支持

仅支持 PDF 标准 14 字体（无需嵌入，开箱即用）：

- **Times**: Times-Roman, Times-Bold, Times-Italic, Times-BoldItalic
- **Helvetica**: Helvetica, Helvetica-Bold, Helvetica-Oblique, Helvetica-BoldOblique
- **Courier**: Courier, Courier-Bold, Courier-Oblique, Courier-BoldOblique
- **Symbol**: Symbol（希腊字母与数学符号）
- **ZapfDingbats**: ZapfDingbats（装饰符号）

**中文/CJK 限制**：标准 14 字体不含 CJK 字形。如需生成含中文的 PDF，建议：
1. 用 `pdf_images_to_pdf` 把渲染好的图片（含中文）封装为 PDF
2. 或在宿主层用视觉模型直接生成 PDF 内容

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

## 限制

- **TJ 字偶间距启发式**：当前用 `-500` 阈值判断是否在 TJ 数组中插入空格。
  部分使用大量字偶间距的 PDF（如代码字体）可能在字符间误插空格。
- **CID 字体无 ToUnicode**：若 CID 字体（CJK 常用）缺少 ToUnicode CMap，
  无法可靠解码（输出 `?` 占位）。可经 `pdf_extract_images` 走视觉路径。
- **加密 PDF**：当前不支持密码输入；加密 PDF 的 `pdf_read_text` 会失败。
- **位置感知简化**：仅按 y 坐标聚合行，不处理多列布局、旋转文本、表格等复杂版面。
