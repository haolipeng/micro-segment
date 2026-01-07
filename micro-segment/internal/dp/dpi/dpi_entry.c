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

/* DPI全局初始化，设置IO回调和配置 */
void dpi_setup(io_callback_t *cb, io_config_t *cfg)
{
    g_io_callback = cb;
    g_io_config = cfg;

    dpi_packet_setup();   /* 初始化数据包处理相关结构 */
    dpi_parser_setup();   /* 初始化协议解析器 */
}

/* DPI线程初始化，分配数据包缓冲区并初始化各子模块 */
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

/* 在端点应用映射表中查找指定端口和协议的应用 */
io_app_t *dpi_ep_app_map_lookup(io_ep_t *ep, uint16_t port, uint8_t ip_proto)
{
    io_app_t key;

    key.port = port;
    key.ip_proto = ip_proto;
    return rcu_map_lookup(&ep->app_map, &key);
}

/* 查找或创建端点的应用映射条目，不存在时自动创建 */
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

/* 设置会话的应用层协议，仅对入站会话生效 */
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

/* 获取会话的应用类型标识 */
uint16_t dpi_ep_get_app(dpi_packet_t *p)
{
    dpi_session_t *s = p->session;

    if (!FLAGS_TEST(s->flags, DPI_SESS_FLAG_INGRESS)) return 0;

    io_app_t *app = ep_app_map_locate(p->ep, s->server.port, s->ip_proto);
    if (app == NULL) return 0;

    return app->application;
}

/* 设置会话的服务器类型和应用类型，由DPI解析器调用 */
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

/* 设置服务器版本信息到应用映射条目 */
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

/* 打印内部子网、特殊IP和策略地址配置到日志文件 */
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

/* 判断IPv4地址是否为内部地址，检查回环地址和内部子网列表 */
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

/* 获取IPv4地址的特殊IP类型（如云厂商、CDN等） */
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

/* 判断IP是否在策略地址映射表中 */
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

/* 比较MAC地址前缀是否匹配 */
bool cmp_mac_prefix(void *m1, void *prefix)
{
    if (!m1 || !prefix) return false;
    return *(uint32_t *)m1 == *(uint32_t *)prefix;
}

/* 向指定会话注入TCP RST包阻断连接，TAP和ProxyMesh模式下不执行 */
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

/* 向数据包对应的会话注入TCP RST */
void dpi_inject_reset(dpi_packet_t *p, bool to_server)
{
    if (unlikely(p->session == NULL)) return;

    dpi_inject_reset_by_session(p->session, to_server);
}

/* 判断NFQ模式下数据包方向，通过IP匹配和端口应用判断入站/出站 */
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

/* 判断ProxyMesh模式下lo接口数据包的方向 */
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

/* 数据包接收主入口，执行端点查找、方向判断、DPI检测和策略决策
 * 支持TC/NFQ/TAP/ProxyMesh模式，NFQ模式返回0=放行/1=丢弃 */
