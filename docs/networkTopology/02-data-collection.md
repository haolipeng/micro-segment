# 连接数据收集机制

## 一、概述

连接数据收集是网络拓扑的基础，负责从数据平面获取网络连接信息并上报给 Controller。

## 二、数据收集架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      连接数据收集架构                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    数据平面 (DP)                         │   │
│  │  - 会话跟踪 (dpi_session)                                │   │
│  │  - 连接统计 (bytes, sessions)                           │   │
│  │  - 协议识别 (application)                                │   │
│  └────────────────────────┬────────────────────────────────┘   │
│                           │ dp.Connection                       │
│                           ▼                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Agent 连接收集                         │   │
│  │                                                          │   │
│  │  ┌──────────────────┐    ┌──────────────────┐           │   │
│  │  │   connectionMap   │    │  putConnections  │           │   │
│  │  │  (连接缓存)       │───▶│  (定期上报)      │           │   │
│  │  └──────────────────┘    └──────────────────┘           │   │
│  │                                   │                      │   │
│  │                                   ▼                      │   │
│  │                          ┌──────────────────┐           │   │
│  │                          │   conn2CLUS      │           │   │
│  │                          │  (格式转换)      │           │   │
│  │                          └──────────────────┘           │   │
│  │                                   │                      │   │
│  │                                   ▼                      │   │
│  │                          ┌──────────────────┐           │   │
│  │                          │ sendConnections  │           │   │
│  │                          │  (gRPC 发送)     │           │   │
│  │                          └──────────────────┘           │   │
│  └────────────────────────────────────┬────────────────────┘   │
│                                       │                         │
│                                       ▼ gRPC                    │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    Controller                            │   │
│  │                 UpdateConnections()                      │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 三、Agent 端连接收集

### 3.1 连接缓存

**源码位置**: `agent/timer.go`

```go
// 连接缓存映射
var connectionMap map[string]*dp.Connection
var connectionMutex sync.Mutex

// 连接上报配置
const connectionMapMax = 8192     // 最大缓存连接数
const connectionListMax = 512     // 单次上报最大连接数
var connectReportInterval = 5     // 上报间隔 (秒)
```

### 3.2 连接收集定时器

**源码位置**: `agent/timer.go:620-699`

```go
func putConnections() {
    var list []*dp.Connection
    var keys []string

    // 1. 检查是否到达上报时间
    if reportTick < nextConnectReportTick {
        return
    }

    // 2. 从缓存中取出连接
    connectionMutex.Lock()
    for key, conn := range connectionMap {
        list = append(list, conn)
        keys = append(keys, key)
        delete(connectionMap, key)

        // 限制单次上报数量
        if len(list) == connectionListMax {
            break
        }
    }
    connectionMutex.Unlock()

    // 3. 转换并发送
    if len(list) > 0 {
        conns := make([]*share.CLUSConnection, len(list))
        for i, c := range list {
            conns[i] = conn2CLUS(c)
        }

        resp, err := sendConnections(conns)
        if err != nil {
            // 发送失败，将连接放回缓存
            connectionMutex.Lock()
            for i, conn := range list {
                connectionMap[keys[i]] = conn
            }
            connectionMutex.Unlock()
        }

        // 处理 Controller 返回的上报间隔调整
        if resp != nil && resp.ReportInterval != 0 {
            connectReportInterval = resp.ReportInterval
        }
    }

    nextConnectReportTick += connectReportInterval
}
```

### 3.3 连接格式转换

**源码位置**: `agent/timer.go`

```go
func conn2CLUS(c *dp.Connection) *share.CLUSConnection {
    conn := &share.CLUSConnection{
        AgentID:      Agent.ID,
        HostID:       Host.ID,
        ClientWL:     c.ClientWL,
        ServerWL:     c.ServerWL,
        ClientIP:     c.ClientIP,
        ServerIP:     c.ServerIP,
        ClientPort:   uint32(c.ClientPort),
        ServerPort:   uint32(c.ServerPort),
        IPProto:      uint32(c.IPProto),
        Application:  c.Application,
        Bytes:        c.Bytes,
        Sessions:     c.Sessions,
        FirstSeenAt:  c.FirstSeenAt,
        LastSeenAt:   c.LastSeenAt,
        ThreatID:     c.ThreatID,
        Severity:     uint32(c.Severity),
        PolicyAction: uint32(c.PolicyAction),
        PolicyId:     c.PolicyId,
        Ingress:      c.Ingress,
        ExternalPeer: c.ExternalPeer,
        LocalPeer:    c.LocalPeer,
        FQDN:         c.FQDN,
        Xff:          c.Xff,
        SvcExtIP:     c.SvcExtIP,
        Nbe:          c.Nbe,
        ToSidecar:    c.ToSidecar,
    }

    // 生成日志 UID
    if c.PolicyAction == C.DP_POLICY_ACTION_VIOLATE ||
        c.PolicyAction == C.DP_POLICY_ACTION_DENY {
        conn.LogUID = fmt.Sprintf("%s-%d", Agent.ID, logUID)
        logUID++
    }

    return conn
}
```

