/**
 * @file dpi_entry.c
 * @brief DPI模块主入口 - 深度包检测核心实现
 * 
 * 本文件实现数据包接收、解析、会话管理的核心逻辑，是流量分析的主入口。
 * 主要功能：
 *   - 数据包接收与方向判断
 *   - 工作负载端点(endpoint)查找与匹配
 *   - 会话创建与应用识别
 *   - TCP RST注入（用于阻断连接）
 *   - 内部IP/策略地址判断
 */

#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <linux/if_ether.h>
#include <arpa/inet.h>

#include "urcu.h"

#include "apis.h"
#include "utils/helper.h"
#include "utils/timer_wheel.h"
#include "utils/rcu_map.h"
#include "dpi/dpi_module.h"

extern void dpi_packet_setup(void);
extern void dpi_parser_setup(void);

/* 全局IO回调函数指针，用于发送数据包 */
io_callback_t *g_io_callback;
/* 全局IO配置，包含混杂模式等设置 */
io_config_t *g_io_config;

/* 每个DP线程的私有数据 */
dpi_thread_data_t g_dpi_thread_data[MAX_DP_THREADS];

/**
 * @brief DPI全局初始化（进程级别，只调用一次）
 * @param cb IO回调函数集合（send_packet等）
 * @param cfg IO配置参数
 */
void dpi_setup(io_callback_t *cb, io_config_t *cfg)
{
    g_io_callback = cb;
    g_io_config = cfg;

    dpi_packet_setup();   /* 初始化数据包处理相关结构 */
    dpi_parser_setup();   /* 初始化协议解析器 */
}

/**
 * @brief DPI线程初始化（每个DP线程调用一次）
 * @param reason 初始化原因（预留参数）
 * 
 * 分配线程私有的数据包缓冲区，初始化各子模块：
 *   - defrag_data: IP分片重组缓冲区
 *   - asm_pkt: TCP流重组缓冲区
 *   - decoded_pkt: 解码后数据包缓冲区
 */
void dpi_init(int reason)
{
    /* 分配IP分片重组缓冲区 */
    th_packet.defrag_data = malloc(DPI_MAX_PKT_LEN);
    if (th_packet.defrag_data == NULL) {
        DEBUG_ERROR(DBG_INIT, "Failed to allocate defrag_data buffer (%d bytes)\n",
                    DPI_MAX_PKT_LEN);
        exit(EXIT_FAILURE);
    }

    /* 分配TCP流重组缓冲区 */
    th_packet.asm_pkt.ptr = malloc(DPI_MAX_PKT_LEN);
    if (th_packet.asm_pkt.ptr == NULL) {
        DEBUG_ERROR(DBG_INIT, "Failed to allocate asm_pkt buffer (%d bytes)\n",
                    DPI_MAX_PKT_LEN);
        free(th_packet.defrag_data);
        exit(EXIT_FAILURE);
    }

    /* 分配解码数据包缓冲区 */
    th_packet.decoded_pkt.ptr = malloc(DPI_MAX_PKT_LEN);
    if (th_packet.decoded_pkt.ptr == NULL) {
        DEBUG_ERROR(DBG_INIT, "Failed to allocate decoded_pkt buffer (%d bytes)\n",
                    DPI_MAX_PKT_LEN);
        free(th_packet.defrag_data);
        free(th_packet.asm_pkt.ptr);
        exit(EXIT_FAILURE);
    }

    /* 初始化各子模块 */
    timer_wheel_init(&th_timer);      /* 定时器轮 */
    dpi_frag_init();                  /* IP分片处理 */
    dpi_session_init();               /* 会话管理 */
    dpi_meter_init();                 /* 流量计量 */
    dpi_log_init();                   /* 日志模块 */
    dpi_policy_init();                /* 策略引擎 */
    dpi_unknown_ip_init();            /* 未知IP追踪 */
    dpi_ip_fqdn_storage_init();       /* IP-FQDN映射存储 */
}

