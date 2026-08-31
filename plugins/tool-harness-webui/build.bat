@echo off
:: ============================================================
:: build.bat - Windows 构建脚本
:: 注意：此脚本与 build.sh 须要同步更新
:: ============================================================

cd "%~dp0"

:: 构建前端：SvelteKit 静态编译输出到 webui/dist，随后 go:embed 嵌入
if exist webui\package.json (
    echo harness-webui: building frontend ^(bun^)...
    cd webui
    bun install --frozen-lockfile >nul 2>&1
    if errorlevel 1 bun install >nul
    :: 生成 .svelte-kit 配置（tsconfig.json extends 依赖；全新克隆时必需）
    bunx svelte-kit sync
    bun run build
    cd ..
)

:: 编译 Go 插件（嵌入前端静态资源）
echo tool-harness-webui: building go plugin...
go build -o tool-harness-webui.exe .
echo tool-harness-webui: built -^> tool-harness-webui.exe
