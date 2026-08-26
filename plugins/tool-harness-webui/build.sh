#!/bin/bash
set -e
cd "$(dirname "$0")"

# ============================================================
# build.sh - Unix/Linux/macOS 构建脚本
# 注意：此脚本与 build.bat 须要同步更新
# ============================================================

# 构建前端：SvelteKit 静态编译输出到 webui/dist，随后 go:embed 嵌入
if [ -d "webui" ] && [ -f "webui/package.json" ]; then
    echo "harness-webui: building frontend (bun)..."
    (cd webui && bun install --frozen-lockfile >/dev/null 2>&1 || bun install >/dev/null)
    (cd webui && bun run build)
fi

# 编译 Go 插件（嵌入前端静态资源）
echo "tool-harness-webui: building go plugin..."
go build -o tool-harness-webui.exe .
echo "tool-harness-webui: built -> tool-harness-webui.exe"