/**
 * @brief 在端点的应用映射表中查找应用
 * @param ep 端点指针
 * @param port 端口号
 * @param ip_proto IP协议号（TCP=6, UDP=17）
 * @return 找到返回io_app_t指针，否则返回NULL
 */
io_app_t *dpi_ep_app_map_lookup(io_ep_t *ep, uint16_t port, uint8_t ip_proto)
{
    io_app_t key;

    key.port = port;
    key.ip_proto = ip_proto;
    return rcu_map_lookup(&ep->app_map, &key);
}

/**
 * @brief 查找或创建端点的应用映射条目
 * @param ep 端点指针
 * @param port 端口号
 * @param ip_proto IP协议号
 * @return 应用条目指针，失败返回NULL
 * 
 * 如果应用不存在则自动创建新条目
 */
static io_app_t *ep_app_map_locate(io_ep_t *ep, uint16_t port, uint8_t ip_proto)
{
    io_app_t *app;

    app = dpi_ep_app_map_lookup(ep, port, ip_proto);
    if (unlikely(app == NULL)) {
        app = calloc(1, sizeof(*app));
        if (app == NULL) return NULL;

        ep->app_ports ++;

        app->port = port;
        app->ip_proto = ip_proto;
        app->src = APP_SRC_DP;  /* 标记为DP层发现的应用 */
        rcu_map_add(&ep->app_map, app, app);
        DEBUG_LOG(DBG_SESSION, NULL, "dp add app port=%u ip_proto=%u\n",
                              port, ip_proto);
    }

    return app;
}

/**
 * @brief 设置会话的应用层协议
 * @param p 数据包上下文
 * @param proto 协议标识（如HTTP、DNS等）
 * 
 * 仅对入站会话生效，用于DPI识别后更新应用信息
 */
void dpi_ep_set_proto(dpi_packet_t *p, uint16_t proto)
{
    dpi_session_t *s = p->session;

    if (!FLAGS_TEST(s->flags, DPI_SESS_FLAG_INGRESS)) return;

    io_app_t *app = ep_app_map_locate(p->ep, s->server.port, s->ip_proto);
    if (unlikely(app == NULL)) return;

    DEBUG_LOG(DBG_SESSION, p, "port=%u ip_proto=%u proto=%u\n",
                              s->server.port, s->ip_proto, proto);

    if (proto != 0 && unlikely(app->proto != proto)) {
        app->proto = proto;
        uatomic_set(&p->ep->app_updated, 1);  /* 标记应用信息已更新 */
    }
}

/**
 * @brief 获取会话的应用类型
 * @param p 数据包上下文
 * @return 应用类型标识，未知返回0
 */
uint16_t dpi_ep_get_app(dpi_packet_t *p)
{
    dpi_session_t *s = p->session;

    if (!FLAGS_TEST(s->flags, DPI_SESS_FLAG_INGRESS)) return 0;

    io_app_t *app = ep_app_map_locate(p->ep, s->server.port, s->ip_proto);
    if (app == NULL) return 0;

    return app->application;
}

/**
 * @brief 设置会话的服务器类型和应用类型
 * @param p 数据包上下文
 * @param server 服务器类型（如nginx、apache）
 * @param application 应用类型（如web、database）
 * 
 * 由DPI协议解析器调用，用于记录识别结果
 */
void dpi_ep_set_app(dpi_packet_t *p, uint16_t server, uint16_t application)
{
    dpi_session_t *s = p->session;

    if (!FLAGS_TEST(s->flags, DPI_SESS_FLAG_INGRESS)) return;

    io_app_t *app = ep_app_map_locate(p->ep, s->server.port, s->ip_proto);
    if (unlikely(app == NULL)) return;

    DEBUG_LOG(DBG_SESSION, p, "port=%u server=%u application=%u\n",
                              s->server.port, server, application);

    if (server != 0 && unlikely(app->server != server)) {
        app->server = server;
        uatomic_set(&p->ep->app_updated, 1);
    }
    if (application != 0 && unlikely(app->application != application)) {
        app->application = application;
        uatomic_set(&p->ep->app_updated, 1);
    }
}

