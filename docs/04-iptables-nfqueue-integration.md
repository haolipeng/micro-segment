# iptables + NFQUEUE 集成

## 一、NFQUEUE 规则创建

NeuVector 使用 iptables NFQUEUE 目标将特定流量送入用户空间处理：

```go
// port.go:1393-1446 - insertIptablesNvRules 函数

func insertIptablesNvRules(intf string, isloopback bool, qno int, appMap map[share.CLUSProtoPort]*share.CLUSApp) {
    for p := range appMap {
        if p.IPProto == syscall.IPPROTO_TCP {
            // 网络接口: 匹配目标端口
            cmd = fmt.Sprintf("iptables -I %v -t filter -i %v -p tcp --dport %d "+
                "-j NFQUEUE --queue-num %d --queue-bypass",
                nvInputChain, intf, p.Port, qno)

            // 响应流量: 匹配源端口
            cmd = fmt.Sprintf("iptables -I %v -t filter -o %v -p tcp --sport %d "+
                "-j NFQUEUE --queue-num %d --queue-bypass",
                nvOutputChain, intf, p.Port, qno)
        }
    }
}
```

## 二、iptables 链结构

```
INPUT Chain:
┌─────────────────────────────────────────┐
│ NV_INPUT_PROXYMESH                      │
│  ├─ -p tcp --dport 8080 -j NFQUEUE     │ ← 应用端口
│  ├─ -p tcp --dport 443 -j NFQUEUE      │
│  ├─ -p udp --dport 53 -j NFQUEUE       │
│  └─ -j RETURN (默认)                    │
└─────────────────────────────────────────┘

OUTPUT Chain:
┌─────────────────────────────────────────┐
│ NV_OUTPUT_PROXYMESH                     │
│  ├─ -p tcp --sport 8080 -j NFQUEUE     │ ← 响应流量
│  └─ -j RETURN                           │
└─────────────────────────────────────────┘
```

## 三、NFQUEUE 数据包接收

NFQUEUE 是 Linux netfilter 框架的用户空间队列机制，NeuVector 用它来接收需要 DPI 检测的数据包。

**源码位置**: `dp/nfq.c:27-76`

```c
// NFQUEUE 回调函数 - 每个数据包触发
static int dp_nfq_rx_cb(struct nfq_q_handle *qh, struct nfgenmsg *nfmsg,
                       struct nfq_data *nfa, void *data) {
    // 1. 获取数据包 ID 和负载
    ph = nfq_get_msg_packet_hdr(nfa);
    id = ntohl(ph->packet_id);
    ret = nfq_get_payload(nfa, &payload_data);

    // 2. NFQUEUE 只返回 L3 层数据，需要构造伪以太网头
    nfq_eth = (struct ethhdr *)dpi_rcv_pkt_ptr;
    memset(nfq_eth->h_dest, 0, ETHER_ADDR_LEN);
    memset(nfq_eth->h_source, 0, ETHER_ADDR_LEN);
    nfq_eth->h_proto = htons(ETH_P_IP);
    memcpy(&dpi_rcv_pkt_ptr[sizeof(struct ethhdr)], payload_data, ret);

    // 3. 调用 DPI 引擎进行检测
    verdict = dpi_recv_packet(&context, dpi_rcv_pkt_ptr, total_len);

    // 4. 根据检测结果设置裁决
    if (verdict == 1) {  // DROP
        nfq_set_verdict(ctx->nfq_ctx.nfq_q_hdl, id, NF_DROP, 0, NULL);
    } else {             // ACCEPT
        nfq_set_verdict(ctx->nfq_ctx.nfq_q_hdl, id, NF_ACCEPT, 0, NULL);
    }
}
```

## 四、NFQUEUE 句柄配置

```c
// nfq.c:155-243 - 打开 NFQUEUE 句柄
int dp_open_nfq_handle(dp_context_t *ctx, int qnum, ...) {
    nfq_hdl = nfq_open();
    nfq_bind_pf(nfq_hdl, AF_INET);
    nfq_q_hdl = nfq_create_queue(nfq_hdl, qnum, &dp_nfq_rx_cb, ctx);

    // 设置复制整个数据包
    nfq_set_mode(nfq_q_hdl, NFQNL_COPY_PACKET, 0xffff);

    // 设置 FAIL_OPEN: 队列满时自动放行 (避免阻塞)
    nfq_set_queue_flags(nfq_q_hdl, NFQA_CFG_F_FAIL_OPEN, NFQA_CFG_F_FAIL_OPEN);

    // 设置 GSO: 不规范化 offload 包
    nfq_set_queue_flags(nfq_q_hdl, NFQA_CFG_F_GSO, NFQA_CFG_F_GSO);
}
```

## 五、关键配置说明

| 配置项 | 说明 |
|--------|------|
| `NFQNL_COPY_PACKET` | 复制整个数据包到用户空间 |
| `NFQA_CFG_F_FAIL_OPEN` | 队列满时自动放行，避免网络阻塞 |
| `NFQA_CFG_F_GSO` | 不规范化 GSO 包，提高性能 |
| `--queue-bypass` | iptables 规则中的旁路选项 |

## 六、NFQUEUE 与 TC 的配合

```
┌─────────────────────────────────────────────────────────────────┐
│                     数据包处理流程                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  TC 模式 (默认):                                                │
│  容器 → veth → TC filter → vbr-neuv → DP (AF_PACKET)           │
│                                                                 │
│  NFQUEUE 模式 (ProxyMesh/特定应用):                             │
│  容器 → iptables → NFQUEUE → DP (netlink) → verdict            │
│                                                                 │
│  混合模式:                                                       │
│  TC 处理 L2/L3 转发，NFQUEUE 处理特定应用端口的 L7 检测         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```
