#!/bin/bash

# 微隔离项目构建脚本

set -e

echo "=== 构建微隔离项目 ==="

# 项目根目录
PROJECT_ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$PROJECT_ROOT"

# 创建构建目录
mkdir -p bin

echo "1. 生成gRPC代码..."
cd api/proto
./generate.sh
cd "$PROJECT_ROOT"

echo "2. 构建DP层..."
cd internal/dp
make clean
make
cd "$PROJECT_ROOT"
echo "✓ DP层构建完成: bin/dp"

echo "3. 构建Controller..."
cd cmd/controller
go build -ldflags "-X main.Version=0.1.0" -o ../../bin/controller
cd "$PROJECT_ROOT"
echo "✓ Controller构建完成: bin/controller"

echo "4. 构建Agent..."
cd cmd/agent
go build -ldflags "-X main.Version=0.1.0" -o ../../bin/agent
cd "$PROJECT_ROOT"
echo "✓ Agent构建完成: bin/agent"

echo "5. 构建Web前端..."
cd web
if [ ! -d "node_modules" ]; then
    echo "安装npm依赖..."
    npm install
fi
npm run build
cd "$PROJECT_ROOT"
echo "✓ Web前端构建完成: web/dist/"

echo ""
echo "=== 构建完成 ==="
echo "可执行文件:"
ls -la bin/
echo ""
echo "Web前端:"
ls -la web/dist/ | head -5

echo ""
echo "下一步运行指南:"
echo "1. 启动DP进程: sudo ./bin/dp"
echo "2. 启动Controller: ./bin/controller"
echo "3. 启动Agent: sudo ./bin/agent --enable-capture"
echo "4. 访问Web界面: http://localhost:8080"
echo ""
echo "注意: Agent需要root权限来设置iptables规则和访问Docker"