void dpi_ep_set_server_ver(dpi_packet_t *p, char *ver, int len)
{
    dpi_session_t *s = p->session;
    int size = min(len+1, SERVER_VER_SIZE);

    if (!(s->flags & DPI_SESS_FLAG_INGRESS)) return;

    io_app_t *app = ep_app_map_locate(p->ep, s->server.port, s->ip_proto);
    if (unlikely(app == NULL)) return;

    strncpy(app->version, ver, size-1);
    //version is null terminated
    app->version[size-1] = '\0';
    DEBUG_LOG(DBG_SESSION, p, "port=%u version=%s\n", s->server.port, app->version);
}

void dpi_print_ip4_internal_fp(FILE *logfp)
{
    int i;
    fprintf(logfp, "INTERNAL SUBNET\n");
    for (i = 0; i < th_internal_subnet4->count; i++) {
        fprintf(logfp, "    internal ip/mask="DBG_IPV4_FORMAT"/"DBG_IPV4_FORMAT"\n",
                    DBG_IPV4_TUPLE(th_internal_subnet4->list[i].ip),
                    DBG_IPV4_TUPLE(th_internal_subnet4->list[i].mask));
    }
    fprintf(logfp, "SPECIAL IP\n");
    for (i = 0; i < th_specialip_subnet4->count; i++) {
        fprintf(logfp, "    special ip/mask="DBG_IPV4_FORMAT"/"DBG_IPV4_FORMAT" iptype:%d\n",
                    DBG_IPV4_TUPLE(th_specialip_subnet4->list[i].ip),
                    DBG_IPV4_TUPLE(th_specialip_subnet4->list[i].mask),
                    th_specialip_subnet4->list[i].iptype);
    }
    fprintf(logfp, "POLICY ADDRESS MAP\n");
    for (i = 0; i < th_policy_addr->count; i++) {
        fprintf(logfp, "    policy ip/mask="DBG_IPV4_FORMAT"/"DBG_IPV4_FORMAT"\n",
                    DBG_IPV4_TUPLE(th_policy_addr->list[i].ip),
                    DBG_IPV4_TUPLE(th_policy_addr->list[i].mask));
    }
}

/**
 * @brief 判断IPv4地址是否为内部地址
 * @param ip 网络字节序的IPv4地址
 * @return true=内部地址，false=外部地址
 * 
 * 检查顺序：
 *   1. 回环地址(127.x.x.x)直接返回true
 *   2. 遍历内部子网列表进行匹配
 */
bool dpi_is_ip4_internal(uint32_t ip)
{
    int i;
    /* 回环地址或内部子网列表为空时，默认为内部地址 */
    if (unlikely(th_internal_subnet4 == NULL) || (th_internal_subnet4->count == 0)
        || ip == htonl(INADDR_LOOPBACK) || IS_IN_LOOPBACK(ntohl(ip))) {
        return true;
    }
    /* 遍历内部子网列表进行掩码匹配 */
    for (i = 0; i < th_internal_subnet4->count; i++) {
        if ((ip & th_internal_subnet4->list[i].mask) == th_internal_subnet4->list[i].ip) {
            return true;
        }
    }
    DEBUG_LOG(DBG_SESSION, NULL, "internal:false\n");
    return false;
}

uint8_t dpi_ip4_iptype(uint32_t ip)
{
    int i;
    if (unlikely(th_specialip_subnet4 == NULL)) {
        return DP_IPTYPE_NONE;
    }
    for (i = 0; i < th_specialip_subnet4->count; i++) {

        /*DEBUG_LOG(DBG_SESSION, NULL,
                  "ip="DBG_IPV4_FORMAT" mask="DBG_IPV4_FORMAT"/"DBG_IPV4_FORMAT"\n",
                  DBG_IPV4_TUPLE(ip), DBG_IPV4_TUPLE(th_specialip_subnet4->list[i].ip),
                  DBG_IPV4_TUPLE(th_specialip_subnet4->list[i].mask));*/

        if ((ip & th_specialip_subnet4->list[i].mask) == th_specialip_subnet4->list[i].ip) {
            DEBUG_LOG(DBG_SESSION, NULL, "iptype(%d)\n", th_specialip_subnet4->list[i].iptype);
            return th_specialip_subnet4->list[i].iptype;
        }
    }
    //DEBUG_LOG(DBG_SESSION, NULL, "DP_IPTYPE_NONE\n");
    return DP_IPTYPE_NONE;
}

