#!/bin/bash
# 生成 gRPC Go 代码
# 需要安装: protoc, protoc-gen-go, protoc-gen-go-grpc

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 检查依赖
if ! command -v protoc &> /dev/null; then
    echo "Error: protoc not found. Please install protobuf compiler."
    echo "  Ubuntu: apt install protobuf-compiler"
    echo "  macOS: brew install protobuf"
    exit 1
fi

if ! command -v protoc-gen-go &> /dev/null; then
    echo "Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "Installing protoc-gen-go-grpc..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

# 生成代码
echo "Generating Go code from proto files..."
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       microseg.proto

echo "Done! Generated files:"
ls -la *.pb.go 2>/dev/null || echo "  (no files generated yet)"
