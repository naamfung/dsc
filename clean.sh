#!/bin/bash
set -e

# ============================================================
# clean.sh - 开发用清理脚本
# 删除主程序二进制与 plugins/ 目录下所有构建产物（插件可执行文件）。
# 仅删除编译产物，绝不改动任何源码（*.go / 配置 / 文档）。
#
# 与 build.sh 对应：build.sh 构建出什么，这里就清掉什么。
# - 主程序：dsc（Unix）/ dsc.exe（Windows）
# - 插件产物：plugins/<插件>/ 下的 .exe（末位可执行）以及
#   tool-lua-host 在 Unix 下无扩展名的二进制 tool-lua-host。
# ============================================================

# 删除主程序（覆盖 dsc 与 dsc.exe）
if [ -f "dsc" ]; then
    echo "removing: dsc"
    rm -f "dsc"
fi
if [ -f "dsc.exe" ]; then
    echo "removing: dsc.exe"
    rm -f "dsc.exe"
fi

# 递归删除 plugins/ 下所有 .exe 构建产物（排除 node_modules 依赖二进制，
# 那些经 bun install 管理、不属于本仓库构建产物）
if [ -d "plugins" ]; then
    # shellcheck disable=SC2038
    find plugins -type f -name "*.exe" -not -path "*/node_modules/*" -print0 | while IFS= read -r -d '' f; do
        echo "removing: $f"
        rm -f "$f"
    done
fi

# tool-lua-host 在 Unix 构建时无扩展名（见 build.sh：LUA_BIN=tool-lua-host）；
# 确保其可执行二进制被清理（排除同名目录，仅删文件）
if [ -f "plugins/tool-lua-host/tool-lua-host" ]; then
    echo "removing: plugins/tool-lua-host/tool-lua-host"
    rm -f "plugins/tool-lua-host/tool-lua-host"
fi

echo "Clean completed."