bool dpi_is_policy_addr(uint32_t ip)
{
    int i;
    if (unlikely(th_policy_addr == NULL)) {
        return false;
    }
    for (i = 0; i < th_policy_addr->count; i++) {
    /*
        DEBUG_LOG(DBG_SESSION, NULL,
                  "ip="DBG_IPV4_FORMAT" mask="DBG_IPV4_FORMAT"/"DBG_IPV4_FORMAT"\n",
                  DBG_IPV4_TUPLE(ip), DBG_IPV4_TUPLE(th_policy_addr->list[i].ip),
                  DBG_IPV4_TUPLE(th_policy_addr->list[i].mask));
    */
        if ((ip ^ th_policy_addr->list[i].ip) == 0) {
            //DEBUG_LOG(DBG_SESSION, NULL, "found in policy address map\n");
            return true;
        }
    }
    DEBUG_LOG(DBG_SESSION, NULL, "unknown:ip\n");
    return false;
}

bool cmp_mac_prefix(void *m1, void *prefix)
{
    if (!m1 || !prefix) return false;
    return *(uint32_t *)m1 == *(uint32_t *)prefix;
}

/**
 * @brief 向指定会话注入TCP RST包（阻断连接）
 * @param sess 目标会话
 * @param to_server true=发送给服务端，false=发送给客户端
 * 
 * 构造并发送TCP RST包，用于Protect模式下阻断违规连接。
 * 注意：TAP模式和ProxyMesh模式下不执行注入。
 */
