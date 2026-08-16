# DSC - Golang Reimplementation of DeepSeek Harness

本项目使用 Go 語言重新實現 DeepSeek Harness 的編程代理（Programming Agent）系統，基於 `go-plugin` 架構，支持多 LLM 插件與 ReAct 循環工具調用。

## 核心功能

- **插件架構**：基於 `go-plugin` 和 gRPC 的宿主與插件通信機制。
- **多 LLM 支持**：支持 OpenAI、Anthropic (Claude) 和 Ollama 模型。
- **ReAct 循環**：實現 Agent 的 Reasoning and Acting 循環，支持多輪推理與工具執行。
- **工具調用**：內置文件操作工具（`read_file`, `write_file`），並支持路徑安全限制（限制在 `./workspace` 目錄內，防止路徑遍歷攻擊）。

## 支持的 LLM 插件

- `llm-openai`
- `llm-anthropic`
- `llm-ollama`

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