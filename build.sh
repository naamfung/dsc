#!/bin/bash
set -e

# ============================================================
# build.sh - 全平台构建脚本（Linux/macOS/FreeBSD + Windows Git Bash/MSYS 等 POSIX shell）
# 可执行文件后缀按 GOOS 决定：Windows 为 .exe，其余平台为空。
# 注意：此脚本与 build.bat 须要同步更新
# ============================================================

# 目标平台 OS 与可执行文件后缀（go env GOOS 反映当前平台）
GOOS="$(go env GOOS)"
BINEXT=""
if [ "$GOOS" = "windows" ]; then BINEXT=".exe"; fi

# 设置 GOPATH 和 PATH 以包含 protoc 生成插件
GOPATH=$(go env GOPATH)
export PATH="$PATH:$GOPATH/bin"

# 编译 proto 文件：仅当检测到 protoc 编译器时才重新生成；未检测到则跳过。
# 理由：proto 代码（*.pb.go）已纳入仓库，且 protoc 编译器二进制不宜入库，
# 用户没有编译器时不应报错，直接复用仓库内已生成的代码即可。
echo "Generating proto Go code..."
PROTOC_BIN=""
if [ -f "protoc_dir/bin/protoc$BINEXT" ]; then
    PROTOC_BIN="protoc_dir/bin/protoc$BINEXT"
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

# build_plugin <name>：构建单个插件为 plugins/<name>/<name><BINEXT> 并 UPX 压缩。
# 目录不存在（本地内部插件未随仓库分发）时跳过。
build_plugin() {
    local name="$1"
    if [ ! -d "plugins/$name" ]; then
        echo "  (skip: plugins/$name 目录不存在)"
        return
    fi
    echo "Building $name plugin..."
    (cd "plugins/$name" && go build -o "$name$BINEXT" .)
    pack "plugins/$name/$name$BINEXT"
}

# 构建主程序
echo "Building main program..."
go build
MAIN_BIN="dsc$BINEXT"
pack "$MAIN_BIN"

# 构建各插件（独立 module，基于 dsc-sdk；本地内部插件缺失时自动跳过）
build_plugin agent-react-loop
build_plugin llm-openai
build_plugin llm-anthropic
build_plugin llm-ollama
build_plugin tool-filesystem
build_plugin tool-str-replace-editor
build_plugin tool-browser-use
build_plugin tool-lisp-eval
build_plugin tool-skill
build_plugin tool-memory-service
build_plugin dsc-notify
build_plugin tool-lua-host
build_plugin tool-agentic-bench

# 构建 tool-harness-webui 插件（先 bun 构建前端再编译 Go；其 build.sh 同样按平台出后缀）
echo "Building tool-harness-webui plugin..."
(cd plugins/tool-harness-webui && bash ./build.sh)
pack "plugins/tool-harness-webui/tool-harness-webui$BINEXT"

build_plugin policy-fs-observation
build_plugin tool-ssh
build_plugin tool-musicplayer
build_plugin tool-jyutzyun
build_plugin tool-2fa-master
build_plugin tool-novelforge

echo "Build completed successfully."
