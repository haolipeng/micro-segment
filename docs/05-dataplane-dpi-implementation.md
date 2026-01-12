# 数据平面 (DP) DPI 实现机制

## 一、DP 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         数据平面 (C 语言实现)                      │
├─────────────────────────────────────────────────────────────────┤
│  主程序入口                                                      │
│  ├── main.c          - 主循环、线程管理                          │
│  ├── nfq.c           - NFQUEUE 数据包接收                        │
│  ├── pkt.c           - 原始数据包处理                            │
│  └── ring.c          - 环形缓冲区管理                            │
├─────────────────────────────────────────────────────────────────┤
│  DPI 核心引擎 (dpi/)                                            │
│  ├── dpi_entry.c     - DPI 入口，数据包分发                      │
│  ├── dpi_session.c   - 会话管理 (TCP/UDP/ICMP)                  │
│  ├── dpi_packet.c    - 数据包解析 (L2/L3/L4)                    │
│  ├── dpi_parser.c    - 协议解析器调度                            │
│  ├── dpi_policy.c    - 策略匹配和执行                            │
│  └── dpi_log.c       - 日志记录                                 │
├─────────────────────────────────────────────────────────────────┤
│  协议解析器 (dpi/parsers/)                                       │
│  ├── dpi_http.c      - HTTP/HTTPS 解析                          │
│  ├── dpi_dns.c       - DNS 解析                                 │
│  ├── dpi_ssl.c       - TLS/SSL 解析                             │
│  ├── dpi_mysql.c     - MySQL 协议                               │
│  ├── dpi_postgresql.c- PostgreSQL 协议                          │
│  ├── dpi_kafka.c     - Kafka 协议                               │
│  ├── dpi_redis.c     - Redis 协议                               │
│  ├── dpi_mongodb.c   - MongoDB 协议                             │
│  ├── dpi_grpc.c      - gRPC 协议                                │
│  └── ... (更多协议)                                              │
├─────────────────────────────────────────────────────────────────┤
│  威胁检测 (dpi/sig/)                                             │
│  ├── dpi_sig.c       - 签名匹配引擎                              │
│  ├── dpi_search.c    - 模式搜索                                 │
│  ├── dpi_hs_search.c - Hyperscan 高速匹配                       │
│  └── dpi_sqlinjection.c - SQL 注入检测                          │
└─────────────────────────────────────────────────────────────────┘
```

## 二、DPI 入口和数据包处理

**源码位置**: `dp/dpi/dpi_entry.c:417-659`

```c
// DPI 主入口函数 - 返回值: 0=ACCEPT, 1=DROP
int dpi_recv_packet(io_ctx_t *ctx, uint8_t *ptr, int len) {
    // 1. 初始化线程本地数据包结构
    memset(&th_packet, 0, offsetof(dpi_packet_t, EOZ));
    th_packet.pkt = ptr;
    th_packet.cap_len = len;

    // 2. RCU 读锁 (无锁并发读)
    rcu_read_lock();

    // 3. 解析以太网头，查找端点 (EP)
    struct ethhdr *eth = (struct ethhdr *)(th_packet.pkt);

    // 根据 MAC 地址查找工作负载
    if (cmp_mac_prefix(eth->h_source, MAC_PREFIX)) {
        mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
    } else if (cmp_mac_prefix(eth->h_dest, MAC_PREFIX)) {
        mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
        th_packet.flags |= DPI_PKT_FLAG_INGRESS;  // 标记为入站
    }

    // 4. 解析 L3/L4 层
    action = dpi_parse_ethernet(&th_packet);

    // 5. 执行 DPI 检测
    if (action == DPI_ACTION_NONE && inspect) {
        action = dpi_inspect_ethernet(&th_packet);
    }

    rcu_read_unlock();

    // 6. 根据动作决定转发或丢弃
    if (!tap && action != DPI_ACTION_DROP && action != DPI_ACTION_RESET) {
        if (nfq) return 0;  // NFQUEUE: ACCEPT
        g_io_callback->send_packet(ctx, ptr, len);
    } else {
        if (nfq) return 1;  // NFQUEUE: DROP
    }
    return 0;
}
```

## 三、会话管理

NeuVector 维护每个 TCP/UDP 连接的会话状态，用于：
- 跟踪连接状态 (TCP 状态机)
- 关联多个数据包到同一会话
- 支持 DPI 解析器的上下文持久化

**源码位置**: `dp/dpi/dpi_session.h:98-133`

```c
// 会话结构
typedef struct dpi_session_ {
    struct cds_lfht_node node;          // 无锁哈希表节点
    timer_entry_t ts_entry;             // 超时定时器

    uint32_t id;                        // 会话 ID
    uint32_t created_at;                // 创建时间

    dpi_wing_t client, server;          // 客户端/服务端状态
    void *parser_data[DPI_PARSER_MAX];  // 各解析器的私有数据

    uint16_t flags;                     // 会话标志
    uint16_t app, base_app;             // 识别的应用
    uint8_t ip_proto;                   // IP 协议 (TCP/UDP/ICMP)
    uint8_t action;                     // 策略动作
    uint8_t severity;                   // 威胁等级
    uint32_t threat_id;                 // 威胁 ID

    dpi_policy_desc_t policy_desc;      // 匹配的策略
    BITMASK_DEFINE(parser_bits, DPI_PARSER_MAX);  // 活跃的解析器
} dpi_session_t;