void dpi_inject_reset_by_session(dpi_session_t *sess, bool to_server)
{
    io_ctx_t ctx;
    struct ethhdr *eth;
    struct iphdr *iph;
    struct tcphdr *tcph;
    uint8_t buffer[sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct tcphdr)];
    dpi_wing_t *c = &sess->client, *s = &sess->server;

    /* TAP模式只监控不干预，ProxyMesh有自己的处理方式 */
    if (FLAGS_TEST(sess->flags, DPI_SESS_FLAG_TAP)) return;
    if (FLAGS_TEST(sess->flags, DPI_SESS_FLAG_PROXYMESH)) return;

    DEBUG_LOG(DBG_SESSION, NULL, "to_server=%d\n", to_server);

    /* 查找端点MAC地址 */
    io_mac_t *mac;
    if (sess->flags & DPI_SESS_FLAG_INGRESS) {
        mac = rcu_map_lookup(&g_ep_map, s->mac);
    } else {
        mac = rcu_map_lookup(&g_ep_map, c->mac);
    }
    if (mac == NULL) return;

    memset(&ctx, 0, sizeof(ctx));

    /* 构造L2层：以太网头 */
    eth = (struct ethhdr *)buffer;
    eth->h_proto = htons(ETH_P_IP);
    uint8_t *uc_mac = mac->ep->ucmac->mac.ether_addr_octet;
    if (sess->flags & DPI_SESS_FLAG_INGRESS) {
        if (to_server) {
            mac_cpy(eth->h_source, c->mac);
            mac_cpy(eth->h_dest, uc_mac);
        } else {
            mac_cpy(eth->h_source, uc_mac);
            mac_cpy(eth->h_dest, c->mac);
        }
    } else {
        if (to_server) {
            mac_cpy(eth->h_source, uc_mac);
            mac_cpy(eth->h_dest, c->mac);
        } else {
            mac_cpy(eth->h_source, c->mac);
            mac_cpy(eth->h_dest, uc_mac);
        }
    }

    /* 构造L3层：IP头 */
    iph = (struct iphdr *)(buffer + sizeof(struct ethhdr));
    iph->version = 4;
    iph->ihl = sizeof(struct iphdr) >> 2;
    iph->tos = 0;
    iph->tot_len = htons(sizeof(struct iphdr) + sizeof(struct tcphdr));
    iph->id = htons((u_int16_t)rand());
    iph->frag_off = htons(0x4000);  /* Don't Fragment标志 */
    iph->ttl = 0xff;
    iph->protocol = IPPROTO_TCP;
    iph->check = 0;
    if (to_server) {
        iph->saddr = c->ip.ip4;
        iph->daddr = s->ip.ip4;
    } else {
        iph->daddr = c->ip.ip4;
        iph->saddr = s->ip.ip4;
    }
    iph->check = get_ip_cksum(iph);
    
    /* 构造L4层：TCP头（RST标志） */
    tcph = (struct tcphdr *)(buffer + sizeof(struct ethhdr) + sizeof(struct iphdr));
    if (to_server) {
        tcph->th_sport = htons(c->port);
        tcph->th_dport = htons(s->port);
        tcph->th_seq = htonl(c->next_seq);
        tcph->th_ack = 0;
        tcph->th_win = 0;
    } else {
        tcph->th_dport = htons(c->port);
        tcph->th_sport = htons(s->port);
        tcph->th_seq = htonl(s->next_seq);
        tcph->th_ack = 0;
        tcph->th_win = 0;
    }
    tcph->th_off = sizeof(struct tcphdr) >> 2;
    tcph->th_x2 = 0;
    tcph->th_flags = TH_RST;  /* RST标志，强制断开连接 */
    tcph->th_sum = 0;
    tcph->th_urp = 0;
    tcph->th_sum = get_l4v4_cksum(iph, tcph, sizeof(struct tcphdr));

    /* 发送RST包 */
    g_io_callback->send_packet(&ctx, buffer, sizeof(buffer));
}

/**
 * @brief 向数据包对应的会话注入TCP RST
 * @param p 数据包上下文
 * @param to_server 发送方向
 */
void dpi_inject_reset(dpi_packet_t *p, bool to_server)
{
    if (unlikely(p->session == NULL)) return;

    dpi_inject_reset_by_session(p->session, to_server);
}

/**
 * @brief 判断NFQ模式下数据包的方向
 * @param p 数据包上下文
 * @return true=入站(ingress)，false=出站(egress)
 * 
 * 判断依据：
 *   1. 目的IP匹配端点IP → 入站
 *   2. 源IP匹配端点IP → 出站
 *   3. 目的端口有应用监听 → 入站
 *   4. 源端口有应用监听 → 出站
 *   5. 默认：目的端口 < 源端口 → 入站
 */
static bool nfq_packet_direction(dpi_packet_t *p)
{
    io_app_t *app = NULL;
    if (p->eth_type == ETH_P_IP) {
        struct iphdr *iph = (struct iphdr *)(p->pkt + p->l3);
        int idx;
        if (p->ep && p->ep->pips) {
            for (idx = 0; idx < p->ep->pips->count; idx++) {
                if ((iph->daddr ^ p->ep->pips->list[idx].ip) == 0) {
                    return true;
                } else if ((iph->saddr ^ p->ep->pips->list[idx].ip) == 0) {
                    return false;
                }
            }
        }
    }
    app = dpi_ep_app_map_lookup(p->ep, p->dport, p->ip_proto);
    if (app != NULL) return true;
    app = dpi_ep_app_map_lookup(p->ep, p->sport, p->ip_proto);
    if (app != NULL) return false;
    return p->dport < p->sport;
}

