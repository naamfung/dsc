#!/bin/bash

# 设置 GOPATH 和 PATH 以包含 protoc 生成插件
GOPATH=$(go env GOPATH)
export PATH="$PATH:$GOPATH/bin"

# 安装 protoc 生成插件（如果尚未安装）
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 编译 proto 文件
echo "Generating proto Go code..."
if [ -f "protoc_dir/bin/protoc.exe" ]; then
    protoc_dir/bin/protoc.exe --proto_path=proto --go_out=proto --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative proto/dsc.proto
else
    protoc --proto_path=proto --go_out=proto --go_opt=paths=source_relative --go-grpc_out=proto --go-grpc_opt=paths=source_relative proto/dsc.proto
fi

# 构建主程序
echo "Building main program..."
go build

# 构建 example 插件
echo "Building example plugin..."
go build -o plugins/example/example.exe ./plugins/example

# 构建 react_loop 插件
echo "Building react_loop plugin..."
go build -o plugins/react_loop/react_loop.exe ./plugins/react_loop

# 构建 llm-openai 插件
echo "Building llm-openai plugin..."
go build -o plugins/llm-openai/llm-openai.exe ./plugins/llm-openai

# 构建 llm-anthropic 插件
echo "Building llm-anthropic plugin..."
go build -o plugins/llm-anthropic/llm-anthropic.exe ./plugins/llm-anthropic

# 构建 llm-ollama 插件
echo "Building llm-ollama plugin..."
go build -o plugins/llm-ollama/llm-ollama.exe ./plugins/llm-ollama

echo "Build completed successfully."

./concat_safe.sh . > v9_src.txt