// 连接方向状态
typedef struct dpi_wing_ {
    uint8_t mac[ETH_ALEN];              // MAC 地址
    uint16_t port;                      // 端口号
    io_ip_t ip;                         // IP 地址
    uint32_t next_seq, init_seq;        // TCP 序列号
    uint32_t tcp_acked, tcp_win;        // TCP 确认和窗口
    uint8_t tcp_state;                  // TCP 状态
    uint32_t pkts, bytes;               // 统计信息
    asm_t asm_cache;                    // 重组缓存
} dpi_wing_t;
```

**会话标志**:
```c
#define DPI_SESS_FLAG_IPV4           0x0001  // IPv4 会话
#define DPI_SESS_FLAG_INGRESS        0x0010  // 入站方向
#define DPI_SESS_FLAG_TAP            0x0020  // TAP 模式
#define DPI_SESS_FLAG_ESTABLISHED    0x0100  // 已建立状态
#define DPI_SESS_FLAG_POLICY_APP_READY 0x0200 // 应用已识别
```

## 四、协议解析器框架

NeuVector 使用插件式解析器架构，每种协议实现独立的解析器。

**源码位置**: `dp/dpi/dpi_parser.c`

```c
// 解析器接口
typedef struct dpi_parser_ {
    void (*new_session)(dpi_packet_t *p);   // 新会话回调
    void (*delete_data)(void *data);        // 清理数据回调
    void (*parser)(dpi_packet_t *p);        // 解析回调
    void (*midstream)(dpi_packet_t *p);     // 中途会话回调

    const char *name;                       // 解析器名称
    uint8_t ip_proto;                       // 支持的协议
    uint8_t type;                           // 解析器类型 ID
} dpi_parser_t;

// 解析器类型到应用 ID 映射
int dpi_parser_2_app[DPI_PARSER_MAX] = {
    [DPI_PARSER_HTTP] = DPI_APP_HTTP,
    [DPI_PARSER_SSL]  = DPI_APP_SSL,
    [DPI_PARSER_SSH]  = DPI_APP_SSH,
    [DPI_PARSER_DNS]  = DPI_APP_DNS,
    [DPI_PARSER_MYSQL] = DPI_APP_MYSQL,
    [DPI_PARSER_REDIS] = DPI_APP_REDIS,
    [DPI_PARSER_KAFKA] = DPI_APP_KAFKA,
    [DPI_PARSER_MONGODB] = DPI_APP_MONGODB,
    [DPI_PARSER_GRPC] = DPI_APP_GRPC,
    // ... 更多映射
};

