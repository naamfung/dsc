@echo off
setlocal

:: ============================================================
:: build.bat - Windows 构建脚本
:: 注意：此脚本与 build.sh 须要同步更新
:: ============================================================

:: 设置 GOPATH 和 PATH 以包含 protoc 生成插件
for /f "tokens=*" %%i in ('go env GOPATH') do set GOPATH=%%i
set PATH=%PATH%;%GOPATH%\bin

:: 安装 protoc 生成插件（如果尚未安装）
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

:: 编译 proto 文件
echo Generating proto Go code...
if exist protoc_dir\bin\protoc.exe (
    protoc_dir\bin\protoc.exe --proto_path=proto --go_out=proto --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative proto/dsc.proto
) else (
    protoc --proto_path=proto --go_out=proto --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative proto/dsc.proto
)

:: 构建主程序
echo Building main program...
go build

:: 构建 agent-react-loop 插件
echo Building agent-react-loop plugin...
go build -o plugins\agent-react-loop\agent-react-loop.exe .\plugins\agent-react-loop

:: 构建 llm-openai 插件
echo Building llm-openai plugin...
cd plugins\llm-openai
go build -o llm-openai.exe .
cd ..\..

:: 构建 llm-anthropic 插件（独立 module，基于 dsc-sdk）
echo Building llm-anthropic plugin...
cd plugins\llm-anthropic
go build -o llm-anthropic.exe .
cd ..\..

:: 构建 llm-ollama 插件（独立 module，基于 dsc-sdk）
echo Building llm-ollama plugin...
cd plugins\llm-ollama
go build -o llm-ollama.exe .
cd ..\..

:: 构建 tool-filesystem 插件（独立 module，基于 dsc-sdk）
echo Building tool-filesystem plugin...
cd plugins\tool-filesystem
go build -o tool-filesystem.exe .
cd ..\..

:: 构建 tool-str-replace-editor 插件（独立 module，基于 dsc-sdk）
echo Building tool-str-replace-editor plugin...
cd plugins\tool-str-replace-editor
go build -o tool-str-replace-editor.exe .
cd ..\..

:: 构建 tool-browser-use 插件（独立 module，基于 dsc-sdk）
echo Building tool-browser-use plugin...
cd plugins\tool-browser-use
go build -o tool-browser-use.exe .
cd ..\..

:: 构建 tool-lisp-eval 插件（独立 module，基于 dsc-sdk）
echo Building tool-lisp-eval plugin...
cd plugins\tool-lisp-eval
go build -o tool-lisp-eval.exe .
cd ..\..

:: 构建 tool-skill 插件（独立 module，基于 dsc-sdk）
echo Building tool-skill plugin...
cd plugins\tool-skill
go build -o tool-skill.exe .
cd ..\..

:: 构建 tool-lua-host 插件
echo Building tool-lua-host plugin...
cd plugins\tool-lua-host
go build
cd ..\..

:: 构建 tool-harness-webui 插件（先 bun 构建前端再编译 Go）
echo Building tool-harness-webui plugin...
cd plugins\tool-harness-webui
call build.bat
cd ..\..

:: 构建 policy-fs-observation 插件
echo Building policy-fs-observation plugin...
go build -o plugins\policy-fs-observation\policy-fs-observation.exe .\plugins\policy-fs-observation

echo Build completed successfully.
endlocal
