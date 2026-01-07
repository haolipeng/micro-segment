# 微隔离 MVP

从NeuVector提取的轻量级容器微隔离解决方案。

## 功能特性

- **网络策略管理**: 基于IP/端口/协议的策略匹配
- **两种策略模式**: Monitor（监控）和 Protect（防护）
- **连接聚合**: 5秒聚合间隔，支持131K并发连接
- **DPI检测**: 深度包检测，识别应用层协议
- **东西向流量监控**: 容器间流量可视化
- **网络拓扑图**: 实时网络拓扑展示

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                      Controller                              │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │ REST API│  │  gRPC   │  │  Cache  │  │ Policy  │        │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │
│       └────────────┴────────────┴────────────┘              │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ gRPC
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Agent                                 │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │ Engine  │  │Aggregator│ │ Policy  │  │DP Client│        │
│  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘        │
│       └────────────┴────────────┴────────────┘              │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ Unix Socket
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         DP                                   │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │
│  │ Policy  │  │   DPI   │  │ Session │  │   NFQ   │        │
│  │ Engine  │  │ Detect  │  │ Manager │  │ Intercept│       │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘        │
└─────────────────────────────────────────────────────────────┘
```

## 快速开始

### 编译

```bash
cd micro-segment

# 编译所有组件
go build ./...

# 编译Agent
go build -o bin/agent ./cmd/agent/

# 编译Controller
go build -o bin/controller ./cmd/controller/
```

### 运行

```bash
# 启动Controller
./bin/controller --http-port 10443 --grpc-port 18400

# 启动Agent
./bin/agent --dp-socket /var/run/dp.sock --grpc-addr localhost:18400

# 启动Web前端（开发模式）
cd web
npm install
npm run dev
# 访问 http://localhost:3000
```

## Web界面

Web前端提供以下功能：

- **仪表盘**: 系统概览，显示工作负载、组、策略等统计
- **网络拓扑**: D3.js力导向图，可视化容器间网络连接
- **策略管理**: 创建、编辑、删除网络策略规则
- **组管理**: 管理容器组，设置策略模式
- **工作负载**: 查看所有容器工作负载详情
- **连接监控**: 实时监控网络连接，过滤违规/拒绝连接

## API

### REST API端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/v1/workloads` | GET | 列出工作负载 |
| `/api/v1/groups` | GET | 列出组 |
| `/api/v1/group` | GET/POST/DELETE | 组CRUD |
| `/api/v1/policies` | GET | 列出策略 |
| `/api/v1/policy` | GET/POST/PUT/DELETE | 策略CRUD |
| `/api/v1/graph` | GET | 获取网络拓扑图 |
| `/api/v1/stats` | GET | 获取统计信息 |
| `/health` | GET | 健康检查 |

### 示例

```bash
# 创建组
curl -X POST http://localhost:10443/api/v1/group \
  -H "Content-Type: application/json" \
  -d '{"name": "web-servers", "policy_mode": "Monitor"}'

# 创建策略
curl -X POST http://localhost:10443/api/v1/policy \
  -H "Content-Type: application/json" \
  -d '{"id": 1001, "from": "web-servers", "to": "db-servers", "ports": "tcp/3306", "action": "allow"}'

# 获取网络拓扑
curl http://localhost:10443/api/v1/graph
```

## 与NeuVector的区别

### 保留的功能
- ✅ 网络策略管理
- ✅ Monitor和Protect模式
- ✅ 连接聚合（5秒）
- ✅ DPI检测
- ✅ 东西向流量监控
- ✅ 网络拓扑图

### 移除的功能
- ❌ Discover学习模式
- ❌ 服务网格支持（Istio/Linkerd）
- ❌ FQDN规则
- ❌ 文件监控
- ❌ 进程监控
- ❌ 漏洞扫描
- ❌ DLP/WAF

## 项目结构

```
micro-segment/
├── cmd/
│   ├── agent/          # Agent入口
│   └── controller/     # Controller入口
├── internal/
│   ├── agent/          # Agent层代码
│   ├── controller/     # Controller层代码
│   └── dp/             # DP层C代码
├── configs/            # 配置文件
├── docs/               # 文档
└── go.mod              # Go模块
```

## 许可证

Apache License 2.0