// 解析器注册
void dpi_parser_setup(void) {
    register_parser(dpi_http_tcp_parser());
    register_parser(dpi_ssl_parser());
    register_parser(dpi_ssh_parser());
    register_parser(dpi_dns_tcp_parser());
    register_parser(dpi_mysql_parser());
    register_parser(dpi_redis_parser());
    register_parser(dpi_kafka_parser());
    register_parser(dpi_grpc_tcp_parser());
    // ... 更多解析器
}
```

**解析器调度流程**:
```c
// dpi_parser.c:197-282
void dpi_proto_parser(dpi_packet_t *p) {
    dpi_session_t *s = p->session;
    dpi_parser_t **list = get_parser_list(p->ip_proto);

    // 如果已确定唯一解析器
    if (s->flags & DPI_SESS_FLAG_ONLY_PARSER) {
        cp = list[s->only_parser];
        cp->parser(p);
    } else {
        // 遍历所有可能的解析器
        for (t = 0; t < DPI_PARSER_MAX; t++) {
            cp = list[t];
            if (cp && BITMASK_TEST(s->parser_bits, t)) {
                cp->parser(p);

                // 解析器可以选择退出
                if (!BITMASK_TEST(s->parser_bits, t)) {
                    dpi_delete_parser_data(s, cp);
                } else {
                    p->parser_left++;
                }
            }
        }

        // 只剩一个解析器时，标记为最终解析器
        if (p->parser_left == 1) {
            s->flags |= DPI_SESS_FLAG_LAST_PARSER;
            s->only_parser = last;
        }
    }
}
```

## 五、HTTP 解析器示例

**源码位置**: `dp/dpi/parsers/dpi_http.c`

```c
// HTTP 会话数据
typedef struct http_data_ {
    http_wing_t client, server;
    uint16_t status;           // HTTP 状态码
    uint8_t method;            // GET/POST/PUT/DELETE/HEAD
    uint8_t proto;             // HTTP/SIP/RTSP
} http_data_t;

// HTTP 方法识别
static http_method_t http_method[] = {
    {"GET",     3, HTTP_PROTO_HTTP, HTTP_METHOD_GET},
    {"PUT",     3, HTTP_PROTO_HTTP, HTTP_METHOD_PUT},
    {"POST",    4, HTTP_PROTO_HTTP, HTTP_METHOD_POST},
    {"DELETE",  6, HTTP_PROTO_HTTP, HTTP_METHOD_DELETE},
    {"HEAD",    4, HTTP_PROTO_HTTP, HTTP_METHOD_HEAD},
    {"CONNECT", 7, HTTP_PROTO_HTTP, HTTP_METHOD_NONE},
};

// HTTP Slowloris 攻击检测
int dpi_http_tick_timeout(dpi_session_t *s, void *parser_data) {
    http_data_t *data = parser_data;

    // 检测头部超时 (Slowloris 攻击特征)
    if (data->url_start_tick > 0) {
        if (th_snap.tick - data->url_start_tick >= HTTP_HEADER_COMPLETE_TIMEOUT) {
            dpi_threat_log_by_session(DPI_THRT_HTTP_SLOWLORIS, s,
                "Header duration=%us, threshold=%us", ...);
            return DPI_SESS_TICK_RESET;  // 重置连接
        }
    }
    return DPI_SESS_TICK_CONTINUE;
}
```

## 六、支持的协议列表

| 协议 | 解析器文件 | 检测能力 |
|------|-----------|---------|
| HTTP/HTTPS | dpi_http.c | 方法、URL、Header、Slowloris 检测 |
| DNS | dpi_dns.c | 查询/响应解析、DNS 隧道检测 |
| TLS/SSL | dpi_ssl.c | 版本、证书、SNI 提取 |
| SSH | dpi_ssh.c | 版本检测 |
| MySQL | dpi_mysql.c | SQL 命令、SQL 注入检测 |
| PostgreSQL | dpi_postgresql.c | SQL 命令解析 |
| Redis | dpi_redis.c | 命令解析 |
| MongoDB | dpi_mongodb.c | BSON 解析 |
| Kafka | dpi_kafka.c | 消息解析 |
| gRPC | dpi_grpc.c | HTTP/2 帧解析 |
| Cassandra | dpi_cassandra.c | CQL 解析 |
| Zookeeper | dpi_zookeeper.c | 命令解析 |
| Couchbase | dpi_couchbase.c | 请求解析 |
| Oracle TNS | dpi_tns.c | TNS 协议 |
| MS SQL TDS | dpi_tds.c | TDS 协议 |

## 七、威胁检测机制

**签名匹配引擎**: `dp/dpi/sig/`

```c
// 使用 Hyperscan 进行高速模式匹配
// dpi_hs_search.c - Intel Hyperscan 集成

// SQL 注入检测
// dpi_sqlinjection.c
void sql_injection_init(void);

