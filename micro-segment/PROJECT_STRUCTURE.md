# 微隔离项目目录结构设计

## 项目概述

基于NeuVector动态微隔离功能提取的独立微隔离产品，采用三层架构：
- **DP层**（数据平面）：C语言实现，负责策略匹配和流量控制
- **Agent层**：Go语言实现，负责容器监控和策略转换
- **Controller层**：Go语言实现，负责策略管理和Web API

---

## 目录结构

```
micro-segment/
├── README.md                    # 项目说明
├── LICENSE                      # 开源协议
├── Makefile                     # 构建脚本
├── go.mod                       # Go模块定义
├── go.sum                       # Go依赖锁定
├── docker-compose.yml           # 容器编排
│
├── cmd/                         # 可执行程序入口
│   ├── agent/                   # Agent程序
│   │   └── main.go
│   ├── controller/              # Controller程序
│   │   └── main.go
│   └── dp/                      # DP程序（C）
│       └── main.c
│
├── pkg/                         # 共享库代码
│   ├── types/                   # 公共数据类型
│   │   ├── policy.go            # 策略相关类型
│   │   ├── connection.go        # 连接相关类型
│   │   └── group.go             # 组相关类型
│   ├── utils/                   # 工具函数
│   │   ├── network.go           # 网络工具
│   │   ├── container.go         # 容器工具
│   │   └── logger.go            # 日志工具
│   └── proto/                   # gRPC协议定义
│       ├── policy.proto         # 策略协议
│       ├── connection.proto     # 连接上报协议
│       └── control.proto        # 控制协议
│
├── internal/                    # 内部实现（不对外暴露）
│   ├── agent/                   # Agent内部实现
│   │   ├── engine/              # 容器引擎
│   │   │   ├── engine.go        # 容器生命周期管理
│   │   │   ├── runtime.go       # 运行时接口
│   │   │   └── events.go        # 事件处理
│   │   ├── policy/              # 策略管理
│   │   │   ├── network.go       # 网络策略计算
│   │   │   ├── converter.go     # 策略转换
│   │   │   └── cache.go         # 策略缓存
│   │   ├── dp/                  # DP通信
│   │   │   ├── client.go        # DP客户端
│   │   │   ├── protocol.go      # 通信协议
│   │   │   └── types.go         # DP数据类型
│   │   ├── probe/               # 容器监控
│   │   │   ├── probe.go         # 监控接口
│   │   │   ├── docker.go        # Docker监控
│   │   │   ├── containerd.go    # Containerd监控
│   │   │   └── network.go       # 网络监控
│   │   ├── connection/          # 连接管理
│   │   │   ├── aggregator.go    # 连接聚合
│   │   │   ├── reporter.go      # 连接上报
│   │   │   └── cache.go         # 连接缓存
│   │   └── config/              # 配置管理
│   │       └── config.go
│   │
│   ├── controller/              # Controller内部实现
│   │   ├── api/                 # REST API
│   │   │   ├── server.go        # HTTP服务器
│   │   │   ├── policy.go        # 策略API
│   │   │   ├── group.go         # 组API
│   │   │   ├── connection.go    # 连接API
│   │   │   └── graph.go         # 拓扑图API
│   │   ├── cache/               # 缓存管理
│   │   │   ├── policy.go        # 策略缓存
│   │   │   ├── group.go         # 组缓存
│   │   │   ├── graph.go         # 拓扑图缓存
│   │   │   └── workload.go      # 工作负载缓存
│   │   ├── rpc/                 # gRPC服务
│   │   │   ├── server.go        # gRPC服务器
│   │   │   ├── agent.go         # Agent通信
│   │   │   └── connection.go    # 连接接收
│   │   ├── storage/             # 持久化存储
│   │   │   ├── interface.go     # 存储接口
│   │   │   ├── etcd.go          # etcd实现
│   │   │   └── memory.go        # 内存实现（测试用）
│   │   └── config/              # 配置管理
│   │       └── config.go
│   │
│   └── dp/                      # DP内部实现（C）
│       ├── dpi/                 # DPI引擎
│       │   ├── dpi_policy.c     # 策略匹配引擎
│       │   ├── dpi_policy.h
│       │   ├── dpi_session.c    # 会话管理
│       │   ├── dpi_session.h
│       │   ├── dpi_packet.c     # 数据包处理
│       │   └── dpi_packet.h
│       ├── ctrl/                # 控制接口
│       │   ├── ctrl.c           # Unix Socket通信
│       │   └── ctrl.h
│       ├── nfq/                 # Netfilter队列
│       │   ├── nfq.c            # NFQ处理
│       │   └── nfq.h
│       └── utils/               # 工具函数
│           ├── rcu_map.c        # 无锁哈希表
│           ├── rcu_map.h
│           └── helper.c
│
├── web/                         # Web前端
│   ├── public/                  # 静态资源
│   ├── src/                     # 源代码
│   │   ├── components/          # React组件
│   │   │   ├── PolicyList/      # 策略列表
│   │   │   ├── GroupList/       # 组列表
│   │   │   ├── NetworkGraph/    # 网络拓扑图
│   │   │   └── ConnectionList/  # 连接列表
│   │   ├── pages/               # 页面
│   │   │   ├── Dashboard.tsx    # 仪表盘
│   │   │   ├── Policy.tsx       # 策略管理
│   │   │   ├── Group.tsx        # 组管理
│   │   │   └── Network.tsx      # 网络可视化
│   │   ├── api/                 # API客户端
│   │   │   └── client.ts
│   │   └── App.tsx              # 应用入口
│   ├── package.json
│   └── tsconfig.json
│
├── configs/                     # 配置文件
│   ├── agent.yaml               # Agent配置示例
│   ├── controller.yaml          # Controller配置示例
│   └── dp.conf                  # DP配置示例
│
├── deployments/                 # 部署文件
│   ├── kubernetes/              # K8s部署
│   │   ├── agent-daemonset.yaml
│   │   ├── controller-deployment.yaml
│   │   └── service.yaml
│   └── docker/                  # Docker部署
│       └── Dockerfile.agent
│       └── Dockerfile.controller
│       └── Dockerfile.dp
│
├── scripts/                     # 脚本工具
│   ├── build.sh                 # 构建脚本
│   ├── test.sh                  # 测试脚本
│   └── deploy.sh                # 部署脚本
│
├── docs/                        # 文档
│   ├── architecture.md          # 架构设计
│   ├── api.md                   # API文档
│   ├── deployment.md            # 部署指南
│   └── development.md           # 开发指南
│
└── tests/                       # 测试
    ├── unit/                    # 单元测试
    ├── integration/             # 集成测试
    └── e2e/                     # 端到端测试
```

