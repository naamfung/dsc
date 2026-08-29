#!/bin/bash
set -e

# ============================================================
# build.sh - Unix/Linux/macOS 构建脚本
# 注意：此脚本与 build.bat 须要同步更新
# ============================================================

# 设置 GOPATH 和 PATH 以包含 protoc 生成插件
GOPATH=$(go env GOPATH)
export PATH="$PATH:$GOPATH/bin"

# 编译 proto 文件：仅当检测到 protoc 编译器时才重新生成；未检测到则跳过。
# 理由：proto 代码（*.pb.go）已纳入仓库，且 protoc 编译器二进制不宜入库，
# 用户没有编译器时不应报错，直接复用仓库内已生成的代码即可。
echo "Generating proto Go code..."
PROTOC_BIN=""
if [ -f "protoc_dir/bin/protoc.exe" ]; then
    PROTOC_BIN="protoc_dir/bin/protoc.exe"
elif command -v protoc >/dev/null 2>&1; then
    PROTOC_BIN="protoc"
fi

if [ -n "$PROTOC_BIN" ]; then
    # 安装 protoc 生成插件（如果尚未安装）
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
    "$PROTOC_BIN" --proto_path=proto --go_out=proto --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative proto/dsc.proto
else
    echo "  (skipped: 未检测到 protoc 编译器，使用仓库内已生成的 proto 代码)"
fi

# ---- UPX 压缩检测 ----
# 当系统存在 upx 工具时，用它压缩构建产物以显著减小体积；未检测到则跳过。
# 压缩失败仅告警并保留原始二进制，不中断后续构建。
GOOS="$(go env GOOS)"
UPX_BIN=""
if command -v upx >/dev/null 2>&1; then
    UPX_BIN="upx"
    echo "UPX detected: $UPX_BIN (构建产物将自动压缩)"
else
    echo "UPX not detected (跳过压缩，可使用 --best 手动压缩以减小体积)"
fi

# pack 压缩单个二进制；未检测到 upx 或文件不存在/压缩失败时均安全跳过。
pack() {
    if [ -z "$UPX_BIN" ]; then return; fi
    if [ -f "$1" ]; then
        if "$UPX_BIN" --best "$1"; then
            :
        else
            echo "  (warn: UPX 压缩失败，保留原始二进制: $1)"
        fi
    else
        echo "  (warn: 构建产物未找到，跳过压缩: $1)"
    fi
}

# 构建主程序
echo "Building main program..."
go build
MAIN_BIN="dsc"
if [ "$GOOS" = "windows" ]; then MAIN_BIN="dsc.exe"; fi
pack "$MAIN_BIN"

# 构建 agent-react-loop 插件
echo "Building agent-react-loop plugin..."
(cd plugins/agent-react-loop && go build -o agent-react-loop.exe .)
pack plugins/agent-react-loop/agent-react-loop.exe

# 构建 llm-openai 插件
echo "Building llm-openai plugin..."
(cd plugins/llm-openai && go build -o llm-openai.exe .)
pack plugins/llm-openai/llm-openai.exe

# 构建 llm-anthropic 插件（独立 module，基于 dsc-sdk）
echo "Building llm-anthropic plugin..."
(cd plugins/llm-anthropic && go build -o llm-anthropic.exe .)
pack plugins/llm-anthropic/llm-anthropic.exe

# 构建 llm-ollama 插件（独立 module，基于 dsc-sdk）
echo "Building llm-ollama plugin..."
(cd plugins/llm-ollama && go build -o llm-ollama.exe .)
pack plugins/llm-ollama/llm-ollama.exe

# 构建 tool-filesystem 插件（独立 module，基于 dsc-sdk）
echo "Building tool-filesystem plugin..."
(cd plugins/tool-filesystem && go build -o tool-filesystem.exe .)
pack plugins/tool-filesystem/tool-filesystem.exe

# 构建 tool-str-replace-editor 插件（独立 module，基于 dsc-sdk）
echo "Building tool-str-replace-editor plugin..."
(cd plugins/tool-str-replace-editor && go build -o tool-str-replace-editor.exe .)
pack plugins/tool-str-replace-editor/tool-str-replace-editor.exe

# 构建 tool-browser-use 插件（独立 module，基于 dsc-sdk）
echo "Building tool-browser-use plugin..."
(cd plugins/tool-browser-use && go build -o tool-browser-use.exe .)
pack plugins/tool-browser-use/tool-browser-use.exe

# 构建 tool-lisp-eval 插件（独立 module，基于 dsc-sdk）
echo "Building tool-lisp-eval plugin..."
(cd plugins/tool-lisp-eval && go build -o tool-lisp-eval.exe .)
pack plugins/tool-lisp-eval/tool-lisp-eval.exe

# 构建 tool-skill 插件（独立 module，基于 dsc-sdk）
echo "Building tool-skill plugin..."
(cd plugins/tool-skill && go build -o tool-skill.exe .)
pack plugins/tool-skill/tool-skill.exe

# 构建 tool-memory-service 插件（独立 module，基于 dsc-sdk）
echo "Building tool-memory-service plugin..."
(cd plugins/tool-memory-service && go build -o tool-memory-service.exe .)
pack plugins/tool-memory-service/tool-memory-service.exe

# 构建 tool-notify 插件（独立 module，基于 dsc-sdk）
echo "Building tool-notify plugin..."
(cd plugins/tool-notify && go build -o tool-notify.exe .)
pack plugins/tool-notify/tool-notify.exe

# 构建 tool-lua-host 插件
echo "Building tool-lua-host plugin..."
cd ./plugins/tool-lua-host && go build && cd ../../
LUA_BIN="tool-lua-host"
if [ "$GOOS" = "windows" ]; then LUA_BIN="tool-lua-host.exe"; fi
pack "plugins/tool-lua-host/$LUA_BIN"

# 构建 tool-harness-webui 插件（先 bun 构建前端再编译 Go）
echo "Building tool-harness-webui plugin..."
cd ./plugins/tool-harness-webui && bash ./build.sh && cd ../../
pack plugins/tool-harness-webui/tool-harness-webui.exe

# 构建 policy-fs-observation 插件（独立 module，基于 dsc-sdk）
echo "Building policy-fs-observation plugin..."
(cd plugins/policy-fs-observation && go build -o policy-fs-observation.exe .)
pack plugins/policy-fs-observation/policy-fs-observation.exe

echo "Build completed successfully."