// return true if packet is ingress to "lo" i/f
static bool proxymesh_packet_direction(dpi_packet_t *p)
{
    io_app_t *app = NULL;
    if (p->eth_type == ETH_P_IP) {//ipv4
        
        struct iphdr *iph = (struct iphdr *)(p->pkt + p->l3);

        if (iph->saddr == iph->daddr) {
            app = dpi_ep_app_map_lookup(p->ep, p->dport, p->ip_proto);
            if (app != NULL) return false;
            app = dpi_ep_app_map_lookup(p->ep, p->sport, p->ip_proto);
            if (app != NULL) return true;
            return p->dport > p->sport;
        } else if (iph->daddr == htonl(INADDR_LOOPBACK) || IS_IN_LOOPBACK(ntohl(iph->daddr))) {
            return true;
        }
    } else {//ipv6
        struct ip6_hdr *ip6h = (struct ip6_hdr *)(p->pkt + p->l3);
        if(memcmp((uint8_t *)(ip6h->ip6_src.s6_addr), (uint8_t *)(ip6h->ip6_dst.s6_addr), 16) == 0) {
            app = dpi_ep_app_map_lookup(p->ep, p->dport, p->ip_proto);
            if (app != NULL) return false;
            app = dpi_ep_app_map_lookup(p->ep, p->sport, p->ip_proto);
            if (app != NULL) return true;
            return p->dport > p->sport;
        } else if (memcmp((uint8_t *)(ip6h->ip6_dst.s6_addr), (uint8_t *)(in6addr_loopback.s6_addr), sizeof(ip6h->ip6_dst.s6_addr)) == 0) {
            return true;
        }
    }
    return false;
}