// 威胁类型定义
#define DPI_THRT_HTTP_SLOWLORIS    // HTTP Slowloris 攻击
#define DPI_THRT_DNS_TUNNEL        // DNS 隧道
#define DPI_THRT_SSL_HEARTBLEED    // Heartbleed 漏洞
#define DPI_THRT_SQL_INJECTION     // SQL 注入
// ... 更多威胁类型
```

## 八、RST 注入 (连接重置)

当检测到威胁或策略违规时，NeuVector 可以注入 TCP RST 包来终止连接：

**源码位置**: `dp/dpi/dpi_entry.c:252-358`

```c
void dpi_inject_reset_by_session(dpi_session_t *sess, bool to_server) {
    // TAP 模式不注入
    if (FLAGS_TEST(sess->flags, DPI_SESS_FLAG_TAP)) return;

    // 构造 RST 包
    // L2: 以太网头
    eth = (struct ethhdr *)buffer;
    eth->h_proto = htons(ETH_P_IP);
    mac_cpy(eth->h_source, ...);
    mac_cpy(eth->h_dest, ...);

    // L3: IP 头
    iph = (struct iphdr *)(buffer + sizeof(struct ethhdr));
    iph->protocol = IPPROTO_TCP;
    iph->saddr = to_server ? c->ip.ip4 : s->ip.ip4;
    iph->daddr = to_server ? s->ip.ip4 : c->ip.ip4;

    // L4: TCP 头 (RST 标志)
    tcph = (struct tcphdr *)(buffer + sizeof(struct ethhdr) + sizeof(struct iphdr));
    tcph->th_flags = TH_RST;
    tcph->th_seq = htonl(to_server ? c->next_seq : s->next_seq);

    // 发送 RST 包
    g_io_callback->send_packet(&ctx, buffer, sizeof(buffer));
}
```

## 九、线程模型和性能优化

```c
// dpi_module.h:36-65
typedef struct dpi_thread_data_ {
    dpi_packet_t packet;            // 线程本地数据包
    dpi_snap_t snap;                // 快照
    io_stats_t stats;               // 统计

    rcu_map_t session4_map;         // IPv4 会话表
    rcu_map_t session6_map;         // IPv6 会话表
    timer_wheel_t timer;            // 定时器轮

    uint8_t dp_msg[DP_MSG_SIZE];    // 消息缓冲
} dpi_thread_data_t;

// 线程本地存储宏
#define th_packet   (g_dpi_thread_data[THREAD_ID].packet)
#define th_session4_map (g_dpi_thread_data[THREAD_ID].session4_map)
```

**性能优化技术**:
1. **RCU (Read-Copy-Update)**: 无锁并发读取端点和会话数据
2. **线程本地存储**: 每个 DP 线程独立的会话表和缓冲区
3. **Hyperscan**: Intel 高性能正则表达式引擎
4. **零拷贝**: 直接处理 NFQUEUE 数据包指针

## 十、完整数据流总结

```
┌─────────────────────────────────────────────────────────────────┐
│ 1. 容器应用发起连接                                              │
│    App → eth0 (容器内)                                          │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌────────────────────────────▼────────────────────────────────────┐
│ 2. TC 规则拦截 (Enforcer 命名空间)                               │
│    vin-xxx → TC filter → pedit MAC → mirred → vbr-neuv         │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌────────────────────────────▼────────────────────────────────────┐
│ 3. DP 数据平面接收                                               │
│    vth-neuv → AF_PACKET socket → ring buffer                   │
│    或: iptables NFQUEUE → netlink socket                       │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌────────────────────────────▼────────────────────────────────────┐
│ 4. DPI 检测流程                                                  │
│    dpi_recv_packet()                                            │
│    ├── dpi_parse_ethernet()  → L2/L3/L4 解析                   │
│    ├── session lookup/create → 会话管理                         │
│    ├── dpi_proto_parser()    → 协议识别 (HTTP/MySQL/...)       │
│    ├── dpi_sig_detect()      → 威胁签名匹配                     │
│    └── dpi_policy_check()    → 策略匹配                         │
└────────────────────────────┬────────────────────────────────────┘
                             ↓
┌────────────────────────────▼────────────────────────────────────┐
│ 5. 动作执行                                                      │
│    ├── ACCEPT → 转发数据包 (TC redirect 或 NFQUEUE verdict)    │
│    ├── DROP   → 丢弃数据包                                      │
│    ├── RESET  → 注入 TCP RST                                    │
│    └── LOG    → 记录威胁/违规日志                               │
└─────────────────────────────────────────────────────────────────┘
```