### 3.4 gRPC 发送

**源码位置**: `agent/grpc.go:271-291`

```go
func sendConnections(conns []*share.CLUSConnection) (*share.CLUSReportResponse, error) {
    // 创建 gRPC 连接
    conn, err := getControllerGRPCClient()
    if err != nil {
        return nil, err
    }

    // 构造请求
    req := &share.CLUSConnectionArray{
        Connections: conns,
    }

    // 发送到 Controller
    ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
    defer cancel()

    resp, err := share.NewControllerCapServiceClient(conn).
        ReportConnections(ctx, req)

    return resp, err
}
```

## 四、连接数据结构

### 4.1 DP 层连接结构

```go
// dp/types.go
type Connection struct {
    ClientWL     string    // 客户端工作负载 ID
    ServerWL     string    // 服务端工作负载 ID
    ClientIP     []byte    // 客户端 IP (4 或 16 字节)
    ServerIP     []byte    // 服务端 IP
    ClientPort   uint16    // 客户端端口
    ServerPort   uint16    // 服务端端口
    IPProto      uint8     // IP 协议 (6=TCP, 17=UDP)
    Application  uint32    // 应用 ID
    Bytes        uint64    // 传输字节数
    Sessions     uint32    // 会话数
    FirstSeenAt  uint32    // 首次发现时间
    LastSeenAt   uint32    // 最后发现时间
    ThreatID     uint32    // 威胁 ID
    Severity     uint8     // 威胁等级
    PolicyAction uint8     // 策略动作
    PolicyId     uint32    // 策略 ID
    Ingress      bool      // 是否入站
    ExternalPeer bool      // 是否外部对端
    FQDN         string    // 服务端域名
    // ...
}
```

### 4.2 CLUSConnection (Protobuf)

**源码位置**: `share/controller_service.proto:113-153`

```protobuf
message CLUSConnection {
    string AgentID = 1;
    string HostID = 2;
    string ClientWL = 3;
    string ServerWL = 4;
    bytes ClientIP = 5;
    bytes ServerIP = 6;
    uint32 ClientPort = 7;
    uint32 ServerPort = 8;
    uint32 IPProto = 9;
    uint32 Application = 10;
    uint64 Bytes = 11;
    uint32 Sessions = 12;
    uint32 FirstSeenAt = 13;
    uint32 LastSeenAt = 14;
    uint32 ThreatID = 15;
    uint32 Severity = 16;
    uint32 PolicyAction = 17;
    uint32 PolicyId = 18;
    bool Ingress = 19;
    bool ExternalPeer = 20;
    bool LocalPeer = 21;
    string FQDN = 22;
    bool Xff = 23;
    bool SvcExtIP = 24;
    bool Nbe = 25;
    bool ToSidecar = 26;
    string LogUID = 27;
    uint32 DlpId = 28;
    uint32 WafId = 29;
    // ... 共 38 个字段
}
```

## 五、Controller 端接收

### 5.1 gRPC 服务端

**源码位置**: `controller/rpc/rpc.go`

```go
func (s *CapServer) ReportConnections(ctx context.Context,
    req *share.CLUSConnectionArray) (*share.CLUSReportResponse, error) {

    // 调用缓存层处理连接
    cacher.UpdateConnections(req.Connections)

    // 返回响应 (可调整上报间隔)
    return &share.CLUSReportResponse{
        ReportInterval: calculateReportInterval(),
    }, nil
}
```

### 5.2 连接处理入口

**源码位置**: `controller/cache/connect.go:762-856`