//return value is only used by nfq, 0 means accept, 1 drop
int dpi_recv_packet(io_ctx_t *ctx, uint8_t *ptr, int len)
{
    int action;
    bool tap = false, inspect = true, isproxymesh = false;
    bool nfq = ctx->nfq;

    th_snap.tick = ctx->tick;

    memset(&th_packet, 0, offsetof(dpi_packet_t, EOZ));

    th_packet.decoded_pkt.len = 0;

    th_packet.pkt = ptr;
    th_packet.cap_len = len;
    th_packet.l2 = 0;

    rcu_read_lock();

    th_internal_subnet4 = g_internal_subnet4;
    th_policy_addr = g_policy_addr;
    th_specialip_subnet4 = g_specialip_subnet4;
    th_xff_enabled = g_xff_enabled;
    th_disable_net_policy = g_disable_net_policy;
    th_detect_unmanaged_wl = g_detect_unmanaged_wl;

    if (likely(th_packet.cap_len >= sizeof(struct ethhdr))) {
        struct ethhdr *eth = (struct ethhdr *)(th_packet.pkt + th_packet.l2);
        io_mac_t *mac = NULL;

        // Lookup workloads
        if (!ctx->tc) {
            // NON-TC mode just fwd the mcast/bcast mac packet
            if (is_mac_m_b_cast(eth->h_dest)) {
                rcu_read_unlock();
                if (!tap && nfq) {
                    //bypass nfq in case of multicast or broadcast
                    return 0;
                }
                g_io_callback->send_packet(ctx, ptr, len);
                return 0;
            }
 
            // in case of quarantine for NON-TC mode we cannot rely on tc rule
            // reset to drop traffic, so we stop send_packet to its peer ctx
            if (ctx->quar) {
                rcu_read_unlock();
                return 1;
            }

            if (mac_cmp(eth->h_source, ctx->ep_mac.ether_addr_octet)) {
                mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
            } else if (mac_cmp(eth->h_dest, ctx->ep_mac.ether_addr_octet)) { 
                mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
                th_packet.flags |= DPI_PKT_FLAG_INGRESS;
            } 
        } else if (cmp_mac_prefix(eth->h_source, MAC_PREFIX)) { 
            mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
        } else if (cmp_mac_prefix(eth->h_dest, MAC_PREFIX)) { 
            mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
            th_packet.flags |= DPI_PKT_FLAG_INGRESS;
        } else
        // For tapped port
        //check dst mac first because src mac may == dst mac for ingress
        if (mac_cmp(eth->h_dest, ctx->ep_mac.ether_addr_octet)) { 
            mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
            th_packet.flags |= DPI_PKT_FLAG_INGRESS;
        } else if (mac_cmp(eth->h_source, ctx->ep_mac.ether_addr_octet)) {
            mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
        }  else if (cmp_mac_prefix(ctx->ep_mac.ether_addr_octet, PROXYMESH_MAC_PREFIX)) {
            /*
             * proxymesh injects its proxy service as a sidecar into POD, 
             * ingress/egress traffic will be redirected to proxy, "lo"
             * interface is monitored to inspect traffic from and to proxy.
             */
            mac = rcu_map_lookup(&g_ep_map, &ctx->ep_mac.ether_addr_octet);
            isproxymesh = true;
            if (th_session4_proxymesh_map.map == NULL) {
                dpi_session_proxymesh_init();
            }
        } else if (nfq) {
            //cilium ep use nfq in protect mode
            mac = rcu_map_lookup(&g_ep_map, &ctx->ep_mac.ether_addr_octet);
        }
        if (likely(mac != NULL)) {
            tap = mac->ep->tap;

            th_packet.ctx = ctx;
            th_packet.ep = mac->ep;
            th_packet.ep_mac = mac->ep->mac->mac.ether_addr_octet;
            th_packet.ep_stats = &mac->ep->stats;
            th_packet.stats = &th_stats;

            IF_DEBUG_LOG(DBG_PACKET, &th_packet) {
                if (FLAGS_TEST(th_packet.flags, DPI_PKT_FLAG_INGRESS)) {
                    DEBUG_LOG_NO_FILTER("pkt_mac="DBG_MAC_FORMAT" ep_mac="DBG_MAC_FORMAT"\n",
                                        DBG_MAC_TUPLE(eth->h_dest), DBG_MAC_TUPLE(*th_packet.ep_mac));
                } else {
                    DEBUG_LOG_NO_FILTER("pkt_mac="DBG_MAC_FORMAT" ep_mac="DBG_MAC_FORMAT"\n",
                                        DBG_MAC_TUPLE(eth->h_source), DBG_MAC_TUPLE(*th_packet.ep_mac));
                }
            }

            if (!isproxymesh && !nfq) {
                if (th_packet.flags & DPI_PKT_FLAG_INGRESS) {
                    th_packet.ep_all_metry = &th_packet.ep_stats->in;
                    th_packet.all_metry = &th_packet.stats->in;
                } else {
                    th_packet.ep_all_metry = &th_packet.ep_stats->out;
                    th_packet.all_metry = &th_packet.stats->out;
                }
    
                if (th_packet.ep_stats->cur_slot != ctx->stats_slot) {
                    dpi_catch_stats_slot(th_packet.ep_stats, ctx->stats_slot);
                }
                if (th_packet.stats->cur_slot != ctx->stats_slot) {
                    dpi_catch_stats_slot(th_packet.stats, ctx->stats_slot);
                }
    
                dpi_inc_stats_packet(&th_packet);
            }
        } else if (g_io_config->promisc) {
            th_packet.ctx = ctx;
            th_packet.flags |= (DPI_PKT_FLAG_INGRESS | DPI_PKT_FLAG_FAKE_EP);
            th_packet.ep = g_io_config->dummy_mac.ep;
            th_packet.ep_mac = g_io_config->dummy_mac.mac.ether_addr_octet;
            th_packet.ep_stats = &g_io_config->dummy_mac.ep->stats;
            th_packet.stats = &th_stats;
            th_packet.ep_all_metry = &th_packet.ep_stats->in;
            th_packet.all_metry = &th_packet.stats->in;
            tap = ctx->tap;
        } else {
            rcu_read_unlock();
            // If not in promisc mode, ignore flooded mac-mismatched pkts 
            //bypass nfq
            return 0;

        }
    }

    // Parse after figuring out direction so that if there is any threat in the packet
    // it can be logged correctly
    action = dpi_parse_ethernet(&th_packet);
    if (unlikely(action == DPI_ACTION_DROP || action == DPI_ACTION_RESET)) {
        rcu_read_unlock();
        if (th_packet.frag_trac != NULL) {
            dpi_frag_discard(th_packet.frag_trac);
        }
        //no drop based on l2 decision because
        //nfq packet's l2 header is fake
        return 0;
    }

    if (isproxymesh || nfq) {
        if (isproxymesh) {
            //direction WRT "lo" i/f is opsite WRT to WL ep
            if (!proxymesh_packet_direction(&th_packet)) {
                th_packet.flags |= DPI_PKT_FLAG_INGRESS;
            }
        } else if (nfq) {
            if (nfq_packet_direction(&th_packet)) {
                th_packet.flags |= DPI_PKT_FLAG_INGRESS;
            }
        }
        if (th_packet.flags & DPI_PKT_FLAG_INGRESS) {
            th_packet.ep_all_metry = &th_packet.ep_stats->in;
            th_packet.all_metry = &th_packet.stats->in;
        } else {
            th_packet.ep_all_metry = &th_packet.ep_stats->out;
            th_packet.all_metry = &th_packet.stats->out;
        }
        
        if (th_packet.ep_stats->cur_slot != ctx->stats_slot) {
            dpi_catch_stats_slot(th_packet.ep_stats, ctx->stats_slot);
        }
        if (th_packet.stats->cur_slot != ctx->stats_slot) {
            dpi_catch_stats_slot(th_packet.stats, ctx->stats_slot);
        }
        
        dpi_inc_stats_packet(&th_packet);
    }
    
    // Bypass broadcast, multicast and non-ip packet
    struct iphdr *iph;
    struct ip6_hdr *ip6h;
    switch (th_packet.eth_type) {
    case ETH_P_IP:
        iph = (struct iphdr *)(th_packet.pkt + th_packet.l3);
        if (INADDR_BROADCAST == ntohl(iph->daddr) || IN_MULTICAST(ntohl(iph->daddr))) {
            inspect = false;
        }
        break;
    case ETH_P_IPV6:
        ip6h = (struct ip6_hdr *)(th_packet.pkt + th_packet.l3);
        if (IN6_IS_ADDR_MULTICAST(&ip6h->ip6_dst)) {
            inspect = false;
        }
        break;
    default:
        inspect = false;
        break;
    }
         
    if (action == DPI_ACTION_NONE && inspect) {
        IF_DEBUG_LOG(DBG_PACKET, &th_packet) {
            debug_dump_packet(&th_packet);
        }
        action = dpi_inspect_ethernet(&th_packet);
        DEBUG_LOG(DBG_PACKET, NULL, "action=%d tap=%d inspect=%d\n",
                  action, tap, inspect);
    }

    rcu_read_unlock();

    if (likely(!tap && action != DPI_ACTION_DROP && action != DPI_ACTION_RESET &&
               action != DPI_ACTION_BLOCK)) {
        if (!tap && nfq) {
            //nfq accept after inspect l4/7 
            return 0;
        }
        if (th_packet.frag_trac != NULL) {
            dpi_frag_send(th_packet.frag_trac, ctx);
        } else {
            g_io_callback->send_packet(ctx, ptr, len);
        }
    } else {
        if (th_packet.frag_trac != NULL) {
            dpi_frag_discard(th_packet.frag_trac);
        }
        if (!tap && nfq) {
            //nfq drop after inspect l4/7 
            return 1;
        }
    }
    return 0;
}

void dpi_timeout(uint32_t tick)
{
    th_snap.tick = tick;

    if (unlikely(!timer_wheel_started(&th_timer))) {
        timer_wheel_start(&th_timer, tick);
    }

    //DEBUG_LOG(DBG_TIMER, NULL, "tick=%u\n", tick);

    rcu_read_lock();
    uint32_t cnt = timer_wheel_roll(&th_timer, tick);
    rcu_read_unlock();

    if (cnt > 0) {
        DEBUG_LOG(DBG_TIMER, NULL, "tick=%u expires=%u\n", tick, cnt);
    }
}
