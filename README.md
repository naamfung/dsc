# DSC

本项目系使用 Golang 實現的編程代理系統，遵循 DeepSeek Harness 的一切皆插件的设计哲学，插件都可以热插拔方式加载或卸载。

## 核心功能

- **插件架構**：基於 `go-plugin` 与 gRPC 的宿主與插件通信機制，支持熱插拔加載或卸載。
- **熱重載（Hot Reload）**：支持 DSCPlugin、Agent、LLM 等類型插件的在線熱重載，無需重啟主程序即可更新插件版本。
- **多 LLM 支持**：支持 OpenAI、Anthropic、Ollama 等主流 LLM 提供商；其中 `llm-openai` 與 `llm-anthropic` 插件支持本地 LlamaCpp 推理引擎。
- **ReAct 循環**：實現 Agent 的 Reasoning and Acting 循環，支持多輪推理與工具執行。
- **工具調用與插件化**：支持通過 Tool 插件擴展工具集，內置文件操作與 shell 執行能力。代碼中提供 `WorkspaceProtectionEnabled` 配置項，啟用時可限制文件操作在 `./workspace` 目錄內，防止路徑遍歷攻擊；默認情況下保護機制已啟用，限制模型文件操作在 `./workspace` 目錄內。
- **RPC 可靠性保障**：跨插件 gRPC 調用支持超時控制與指數退避重試機制；採用語義化版本範圍（`>=1.0, <2.0`）進行插件 API 兼容性檢查，允許補丁與次版本升級。
- **TUI 工具調用顯示增強**：在 TUI 界面中，針對 `tool-str-replace-editor` 工具，會根據具體的 `command`（如 `view`, `create`, `str_replace`, `insert`）和 `path` 參數，顯示為 `Edit(View, /root/file/path)`、`Edit(Create, /root/file/path)`、`Edit(StrReplace, /root/file/path)`、`Edit(Insert, /root/file/path)` 等格式，提供更清晰的操作方向性展示。

## 支持的插件

### LLM 插件
- `llm-openai`
- `llm-anthropic`
- `llm-ollama`

### Agent 插件
- `agent-react-loop`

### Tool 插件
- `tool-filesystem`
- `tool-str-replace-editor`
- `tool-browser-use`
- `tool-lisp-eval`
- `tool-skill`

### Policy 插件
- `policy-fs-observation`

## 構建與運行

使用提供的構建腳本編譯主程序與所有插件：

```bash
./build.sh
```

運行主程序並指定 LLM 提供商（默認為 openai）：

```bash
LLM_PROVIDER=openai ./main
```

## 許可證

本項目基於 [Apache-2.0 License](LICENSE) 許可。