```go
func UpdateConnections(conns []*share.CLUSConnection) {
    graphMutex.Lock()
    defer graphMutex.Unlock()

    for _, conn := range conns {
        // 1. 确定源和目标节点
        from := getEndpointID(conn.ClientWL, conn.ClientIP)
        to := getEndpointID(conn.ServerWL, conn.ServerIP)

        // 2. 跳过自连接
        if from == to {
            continue
        }

        // 3. 添加到图
        addConnectToGraph(from, to, conn)

        // 4. 处理违规/拒绝日志
        if conn.PolicyAction == C.DP_POLICY_ACTION_VIOLATE ||
            conn.PolicyAction == C.DP_POLICY_ACTION_DENY {
            logViolation(conn)
        }
    }

    // 5. 标记需要重新计算策略
    policyUpdated = true
}
```

## 六、连接聚合键

连接按以下维度聚合：

```go
type graphKey struct {
    port        uint16   // 服务端口
    ipproto     uint8    // IP 协议
    application uint32   // 应用 ID
    cip         uint32   // 客户端 IP (hash)
    sip         uint32   // 服务端 IP (hash)
}
```

## 七、上报策略

### 7.1 优先级处理

```go
// 高优先级连接 (总是上报)
- PolicyAction == DP_POLICY_ACTION_LEARN    // 学习模式
- PolicyAction == DP_POLICY_ACTION_VIOLATE  // 违规
- PolicyAction == DP_POLICY_ACTION_DENY     // 拒绝

// 普通连接 (缓存满时可能丢弃)
- PolicyAction == DP_POLICY_ACTION_ALLOW    // 允许
- PolicyAction == DP_POLICY_ACTION_OPEN     // 开放
```

### 7.2 动态间隔调整

Controller 可以根据负载动态调整上报间隔：

```go
// Controller 返回新的上报间隔
if resp.ReportInterval != 0 {
    connectReportInterval = resp.ReportInterval
}
```

## 八、简化实现示例

```go
package collector

import (
    "sync"
    "time"
)

type Connection struct {
    ClientID    string
    ServerID    string
    ClientIP    string
    ServerIP    string
    ServerPort  uint16
    Protocol    string
    Application string
    Bytes       uint64
    Sessions    uint32
    LastSeen    time.Time
}

type Collector struct {
    connections map[string]*Connection
    mu          sync.Mutex
    interval    time.Duration
    sendFunc    func([]*Connection) error
    stopCh      chan struct{}
}

func NewCollector(interval time.Duration, sendFunc func([]*Connection) error) *Collector {
    return &Collector{
        connections: make(map[string]*Connection),
        interval:    interval,
        sendFunc:    sendFunc,
        stopCh:      make(chan struct{}),
    }
}

// 添加或更新连接
func (c *Collector) AddConnection(conn *Connection) {
    key := c.makeKey(conn)

    c.mu.Lock()
    defer c.mu.Unlock()

    if existing, ok := c.connections[key]; ok {
        // 更新现有连接
        existing.Bytes += conn.Bytes
        existing.Sessions += conn.Sessions
        existing.LastSeen = conn.LastSeen
    } else {
        // 添加新连接
        c.connections[key] = conn
    }
}

func (c *Collector) makeKey(conn *Connection) string {
    return fmt.Sprintf("%s-%s-%s-%d-%s",
        conn.ClientID, conn.ServerID, conn.ServerIP,
        conn.ServerPort, conn.Protocol)
}

// 启动定期上报
func (c *Collector) Start() {
    ticker := time.NewTicker(c.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            c.flush()
        case <-c.stopCh:
            return
        }
    }
}

func (c *Collector) flush() {
    c.mu.Lock()
    if len(c.connections) == 0 {
        c.mu.Unlock()
        return
    }

    // 取出所有连接
    conns := make([]*Connection, 0, len(c.connections))
    for _, conn := range c.connections {
        conns = append(conns, conn)
    }
    c.connections = make(map[string]*Connection)
    c.mu.Unlock()

    // 发送
    if err := c.sendFunc(conns); err != nil {
        // 发送失败，放回缓存
        c.mu.Lock()
        for _, conn := range conns {
            key := c.makeKey(conn)
            c.connections[key] = conn
        }
        c.mu.Unlock()
    }
}

func (c *Collector) Stop() {
    close(c.stopCh)
}
```

## 九、关键要点

1. **批量上报**: 缓存连接后批量发送，减少 gRPC 调用次数
2. **优先级队列**: 违规/拒绝连接优先上报
3. **动态调整**: Controller 可调整上报间隔
4. **失败重试**: 发送失败时将连接放回缓存
5. **容量限制**: 缓存满时丢弃低优先级连接
