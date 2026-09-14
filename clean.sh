#!/bin/sh
# clean.sh - DSC 跨平台开发用清理脚本
#
# 删除主程序二进制与 plugins/ 目录下所有构建产物（插件可执行文件），
# 以及运行时自愈产生的备份/临时目录。仅删除编译产物，绝不改动源码
# （*.go / 配置 / 文档）。
#
# 跨平台兼容：本脚本以 POSIX sh 编写，可在以下环境原生运行：
#   - Linux（sh / dash / bash / zsh / ksh）
#   - macOS（sh / bash / zsh）
#   - FreeBSD / GhostBSD（sh / bash）
#   - Windows 经 Git Bash / WSL / MSYS2（带 find 与 rm 即可）
# 不依赖 bash 专属语法（无 read -d / mapfile / ${arr[@]} 等）。
#
# 清理范围（覆盖对齐 AGENTS.md 七端的所有平台产物）：
#   - 主程序：dsc（Unix 无扩展名）/ dsc.exe（Windows）/ dsc.exe~（Windows 备份）
#   - 插件产物：plugins/<插件>/ 下
#       *.exe、*.exe~（Windows 二进制）
#       无扩展名可执行文件（Unix 二进制，如 dsc-plugin-*、tool-harness-webui、tool-pdf）
#   - 备份/临时目录：config/presets/preset-backups、config/config-backups、
#     plugins-backup、attachments、sessions、memory、temp、dist（builder 产物）
#
# 排除目录：node_modules / .svelte-kit / webui（前端依赖与构建缓存，
#   由 bun 管理，不属于本仓库构建产物）

set -e

# 切换到脚本所在目录（即仓库根），保证从任何工作目录调用都能正确清理
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$script_dir"

remove_if_exists() {
    if [ -e "$1" ]; then
        echo "removing: $1"
        rm -rf "$1"
    fi
}

# 1. 删除主程序（覆盖 Unix 与 Windows 命名）
for f in dsc dsc.exe dsc.exe~; do
    remove_if_exists "$f"
done

# 2. 递归删除 plugins/ 下所有 .exe / .exe~ 构建产物
#    排除 node_modules（前端依赖二进制，由 bun install 管理）与
#    webui（前端目录，含 svelte-kit 构建缓存，非 Go 产物）
if [ -d "plugins" ]; then
    find plugins -type f \( -name '*.exe' -o -name '*.exe~' \) \
        -not -path '*/node_modules/*' \
        -not -path '*/.svelte-kit/*' \
        -not -path '*/webui/*' \
        -print 2>/dev/null | while IFS= read -r f; do
        echo "removing: $f"
        rm -f "$f"
    done
fi

# 3. 删除 Unix 平台无扩展名的插件二进制（如 dsc-plugin-*、tool-harness-webui、tool-pdf）
#    build.sh / builder 按平台出后缀：Windows 为 .exe、Unix 为无扩展名。
#    一律删除 plugins/ 下无扩展名（不含点）的文件，
#    排除 node_modules / .svelte-kit / webui / 任何 .md / .go / .yaml 等带点文件
#    以及 LICENSE / README 等无点文本（保留源码与文档）。
if [ -d "plugins" ]; then
    find plugins -type f ! -name '*.*' \
        -not -path '*/node_modules/*' \
        -not -path '*/.svelte-kit/*' \
        -not -path '*/webui/*' \
        -not -name 'LICENSE' \
        -not -name 'README' \
        -not -name 'README.md' \
        -print 2>/dev/null | while IFS= read -r f; do
        # 进一步过滤：保留无扩展名的源码文件（罕见但防御性）
        case "$f" in
            *.go|*.mod|*.sum|*.yaml|*.yml|*.json|*.md|*.txt|*.toml|*.lock) continue ;;
        esac
        echo "removing: $f"
        rm -f "$f"
    done
fi

# 4. 删除 builder 产生的 dist/ 目录（七端交叉编译发布包输出）
remove_if_exists "dist"

# 5. 删除运行时自愈产生的备份目录与临时目录
remove_if_exists "./config/presets/preset-backups"
remove_if_exists "./config/config-backups"
remove_if_exists "./plugins-backup"
remove_if_exists "./attachments"
remove_if_exists "./sessions"
remove_if_exists "./memory"
remove_if_exists "./temp"

echo "Clean completed."
