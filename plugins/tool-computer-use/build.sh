#!/bin/bash
#
# build.sh — tool-computer-use 一键构建脚本（CGO + robotgo + X11）
#
# 用法：
#   ./build.sh                    # 默认 gcc 构建
#   CC="zig cc" ./build.sh        # 用 zig cc 替代 gcc（需 zig 在 PATH）
#   OUT=/path/to/binary ./build.sh # 指定输出路径
#
# robotgo 依赖 X11 开发头文件（libX11-dev / libxtst-dev / libxi-dev / libxext-dev）。
# 若系统已安装（dpkg -l 可查），直接构建。
# 若未安装且无 root 权限，脚本自动下载 .deb 包解压到临时目录，
# 经 CGO_CPPFLAGS/CGO_LDFLAGS 注入路径构建。
#
# 运行时依赖（目标机器需安装 runtime 包，非 dev 包）：
#   libx11-6 / libxtst6 / libxi6 / libxext6
#   （大多数桌面 Linux 发行版预装）

set -e

# 输出路径（默认当前目录下的 tool-computer-use）
OUT="${OUT:-tool-computer-use}"
SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

# 检测工具是否可用
has_tool() {
    command -v "$1" >/dev/null 2>&1
}

echo "=== tool-computer-use build ==="
echo "CC=${CC:-gcc}"
echo "OUT=${OUT}"
echo ""

# 确保 go 可用
if ! has_tool go; then
    echo "ERROR: go not found in PATH"
    exit 1
fi

# 检测 X11 开发头文件是否已安装
NEED_HEADERS=false
X11_INC=""
X11_LIB=""

if [ ! -f /usr/include/X11/extensions/XTest.h ]; then
    echo "X11/extensions/XTest.h not found — need libxtst-dev"
    NEED_HEADERS=true
fi
if [ ! -f /usr/include/X11/extensions/XInput2.h ]; then
    echo "X11/extensions/XInput2.h not found — need libxi-dev"
    NEED_HEADERS=true
fi

if [ "$NEED_HEADERS" = true ]; then
    echo ""
    echo "X11 dev headers missing. Attempting to download .deb packages (no root needed)..."
    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    # 下载并解压缺失的 dev 包
    for pkg in libxtst-dev libxi-dev libxext-dev; do
        echo "  downloading $pkg..."
        cd "$TMPDIR"
        if apt-get download "$pkg" 2>/dev/null; then
            dpkg-deb -x "$pkg"*.deb "$TMPDIR/extract" 2>/dev/null || true
        else
            echo "  WARNING: cannot download $pkg — build may fail"
        fi
    done

    # 设置编译路径
    if [ -d "$TMPDIR/extract/usr/include" ]; then
        X11_INC="-I$TMPDIR/extract/usr/include"
        export CGO_CPPFLAGS="$X11_INC"
    fi
    if [ -d "$TMPDIR/extract/usr/lib/x86_64-linux-gnu" ]; then
        X11_LIB="-L$TMPDIR/extract/usr/lib/x86_64-linux-gnu -lXext"
        export CGO_LDFLAGS="$X11_LIB"
    fi
    echo "  CGO_CPPFLAGS=$X11_INC"
    echo "  CGO_LDFLAGS=$X11_LIB"
    cd "$SCRIPT_DIR"
fi

# 构建
echo ""
echo "Building..."
go build -ldflags="-s -w" -o "$OUT" .

echo ""
echo "Done: $(file "$OUT" | cut -d, -f1)"
echo "Size: $(ls -lh "$OUT" | awk '{print $5}')"

# 显示运行时依赖
if has_tool ldd; then
    echo ""
    echo "Runtime dependencies:"
    ldd "$OUT" | grep -v "linux-vdso\|ld-linux" | awk '{print "  "$1}'
fi
