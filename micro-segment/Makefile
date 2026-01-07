.PHONY: all clean dp agent controller web test

# 版本信息
VERSION ?= 0.1.0
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# 构建目录
BUILD_DIR := bin
DP_DIR := internal/dp
AGENT_DIR := cmd/agent
CONTROLLER_DIR := cmd/controller
WEB_DIR := web

# Go编译参数
GO := go
GOFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# C编译参数
CC := gcc
CFLAGS := -Wall -O2 -g
LDFLAGS := -ljansson -lurcu -lnetfilter_queue -lpthread

all: dp agent controller

# 构建数据平面（C）
dp:
	@echo "Building DP..."
	@mkdir -p $(BUILD_DIR)
	$(MAKE) -C $(DP_DIR)
	@cp $(DP_DIR)/dp $(BUILD_DIR)/
	@echo "DP built successfully: $(BUILD_DIR)/dp"

# 构建Agent（Go）
agent:
	@echo "Building Agent..."
	@mkdir -p $(BUILD_DIR)
	cd $(AGENT_DIR) && $(GO) build $(GOFLAGS) -o ../../$(BUILD_DIR)/agent
	@echo "Agent built successfully: $(BUILD_DIR)/agent"

# 构建Controller（Go）
controller:
	@echo "Building Controller..."
	@mkdir -p $(BUILD_DIR)
	cd $(CONTROLLER_DIR) && $(GO) build $(GOFLAGS) -o ../../$(BUILD_DIR)/controller
	@echo "Controller built successfully: $(BUILD_DIR)/controller"

# 构建Web前端
web:
	@echo "Building Web UI..."
	cd $(WEB_DIR) && npm install && npm run build
	@echo "Web UI built successfully: $(WEB_DIR)/dist/"

# 生成protobuf
proto:
	@echo "Generating protobuf..."
	protoc --go_out=. --go-grpc_out=. pkg/proto/*.proto
	@echo "Protobuf generated successfully"

# 运行测试
test:
	@echo "Running tests..."
	$(GO) test -v ./...
	@echo "Tests completed"

# 运行单元测试
test-unit:
	@echo "Running unit tests..."
	$(GO) test -v ./tests/unit/...

# 运行集成测试
test-integration:
	@echo "Running integration tests..."
	$(GO) test -v ./tests/integration/...

# 代码格式化
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	find $(DP_DIR) -name "*.c" -o -name "*.h" | xargs clang-format -i

# 代码检查
lint:
	@echo "Running linters..."
	golangci-lint run ./...

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	$(MAKE) -C $(DP_DIR) clean
	rm -rf $(WEB_DIR)/dist $(WEB_DIR)/node_modules
	@echo "Clean completed"

# 安装依赖
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	cd $(WEB_DIR) && npm install

# Docker镜像构建
docker-build:
	@echo "Building Docker images..."
	docker build -f deployments/docker/Dockerfile.agent -t micro-segment-agent:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.controller -t micro-segment-controller:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.dp -t micro-segment-dp:$(VERSION) .

# 帮助信息
help:
	@echo "Micro-Segment Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make all              - Build all components"
	@echo "  make dp               - Build DP (data plane)"
	@echo "  make agent            - Build Agent"
	@echo "  make controller       - Build Controller"
	@echo "  make web              - Build Web UI"
	@echo "  make proto            - Generate protobuf"
	@echo "  make test             - Run all tests"
	@echo "  make test-unit        - Run unit tests"
	@echo "  make test-integration - Run integration tests"
	@echo "  make fmt              - Format code"
	@echo "  make lint             - Run linters"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make deps             - Install dependencies"
	@echo "  make docker-build     - Build Docker images"
	@echo ""
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
