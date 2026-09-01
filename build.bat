@echo off
setlocal

:: ============================================================
:: build.bat - Windows 构建脚本
:: 注意：此脚本与 build.sh 须要同步更新
:: ============================================================

:: 设置 GOPATH 和 PATH 以包含 protoc 生成插件
for /f "tokens=*" %%i in ('go env GOPATH') do set GOPATH=%%i
set PATH=%PATH%;%GOPATH%\bin

:: 编译 proto 文件：仅当检测到 protoc 编译器时才重新生成；未检测到则跳过。
:: 理由：proto 代码（*.pb.go）已纳入仓库，且 protoc 编译器二进制不宜入库，
:: 用户没有编译器时不应报错，直接复用仓库内已生成的代码即可。
echo Generating proto Go code...
set "PROTOC_BIN="
if exist protoc_dir\bin\protoc.exe (
    set "PROTOC_BIN=protoc_dir\bin\protoc.exe"
) else (
    where protoc >nul 2>nul
    if not errorlevel 1 set "PROTOC_BIN=protoc"
)

if defined PROTOC_BIN (
    :: 安装 protoc 生成插件（如果尚未安装）
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
    %PROTOC_BIN% --proto_path=proto --go_out=proto --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative proto/dsc.proto
) else (
    echo   (skipped: 未检测到 protoc 编译器，使用仓库内已生成的 proto 代码^)
)

:: ---- UPX 压缩检测 ----
:: 当系统存在 upx 工具时，用它压缩构建产物以显著减小体积；未检测到则跳过。
set "UPX_BIN="
where upx >nul 2>nul
if not errorlevel 1 set "UPX_BIN=upx"
if defined UPX_BIN (
    echo UPX detected: %UPX_BIN% ^(构建产物将自动压缩^)
) else (
    echo UPX not detected ^(跳过压缩，可使用 --best 手动压缩以减小体积^)
)

:: 构建主程序
echo Building main program...
go build
call :pack "dsc.exe"

:: 构建 agent-react-loop 插件（独立 module，基于 dsc-sdk）
echo Building agent-react-loop plugin...
cd plugins\agent-react-loop
go build -o agent-react-loop.exe .
cd ..\..
call :pack "plugins\agent-react-loop\agent-react-loop.exe"

:: 构建 llm-openai 插件
echo Building llm-openai plugin...
cd plugins\llm-openai
go build -o llm-openai.exe .
cd ..\..
call :pack "plugins\llm-openai\llm-openai.exe"

:: 构建 llm-anthropic 插件（独立 module，基于 dsc-sdk）
echo Building llm-anthropic plugin...
cd plugins\llm-anthropic
go build -o llm-anthropic.exe .
cd ..\..
call :pack "plugins\llm-anthropic\llm-anthropic.exe"

:: 构建 llm-ollama 插件（独立 module，基于 dsc-sdk）
echo Building llm-ollama plugin...
cd plugins\llm-ollama
go build -o llm-ollama.exe .
cd ..\..
call :pack "plugins\llm-ollama\llm-ollama.exe"

:: 构建 tool-filesystem 插件（独立 module，基于 dsc-sdk）
echo Building tool-filesystem plugin...
cd plugins\tool-filesystem
go build -o tool-filesystem.exe .
cd ..\..
call :pack "plugins\tool-filesystem\tool-filesystem.exe"

:: 构建 tool-str-replace-editor 插件（独立 module，基于 dsc-sdk）
echo Building tool-str-replace-editor plugin...
cd plugins\tool-str-replace-editor
go build -o tool-str-replace-editor.exe .
cd ..\..
call :pack "plugins\tool-str-replace-editor\tool-str-replace-editor.exe"

:: 构建 tool-browser-use 插件（独立 module，基于 dsc-sdk）
echo Building tool-browser-use plugin...
cd plugins\tool-browser-use
go build -o tool-browser-use.exe .
cd ..\..
call :pack "plugins\tool-browser-use\tool-browser-use.exe"

:: 构建 tool-lisp-eval 插件（独立 module，基于 dsc-sdk）
echo Building tool-lisp-eval plugin...
cd plugins\tool-lisp-eval
go build -o tool-lisp-eval.exe .
cd ..\..
call :pack "plugins\tool-lisp-eval\tool-lisp-eval.exe"

