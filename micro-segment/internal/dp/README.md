# DP (数据平面) 实现

## 概述

DP层是微隔离系统的核心，负责实时策略匹配和流量控制。

## 目录结构

```
dp/
├── main.c              # 主程序入口
├── main.h              # 主程序头文件
├── debug.c/h           # 调试日志
├── apis.h              # API定义
├── defs.h              # 全局定义
├── base.h              # 基础定义
├── ctrl/               # 控制接口
│   └── ctrl.c          # Unix Socket通信
├── dpi/                # DPI引擎
│   ├── dpi_policy.c/h  # 策略匹配引擎
│   ├── dpi_session.c/h # 会话管理
│   └── dpi_packet.c/h  # 数据包处理
├── nfq/                # Netfilter队列
│   └── nfq.c           # NFQ处理
└── utils/              # 工具库
    ├── rcu_map.c/h     # 无锁哈希表
    ├── helper.c/h      # 辅助函数
    ├── timer_*.c/h     # 定时器
    └── bitmap.c/h      # 位图
```

## 核心功能

### 1. 策略匹配引擎 (dpi/dpi_policy.c)

**功能**：
- 基于5元组的策略查找
- 支持精确匹配和范围匹配
- 使用RCU无锁哈希表实现高性能

**关键函数**：
```c
int dpi_policy_lookup(dpi_packet_t *p, dpi_policy_hdl_t *hdl, 
                      uint32_t app, bool to_server, bool xff, 
                      dpi_policy_desc_t *desc, uint32_t xff_replace_dst_ip);
```

**匹配流程**：
1. 提取5元组 (sip, dip, dport, proto, app)
2. 精确匹配：`rcu_map_lookup(&hdl->policy_map, &key)`
3. 范围匹配：`rcu_map_lookup(&hdl->range_policy_map, &key)`
4. 返回默认动作：`hdl->def_action`

### 2. 会话管理 (dpi/dpi_session.c)

**功能**：
- 维护连接会话状态
- 跟踪流量统计
- 检测东西向流量（POLICY_DESC_INTERNAL）

**关键数据结构**：
```c
typedef struct dpi_session_ {
    uint32_t id;
    io_ip_t client_ip;
    io_ip_t server_ip;
    uint16_t client_port;
    uint16_t server_port;
    uint8_t ip_proto;
    dpi_policy_desc_t policy_desc;
    // ...
} dpi_session_t;
```

### 3. 控制接口 (ctrl/ctrl.c)

**功能**：
- 通过Unix Socket接收Agent命令
- 加载策略到内存
- 上报连接数据

**Socket路径**：`/tmp/dp_listen.sock`

**消息格式**：JSON

### 4. 网络拦截 (nfq/nfq.c)

**功能**：
- 使用Netfilter Queue拦截数据包
- 执行策略判决
- 允许/拒绝流量

## 编译

```bash
cd internal/dp
make
```

**依赖库**：
- libjansson - JSON解析
- liburcu - RCU无锁并发
- libnetfilter_queue - 网络拦截

## 运行

```bash
sudo ./bin/dp -c configs/dp.conf
```

**需要root权限**：
- 访问Netfilter Queue
- 配置网络规则

## 性能优化

### 1. 无锁并发 (RCU)

使用Read-Copy-Update实现无锁读取：
- 读操作：O(1)，无锁
- 写操作：复制-更新-发布

### 2. 哈希表

策略存储在哈希表中：
- 查找：O(1)
- 冲突解决：链表法

### 3. 多线程

支持多个工作线程并行处理数据包。

## 简化说明

相比NeuVector，本版本移除了：

1. **学习模式** (DP_POLICY_ACTION_LEARN)
   - 不再支持自动规则生成
   - 仅支持Monitor和Protect模式

2. **服务网格支持** (POLICY_DESC_MESH_TO_SVR)
   - 移除Istio/Linkerd特殊处理
   - 移除sidecar检测逻辑

3. **FQDN规则** (POLICY_HDL_FLAG_FQDN)
   - 移除域名匹配功能
   - 仅支持IP地址规则

4. **DLP/WAF**
   - 移除数据防泄漏功能
   - 移除Web应用防火墙

## 调试

### 启用调试日志

```bash
./bin/dp -c configs/dp.conf -d
```

### 查看策略

```bash
# 发送SIGUSR1信号
kill -USR1 <dp_pid>

# 查看日志
tail -f /var/log/micro-segment/dp.log
```

### 性能分析

```bash
# 使用perf
perf record -g ./bin/dp -c configs/dp.conf
perf report

# 使用valgrind
valgrind --tool=callgrind ./bin/dp -c configs/dp.conf
```

## 故障排查

### 1. 无法启动

**问题**：Permission denied
```
Failed to create nfq queue: Operation not permitted
```

**解决**：
```bash
# 使用root权限
sudo ./bin/dp -c configs/dp.conf

# 或添加capabilities
sudo setcap cap_net_admin,cap_net_raw+ep ./bin/dp
```

### 2. 策略不生效

**检查**：
- Agent是否正常运行
- Unix Socket是否连接
- 策略是否正确加载

```bash
# 检查Socket
ls -la /tmp/dp_listen.sock

# 查看日志
tail -f /var/log/micro-segment/dp.log
```

### 3. 性能问题

**优化**：
- 增加工作线程数
- 调整哈希表大小
- 检查策略规则数量

## 参考

- [Netfilter Queue文档](https://www.netfilter.org/projects/libnetfilter_queue/)
- [RCU原理](https://lwn.net/Articles/262464/)
- [策略匹配算法](../../docs/architecture.md)