---

## 核心模块说明

### 1. Agent模块

**职责**：
- 监控容器生命周期事件
- 接收Controller下发的策略
- 转换策略并推送到DP
- 聚合连接数据并上报

**关键文件**：
- `internal/agent/engine/engine.go` - 容器引擎核心
- `internal/agent/policy/network.go` - 网络策略计算
- `internal/agent/connection/aggregator.go` - 连接聚合（5秒定时器）
- `internal/agent/dp/client.go` - DP通信客户端

---

### 2. Controller模块

**职责**：
- 提供REST API和Web UI
- 管理策略和组配置
- 接收Agent上报的连接数据
- 维护网络拓扑图（wlGraph）

**关键文件**：
- `internal/controller/api/policy.go` - 策略管理API
- `internal/controller/cache/graph.go` - 网络拓扑图
- `internal/controller/rpc/connection.go` - 连接接收
- `internal/controller/storage/etcd.go` - 持久化存储

---

### 3. DP模块

**职责**：
- 实时策略匹配
- DPI应用层检测
- 网络流量拦截
- 违规日志记录

**关键文件**：
- `internal/dp/dpi/dpi_policy.c` - 策略匹配引擎
- `internal/dp/dpi/dpi_session.c` - 会话管理
- `internal/dp/ctrl/ctrl.c` - Unix Socket通信
- `internal/dp/utils/rcu_map.c` - 无锁哈希表

---

## 数据流

