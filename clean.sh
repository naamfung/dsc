#!/bin/bash
set -e

# ============================================================
# clean.sh - 开发用清理脚本
# 删除主程序二进制与 plugins/ 目录下所有构建产物（插件可执行文件）。
# 仅删除编译产物，绝不改动任何源码（*.go / 配置 / 文档）。
#
# 与 build.sh 对应：build.sh 构建出什么，这里就清掉什么。
# - 主程序：dsc（Unix）/ dsc.exe / dsc.exe~（Windows）
# - 插件产物：plugins/<插件>/ 下的 .exe / .exe~（Windows）或无扩展名文件（Unix，如
#   dsc-plugin-*、tool-harness-webui）
# ============================================================

# 删除主程序（覆盖 dsc、dsc.exe 与 dsc.exe~）
for f in "dsc" "dsc.exe" "dsc.exe~"; do
    if [ -f "$f" ]; then
        echo "removing: $f"
        rm -f "$f"
    fi
done

# 递归删除 plugins/ 下所有 .exe / .exe~ 构建产物（排除 node_modules 依赖二进制，
# 那些经 bun install 管理、不属于本仓库构建产物）
if [ -d "plugins" ]; then
    # shellcheck disable=SC2038
    find plugins -type f \( -name "*.exe" -o -name "*.exe~" \) -not -path "*/node_modules/*" -print0 | while IFS= read -r -d '' f; do
        echo "removing: $f"
        rm -f "$f"
    done
fi

# 删除 Unix 平台无扩展名的插件二进制（如 dsc-plugin-*、tool-harness-webui；build.sh
# 按平台出后缀：Windows 为 .exe、Unix 为无扩展名）。一律删除 plugins/ 下无扩展名
# 的文件，并排除 node_modules 依赖内的无点文件（如 LICENSE）。
if [ -d "plugins" ]; then
    # shellcheck disable=SC2038
    find plugins -type f ! -name '*.*' -not -path '*/node_modules/*' -print0 | while IFS= read -r -d '' f; do
        echo "removing: $f"
        rm -f "$f"
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