:: 构建 tool-skill 插件（独立 module，基于 dsc-sdk）
echo Building tool-skill plugin...
cd plugins\tool-skill
go build -o tool-skill.exe .
cd ..\..
call :pack "plugins\tool-skill\tool-skill.exe"

:: 构建 tool-memory-service 插件（独立 module，基于 dsc-sdk）
echo Building tool-memory-service plugin...
cd plugins\tool-memory-service
go build -o tool-memory-service.exe .
cd ..\..
call :pack "plugins\tool-memory-service\tool-memory-service.exe"

:: 构建 dsc-notify 插件（独立 module，基于 dsc-sdk）
echo Building dsc-notify plugin...
cd plugins\dsc-notify
go build -o dsc-notify.exe .
cd ..\..
call :pack "plugins\dsc-notify\dsc-notify.exe"

:: 构建 tool-lua-host 插件
echo Building tool-lua-host plugin...
cd plugins\tool-lua-host
go build
cd ..\..
call :pack "plugins\tool-lua-host\tool-lua-host.exe"

:: 构建 tool-harness-webui 插件（先 bun 构建前端再编译 Go）
echo Building tool-harness-webui plugin...
cd plugins\tool-harness-webui
call build.bat
cd ..\..
call :pack "plugins\tool-harness-webui\tool-harness-webui.exe"

:: 构建 policy-fs-observation 插件（独立 module，基于 dsc-sdk）
echo Building policy-fs-observation plugin...
cd plugins\policy-fs-observation
go build -o policy-fs-observation.exe .
cd ..\..
call :pack "plugins\policy-fs-observation\policy-fs-observation.exe"

:: 构建 tool-ssh 插件（独立 module，基于 dsc-sdk）
echo Building tool-ssh plugin...
cd plugins\tool-ssh
go build -o tool-ssh.exe .
cd ..\..
call :pack "plugins\tool-ssh\tool-ssh.exe"

:: 构建 tool-musicplayer 插件（独立 module，基于 dsc-sdk）
echo Building tool-musicplayer plugin...
cd plugins\tool-musicplayer
go build -o tool-musicplayer.exe .
cd ..\..
call :pack "plugins\tool-musicplayer\tool-musicplayer.exe"

:: 构建 tool-jyutzyun 插件（独立 module，基于 dsc-sdk；本地内部插件，
:: 未随仓库分发的环境可能无此目录，存在才编译、否则跳过）
echo Building tool-jyutzyun plugin...
if exist plugins\tool-jyutzyun (
    cd plugins\tool-jyutzyun
    go build -o tool-jyutzyun.exe .
    cd ..\..
    call :pack "plugins\tool-jyutzyun\tool-jyutzyun.exe"
) else (
    echo   ^(skip: plugins\tool-jyutzyun 目录不存在^)
)

:: 构建 tool-2fa-master 插件（独立 module，基于 dsc-sdk；本地内部插件，
:: 未随仓库分发的环境可能无此目录，存在才编译、否则跳过）
echo Building tool-2fa-master plugin...
if exist plugins\tool-2fa-master (
    cd plugins\tool-2fa-master
    go build -o tool-2fa-master.exe .
    cd ..\..
    call :pack "plugins\tool-2fa-master\tool-2fa-master.exe"
) else (
    echo   ^(skip: plugins\tool-2fa-master 目录不存在^)
)

:: 构建 tool-novelforge 插件（独立 module，基于 dsc-sdk；本地内部插件，
:: 未随仓库分发的环境可能无此目录，存在才编译、否则跳过）
echo Building tool-novelforge plugin...
if exist plugins\tool-novelforge (
    cd plugins\tool-novelforge
    go build -o novelforge.exe .
    cd ..\..
    call :pack "plugins\tool-novelforge\novelforge.exe"
) else (
    echo   ^(skip: plugins\tool-novelforge 目录不存在^)
)

echo Build completed successfully.
endlocal
goto :eof

:: ---- 压缩单个二进制（未检测到 UPX 或文件不存在/压缩失败时均安全跳过）----
:pack
if not defined UPX_BIN exit /b 0
if not exist "%~1" (
    echo   ^(warn: 构建产物未找到，跳过压缩: %~1^)
    exit /b 0
)
echo Compressing %~1 with UPX...
%UPX_BIN% --best "%~1"
exit /b 0