```
┌─────────────────────────────────────────────────────────────┐
│                    Web UI (React)                            │
│                    /web/src/                                 │
└────────────────────┬────────────────────────────────────────┘
                     │ REST API
                     ↓
┌─────────────────────────────────────────────────────────────┐
│                    Controller                                │
│  /internal/controller/                                       │
│  - REST API (api/)                                           │
│  - 策略管理 (cache/policy.go)                                │
│  - 拓扑图 (cache/graph.go)                                   │
│  - 存储 (storage/)                                           │
└────────────────────┬────────────────────────────────────────┘
                     │ gRPC
                     ↓
┌─────────────────────────────────────────────────────────────┐
│                    Agent (DaemonSet)                         │
│  /internal/agent/                                            │
│  - 容器监控 (engine/)                                        │
│  - 策略转换 (policy/)                                        │
│  - 连接聚合 (connection/)                                    │
└────────────────────┬────────────────────────────────────────┘
                     │ Unix Socket (JSON)
                     ↓
┌─────────────────────────────────────────────────────────────┐
│                    DP (数据平面)                             │
│  /internal/dp/                                               │
│  - 策略匹配 (dpi/dpi_policy.c)                               │
│  - DPI检测 (dpi/dpi_session.c)                               │
│  - 流量拦截 (nfq/)                                           │
└─────────────────────────────────────────────────────────────┘
```

---

## 构建流程

### 1. DP层（C）
```bash
cd internal/dp
make
# 生成: bin/dp
```

### 2. Agent层（Go）
```bash
cd cmd/agent
go build -o ../../bin/agent
```

### 3. Controller层（Go）
```bash
cd cmd/controller
go build -o ../../bin/controller
```

### 4. Web前端
```bash
cd web
npm install
npm run build
# 生成: web/dist/
```

---

## 配置文件示例

### Agent配置 (configs/agent.yaml)
```yaml
agent:
  id: agent-001
  controller_endpoint: controller:8443
  dp_socket: /tmp/dp_listen.sock
  
connection:
  map_size: 131072        # connectionMap大小
  report_interval: 5      # 上报间隔（秒）
  batch_size: 8192        # 批量上报大小

logging:
  level: info
  file: /var/log/agent.log
```

### Controller配置 (configs/controller.yaml)
```yaml
controller:
  listen_addr: :8443
  grpc_addr: :8444
  
storage:
  type: etcd
  endpoints:
    - etcd:2379
  
graph:
  update_interval: 5      # 图更新间隔（秒）
  max_nodes: 10000        # 最大节点数

logging:
  level: info
  file: /var/log/controller.log
```

---

## 部署方式

### Kubernetes部署
```bash
kubectl apply -f deployments/kubernetes/
```

### Docker Compose部署
```bash
docker-compose up -d
```

---

## 开发指南

### 1. 环境准备
```bash
# Go环境
go version  # >= 1.20

# C编译环境
gcc --version
make --version

# 依赖库
apt-get install libjansson-dev liburcu-dev libnetfilter-queue-dev
```

### 2. 本地开发
```bash
# 启动Controller
./bin/controller -c configs/controller.yaml

# 启动Agent
./bin/agent -c configs/agent.yaml

# 启动DP
./bin/dp -c configs/dp.conf
```

### 3. 运行测试
```bash
make test
```

---

## 与NeuVector的差异

| 功能 | NeuVector | 微隔离MVP |
|------|-----------|-----------|
| 策略模式 | Discover/Monitor/Protect | Monitor/Protect |
| 学习功能 | ✅ | ❌ |
| 服务网格 | ✅ | ❌ |
| FQDN规则 | ✅ | ❌ |
| 文件监控 | ✅ | ❌ |
| 进程监控 | ✅ | ❌ |
| 漏洞扫描 | ✅ | ❌ |
| DLP/WAF | ✅ | ❌ |
| 连接聚合 | 5秒 | 5秒 |
| connectionMap | 32K | 131K |

---

## 下一步

1. ✅ 创建目录结构
2. ⬜ 从NeuVector提取核心代码
3. ⬜ 按清单删除/简化代码
4. ⬜ 实现基础配置管理
5. ⬜ 编译验证
6. ⬜ 功能测试
7. ⬜ 编写文档
8. ⬜ 容器化部署
