# NeuVector Traffic Control 微隔离技术深度调研报告

## 一、技术架构概览

NeuVector 使用 Linux Traffic Control (TC) 技术实现容器微隔离，**核心思想是在不修改内核的情况下，通过用户空间工具实现网络流量的拦截、镜像和转发控制**。

```
┌─────────────────────────────────────────────────────────────────┐
│                     容器网络命名空间                              │
│  ┌─────────────┐                                                │
│  │   应用进程   │                                                │
│  └──────┬──────┘                                                │
│         ↓                                                       │
│  ┌─────────────┐     veth pair      ┌─────────────────────────┐│
│  │ eth0 (local)│◄──────────────────►│ vin-xxx (inPort)        ││
│  │ 保留原MAC/IP │                    │ 移到 Enforcer 命名空间   ││
│  └─────────────┘                    └─────────────────────────┘│
└─────────────────────────────────────────────────────────────────┘
                                              ↓ (跨命名空间)
┌─────────────────────────────────────────────────────────────────┐
│                    Enforcer 网络命名空间                          │
│                                                                 │
│  ┌─────────────────┐    ┌─────────────────┐                    │
│  │ vex-xxx (exPort)│    │ vin-xxx (inPort)│                    │
│  │ 原容器网卡      │    │ veth 对端       │                    │
│  └────────┬────────┘    └────────┬────────┘                    │
│           │ TC ingress qdisc     │ TC ingress qdisc            │
│           │ + u32 filter         │ + u32 filter                │
│           └──────────┬───────────┘                             │
│                      ↓                                         │
│           ┌─────────────────────┐                              │
│           │   vbr-neuv (虚拟桥) │  ← TC 规则汇聚点              │
│           └──────────┬──────────┘                              │
│                      ↓                                         │
│           ┌─────────────────────┐                              │
│           │   vth-neuv (桥对端) │  → 连接 DP 数据平面           │
│           └─────────────────────┘                              │
└─────────────────────────────────────────────────────────────────┘
```

## 二、核心源码分析

### 2.1 关键文件位置

| 文件 | 作用 |
|------|------|
| `agent/pipe/tc.go` | TC 规则管理核心实现 |
| `agent/pipe/port.go` | 端口拦截、命名空间操作 |
| `agent/pipe/link_linux.go` | veth 对创建 |
| `agent/pipe/ovs.go` | OVS 驱动（备选方案） |

### 2.2 核心数据结构

```go
// tc.go:16-24
type tcPortInfo struct {
    idx  uint   // 端口在 enforcer 命名空间中的索引
    pref uint   // TC filter 优先级 ID
}

type tcPipeDriver struct {
    prefs   utils.Set                  // 已使用的优先级集合
    portMap map[string]*tcPortInfo     // 端口名 → 端口信息
}

// port.go:54-66 - 拦截端口对
type InterceptPair struct {
    Port   string              // 原端口名 (如 eth0)
    Peer   string              // 宿主机侧对端名
    inPort string              // 内部端口 (vin-xxx)
    exPort string              // 外部端口 (vex-xxx)
    MAC    net.HardwareAddr    // 原容器 MAC
    UCMAC  net.HardwareAddr    // NeuVector 标识 MAC (单播)
    BCMAC  net.HardwareAddr    // NeuVector 标识 MAC (广播)
    Addrs  []share.CLUSIPAddr  // IP 地址列表
}
```

## 三、关键技术特点总结

### 3.1 无内核模块设计

- 完全使用 Linux 标准工具: `tc`, `iptables`, `netlink`
- 无需加载内核模块，兼容性强
- 适用于任何支持 netfilter 的 Linux 系统

### 3.2 透明拦截

- 容器内应用无感知 (MAC/IP 保持不变)
- 不修改容器镜像
- 动态注入/移除

### 3.3 双层隔离

- **L2/L3**: TC filter 基于 MAC 地址匹配
- **L4/L7**: NFQUEUE + DP 数据平面 DPI 检测

### 3.4 防循环机制

- 使用特殊 UCMAC (`4e:65:75:56:xx:xx`) 标识已处理包
- vbr-neuv 上配置 DROP 规则防止循环

## 四、文档索引

- [02-tc-rules-implementation.md](./02-tc-rules-implementation.md) - TC 规则实现机制
- [03-container-port-interception.md](./03-container-port-interception.md) - 容器端口拦截机制
- [04-iptables-nfqueue-integration.md](./04-iptables-nfqueue-integration.md) - iptables + NFQUEUE 集成
- [05-dataplane-dpi-implementation.md](./05-dataplane-dpi-implementation.md) - 数据平面 DPI 实现
- [06-debugging-commands.md](./06-debugging-commands.md) - 调试命令参考
