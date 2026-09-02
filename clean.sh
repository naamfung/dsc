#!/bin/bash
set -e

# ============================================================
# clean.sh - 开发用清理脚本
# 删除主程序二进制与 plugins/ 目录下所有构建产物（插件可执行文件）。
# 仅删除编译产物，绝不改动任何源码（*.go / 配置 / 文档）。
#
# 与 build.sh 对应：build.sh 构建出什么，这里就清掉什么。
# - 主程序：dsc（Unix）/ dsc.exe（Windows）
# - 插件产物：plugins/<插件>/ 下的 <插件>.exe（Windows）或无扩展名 <插件>（Unix）
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

# 删除 Unix 平台无扩展名的插件二进制（build.sh 按平台出后缀：Windows 为
# <插件名>.exe、Unix 为 <插件名>）。插件产物固定在 plugins/<插件>/<插件><后缀>，
# 按目录基名逐一清理（排除同名目录、仅删文件），覆盖 tool-lua-host 等全部插件。
if [ -d "plugins" ]; then
    for dir in plugins/*/; do
        [ -d "$dir" ] || continue
        name="$(basename "$dir")"
        if [ -f "$dir$name" ]; then
            echo "removing: $dir$name"
            rm -f "$dir$name"
        fi
    done
fi

rm -rf ./config/presets/preset-backups
rm -rf ./config/config-backups
rm -rf ./plugins-backup
rm -rf ./attachments
rm -rf ./sessions
rm -rf ./memory
rm -rf ./temp

echo "Clean completed."