# 开发指南

## 环境准备

### 系统要求

- Linux (Ubuntu 20.04+ / CentOS 8+)
- Go 1.20+
- GCC 9.0+
- Make
- Docker (可选)

### 安装依赖

#### Ubuntu/Debian
```bash
# 基础工具
sudo apt-get update
sudo apt-get install -y build-essential git

# Go
wget https://go.dev/dl/go1.20.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.20.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# C库
sudo apt-get install -y \
    libjansson-dev \
    liburcu-dev \
    libnetfilter-queue-dev \
    iptables-dev

# 可选：protobuf
sudo apt-get install -y protobuf-compiler
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

#### CentOS/RHEL
```bash
# 基础工具
sudo yum groupinstall -y "Development Tools"
sudo yum install -y git

# C库
sudo yum install -y \
    jansson-devel \
    userspace-rcu-devel \
    libnetfilter_queue-devel \
    iptables-devel
```

## 项目结构

```
micro-segment/
├── cmd/                    # 可执行程序入口
│   ├── agent/             # Agent主程序
│   ├── controller/        # Controller主程序
│   └── dp/                # DP主程序
├── internal/              # 内部实现
│   ├── agent/             # Agent实现
│   ├── controller/        # Controller实现
│   └── dp/                # DP实现（C）
├── pkg/                   # 共享库
│   ├── types/             # 公共类型
│   ├── utils/             # 工具函数
│   └── proto/             # gRPC协议
└── web/                   # Web前端
```

## 构建

### 快速构建

```bash
# 构建所有组件
make all

# 或使用脚本
./scripts/build.sh
```

### 分别构建

```bash
# 构建DP
make dp

# 构建Agent
make agent

# 构建Controller
make controller

# 构建Web
make web
```

### 生成protobuf

```bash
make proto
```

## 运行

### 本地开发

#### 1. 启动etcd
```bash
docker run -d \
    --name etcd \
    -p 2379:2379 \
    quay.io/coreos/etcd:v3.5.11 \
    etcd \
    --advertise-client-urls http://0.0.0.0:2379 \
    --listen-client-urls http://0.0.0.0:2379
```

#### 2. 启动Controller
```bash
./bin/controller -c configs/controller.yaml
```

#### 3. 启动Agent
```bash
sudo ./bin/agent -c configs/agent.yaml
```

#### 4. 启动DP
```bash
sudo ./bin/dp -c configs/dp.conf
```

### Docker Compose

```bash
docker-compose up -d
```

## 测试

### 运行所有测试
```bash
make test
```

### 运行单元测试
```bash
make test-unit
```

### 运行集成测试
```bash
make test-integration
```

### 测试覆盖率
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 代码规范

### Go代码

```bash
# 格式化
go fmt ./...

# Lint检查
golangci-lint run ./...

# 静态分析
go vet ./...
```

### C代码

```bash
# 格式化
find internal/dp -name "*.c" -o -name "*.h" | xargs clang-format -i

# 静态分析
cppcheck internal/dp/
```

## 调试

### Agent调试

```bash
# 启用debug日志
./bin/agent -c configs/agent.yaml --log-level debug

# 使用delve调试
dlv exec ./bin/agent -- -c configs/agent.yaml
```

### DP调试

```bash
# 启用debug日志
./bin/dp -c configs/dp.conf -d

# 使用gdb调试
gdb --args ./bin/dp -c configs/dp.conf
```

### 查看日志

```bash
# Agent日志
tail -f /var/log/micro-segment/agent.log

# Controller日志
tail -f /var/log/micro-segment/controller.log

# DP日志
tail -f /var/log/micro-segment/dp.log
```

## 开发工作流

### 1. 创建功能分支

```bash
git checkout -b feature/your-feature
```

### 2. 开发和测试

```bash
# 修改代码
vim internal/agent/engine/engine.go

# 运行测试
go test ./internal/agent/engine/

# 构建
make agent
```

### 3. 提交代码

```bash
git add .
git commit -m "feat: add new feature"
git push origin feature/your-feature
```

### 4. 创建Pull Request

## 常见问题

### 1. DP编译失败

**问题**：找不到libjansson
```
fatal error: jansson.h: No such file or directory
```

**解决**：
```bash
sudo apt-get install libjansson-dev
```

### 2. Agent无法连接Controller

**问题**：gRPC连接失败
```
Failed to connect to controller: connection refused
```

**解决**：
- 检查Controller是否启动
- 检查防火墙规则
- 验证配置文件中的endpoint

### 3. DP无法拦截流量

**问题**：没有权限
```
Failed to create nfq queue: Operation not permitted
```

**解决**：
```bash
# 使用root权限运行
sudo ./bin/dp -c configs/dp.conf

# 或添加capabilities
sudo setcap cap_net_admin,cap_net_raw+ep ./bin/dp
```

## 性能分析

### Go性能分析

```bash
# CPU profiling
go test -cpuprofile=cpu.prof ./...
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof ./...
go tool pprof mem.prof
```

### C性能分析

```bash
# 使用perf
perf record -g ./bin/dp -c configs/dp.conf
perf report

# 使用valgrind
valgrind --tool=callgrind ./bin/dp -c configs/dp.conf
```

## 贡献指南

1. Fork项目
2. 创建功能分支
3. 提交代码
4. 运行测试
5. 创建Pull Request

### Commit规范

```
feat: 新功能
fix: 修复bug
docs: 文档更新
style: 代码格式
refactor: 重构
test: 测试
chore: 构建/工具
```

## 资源

- [Go文档](https://go.dev/doc/)
- [gRPC文档](https://grpc.io/docs/)
- [etcd文档](https://etcd.io/docs/)
- [Netfilter文档](https://www.netfilter.org/documentation/)