int dpi_recv_packet(io_ctx_t *ctx, uint8_t *ptr, int len)
{
    int action;
    bool tap = false, inspect = true, isproxymesh = false;
    bool nfq = ctx->nfq;

    th_snap.tick = ctx->tick;

    /* 清空线程私有的数据包结构（保留缓冲区指针） */
    memset(&th_packet, 0, offsetof(dpi_packet_t, EOZ));

    th_packet.decoded_pkt.len = 0;

    th_packet.pkt = ptr;
    th_packet.cap_len = len;
    th_packet.l2 = 0;

    rcu_read_lock();

    /* 获取线程本地的配置副本（RCU保护） */
    th_internal_subnet4 = g_internal_subnet4;
    th_policy_addr = g_policy_addr;
    th_specialip_subnet4 = g_specialip_subnet4;
    th_xff_enabled = g_xff_enabled;
    th_disable_net_policy = g_disable_net_policy;
    th_detect_unmanaged_wl = g_detect_unmanaged_wl;

    /* 解析以太网头，查找工作负载端点 */
    if (likely(th_packet.cap_len >= sizeof(struct ethhdr))) {
        struct ethhdr *eth = (struct ethhdr *)(th_packet.pkt + th_packet.l2);
        io_mac_t *mac = NULL;

        /* 根据不同模式查找端点 */
        if (!ctx->tc) {
            /* 非TC模式：直接转发广播/组播包 */
            if (is_mac_m_b_cast(eth->h_dest)) {
                rcu_read_unlock();
                if (!tap && nfq) {
                    return 0;  /* NFQ模式放行 */
                }
                g_io_callback->send_packet(ctx, ptr, len);
                return 0;
            }
 
            /* 隔离模式下丢弃流量 */
            if (ctx->quar) {
                rcu_read_unlock();
                return 1;
            }

            /* 通过MAC地址查找端点并判断方向 */
            if (mac_cmp(eth->h_source, ctx->ep_mac.ether_addr_octet)) {
                mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
            } else if (mac_cmp(eth->h_dest, ctx->ep_mac.ether_addr_octet)) { 
                mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
                th_packet.flags |= DPI_PKT_FLAG_INGRESS;
            } 
        } else if (cmp_mac_prefix(eth->h_source, MAC_PREFIX)) { 
            /* TC模式：通过MAC前缀识别NeuVector管理的接口 */
            mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
        } else if (cmp_mac_prefix(eth->h_dest, MAC_PREFIX)) { 
            mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
            th_packet.flags |= DPI_PKT_FLAG_INGRESS;
        } else
        /* TAP模式：通过上下文中的MAC地址匹配 */
        if (mac_cmp(eth->h_dest, ctx->ep_mac.ether_addr_octet)) { 
            mac = rcu_map_lookup(&g_ep_map, &eth->h_dest);
            th_packet.flags |= DPI_PKT_FLAG_INGRESS;
        } else if (mac_cmp(eth->h_source, ctx->ep_mac.ether_addr_octet)) {
            mac = rcu_map_lookup(&g_ep_map, &eth->h_source);
        }  else if (cmp_mac_prefix(ctx->ep_mac.ether_addr_octet, PROXYMESH_MAC_PREFIX)) {
            /* ProxyMesh模式：监控lo接口的sidecar流量 */
            mac = rcu_map_lookup(&g_ep_map, &ctx->ep_mac.ether_addr_octet);
            isproxymesh = true;
            if (th_session4_proxymesh_map.map == NULL) {
                dpi_session_proxymesh_init();
            }
        } else if (nfq) {
            /* NFQ模式（Cilium等场景） */
            mac = rcu_map_lookup(&g_ep_map, &ctx->ep_mac.ether_addr_octet);
        }

        /* 找到端点，初始化数据包上下文 */
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

            /* 更新流量统计（非ProxyMesh和NFQ模式） */
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
            /* 混杂模式：使用虚拟端点处理未知流量 */
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
            /* 非混杂模式忽略未知MAC的数据包 */
            return 0;

        }
    }

    /* 解析以太网层协议 */
    action = dpi_parse_ethernet(&th_packet);
    if (unlikely(action == DPI_ACTION_DROP || action == DPI_ACTION_RESET)) {
        rcu_read_unlock();
        if (th_packet.frag_trac != NULL) {
            dpi_frag_discard(th_packet.frag_trac);
        }
        return 0;
    }

    /* ProxyMesh和NFQ模式需要重新判断方向 */
    if (isproxymesh || nfq) {
        if (isproxymesh) {
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
    
    /* 跳过广播、组播和非IP数据包的深度检测 */
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
         
    /* 执行DPI深度检测 */
    if (action == DPI_ACTION_NONE && inspect) {
        IF_DEBUG_LOG(DBG_PACKET, &th_packet) {
            debug_dump_packet(&th_packet);
        }
        action = dpi_inspect_ethernet(&th_packet);  /* 核心检测逻辑 */
        DEBUG_LOG(DBG_PACKET, NULL, "action=%d tap=%d inspect=%d\n",
                  action, tap, inspect);
    }

    rcu_read_unlock();

    /* 根据检测结果处理数据包 */
    if (likely(!tap && action != DPI_ACTION_DROP && action != DPI_ACTION_RESET &&
               action != DPI_ACTION_BLOCK)) {
        /* 放行：转发数据包 */
        if (!tap && nfq) {
            return 0;  /* NFQ模式返回0表示放行 */
        }
        if (th_packet.frag_trac != NULL) {
            dpi_frag_send(th_packet.frag_trac, ctx);
        } else {
            g_io_callback->send_packet(ctx, ptr, len);
        }
    } else {
        /* 丢弃或阻断 */
        if (th_packet.frag_trac != NULL) {
            dpi_frag_discard(th_packet.frag_trac);
        }
        if (!tap && nfq) {
            return 1;  /* NFQ模式返回1表示丢弃 */
        }
    }
    return 0;
}

/* 定时器处理函数，驱动定时器轮转动，触发会话超时清理等定时任务 */
void dpi_timeout(uint32_t tick)
{
    th_snap.tick = tick;

    /* 首次调用时启动定时器轮 */
    if (unlikely(!timer_wheel_started(&th_timer))) {
        timer_wheel_start(&th_timer, tick);
    }

    rcu_read_lock();
    uint32_t cnt = timer_wheel_roll(&th_timer, tick);  /* 转动定时器轮 */
    rcu_read_unlock();

    if (cnt > 0) {
        DEBUG_LOG(DBG_TIMER, NULL, "tick=%u expires=%u\n", tick, cnt);
    }
}
