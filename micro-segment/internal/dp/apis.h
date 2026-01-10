/**
 * @file apis.h
 * @brief DP层核心API定义 - DPI模块与其他模块间的接口
 * 
 * 定义DP层的核心数据结构和API接口：
 *   - IO上下文和回调函数
 *   - 端点(endpoint)和MAC地址映射
 *   - 网络统计和计量结构
 *   - 策略规则和应用识别
 *   - FQDN域名解析
 *   - 控制请求和消息定义
 */

#ifndef __DP_APIS_H__
#define __DP_APIS_H__

//
// Definitions between dpi and other modules.
//

#include <stddef.h>
#include <stdint.h>
#include <stdarg.h>
#include <netinet/ether.h>
#include <arpa/inet.h>
#include <pthread.h>

#include "jansson.h"
#include "urcu/list.h"
#include "defs.h"
#include "utils/rcu_map.h"
#include "utils/bitmap.h"
#include "utils/timer_wheel.h"

#define MAX_THREAD_NAME_LEN 32
extern __thread int THREAD_ID;
extern __thread char THREAD_NAME[MAX_THREAD_NAME_LEN];

/* 控制请求类型枚举 */
enum {
    CTRL_REQ_NONE = 0,
    CTRL_REQ_COUNT_SESSION,     /* 统计会话数量 */
    CTRL_REQ_LIST_SESSION,      /* 列出会话信息 */
    CTRL_REQ_CLEAR_SESSION,     /* 清理会话 */
    CTRL_REQ_LIST_METER,        /* 列出流量计量 */
    CTRL_REQ_DEL_MAC,           /* 删除MAC地址 */
    CTRL_REQ_DUMP_POLICY,       /* 转储策略信息 */
};

/* DLP控制请求类型 */
enum {
    CTRL_DLP_REQ_NONE = 0,
    CTRL_DLP_REQ_BLD,           /* 构建DLP规则 */
    CTRL_DLP_REQ_DEL,           /* 删除DLP规则 */
};

#define MAC_PREFIX "NeuV"                    /* NeuVector MAC前缀 */
#define PROXYMESH_MAC_PREFIX "lkst"          /* ProxyMesh MAC前缀 */
#define IFACE_NAME_LEN 32

/* IP地址联合体，支持IPv4和IPv6 */
typedef union io_ip_ {
    struct in6_addr ip6;
    uint32_t ip4;
} io_ip_t;

/* DP层全局计数器，统计各类数据包和会话信息 */
typedef struct io_counter_ {
    uint64_t pkt_id, err_pkts, unkn_pkts, ipv4_pkts, ipv6_pkts;
    uint64_t tcp_pkts, tcp_nosess_pkts, udp_pkts, icmp_pkts, other_pkts;
    uint64_t drop_pkts, total_asms, freed_asms;
    uint64_t total_frags, tmout_frags, freed_frags;

    uint64_t sess_id, tcp_sess, udp_sess, icmp_sess, ip_sess;
    uint32_t cur_sess, cur_tcp_sess, cur_udp_sess, cur_icmp_sess, cur_ip_sess;

    uint64_t parser_sess[DPI_PARSER_MAX], parser_pkts[DPI_PARSER_MAX];

    uint64_t drop_meters, proxy_meters;
    uint64_t cur_meters, cur_log_caches;
    uint32_t type1_rules, type2_rules, domains, domain_ips;
} io_counter_t;

#define STATS_SLOTS 60          /* 统计环形缓冲区槽数 */
#define STATS_INTERVAL 5        /* 统计间隔（秒） */

/* 流量统计指标，包含环形缓冲区用于时间序列统计 */
typedef struct io_metry_ {
    uint64_t session;           /* 总会话数 */
    uint64_t packet;            /* 总数据包数 */
    uint64_t byte;              /* 总字节数 */
    uint32_t sess_ring[STATS_SLOTS];    /* 会话数环形缓冲区 */
    uint32_t pkt_ring[STATS_SLOTS];     /* 数据包数环形缓冲区 */
    uint32_t byte_ring[STATS_SLOTS];    /* 字节数环形缓冲区 */
    uint32_t cur_session;       /* 当前活跃会话数 */
} io_metry_t;

/* 端点统计信息，区分入站和出站流量 */
typedef struct io_stats_ {
    uint32_t cur_slot;          /* 当前时间槽 */
    io_metry_t in, out;         /* 入站和出站统计 */
    // io_metry_t app_in[DPI_PROTO_MAX];
    // io_metry_t app_out[DPI_PROTO_MAX];
} io_stats_t;

/* 应用识别信息，记录端口对应的服务和应用类型 */
typedef struct io_app_ {
    struct cds_lfht_node node;
    uint16_t port;              /* 端口号 */
    uint16_t proto;             /* 协议类型 */
    uint16_t server;            /* 服务器类型（如nginx、apache） */
    uint16_t application;       /* 应用类型（如web、database） */
#define SERVER_VER_SIZE 32
    char version[SERVER_VER_SIZE];  /* 服务器版本信息 */
    bool listen;                /* 是否为监听端口 */
    uint8_t ip_proto;           /* IP协议号（TCP=6, UDP=17） */
#define APP_SRC_CTRL  1         /* 来源：Controller配置 */
#define APP_SRC_DP    2         /* 来源：DP层发现 */
    uint8_t src;                /* 应用信息来源 */
} io_app_t;

/* 端点IP信息 */
typedef struct io_pip_ {
    uint32_t ip;
} io_pip_t;

/* 内部IP列表，用于ProxyMesh等场景 */
typedef struct io_internal_pip_ {
    int count;
    io_pip_t list[0];
} io_internal_pip_t;

#define DLP_RULETYPE_INSIDE "inside"
#define DLP_RULETYPE_OUTSIDE "outside"
#define WAF_RULETYPE_INSIDE "wafinside"
#define WAF_RULETYPE_OUTSIDE "wafoutside"
/* 工作负载端点定义，包含网络接口、MAC地址、统计信息和策略配置 */
typedef struct io_ep_ {
    char iface[IFACE_NAME_LEN]; /* 网络接口名称 */
    struct io_mac_ *mac;        /* 原始MAC地址 */
    struct io_mac_ *ucmac;      /* 单播MAC地址 */
    struct io_mac_ *bcmac;      /* 广播MAC地址 */
    struct ether_addr pmac;     /* ProxyMesh原始MAC */
    io_internal_pip_t *pips;    /* ProxyMesh父级IP列表 */

    uint32_t COPY_START;

    io_stats_t stats;           /* 流量统计信息 */

    rcu_map_t app_map;          /* 应用映射表 */
    uint32_t app_updated;       /* 应用信息更新标志 */
    uint16_t app_ports;         /* 应用端口数量 */

    bool tap;                   /* TAP模式标志 */
    uint8_t cassandra_svr: 1,   /* 各种服务器/客户端标识位 */
            kafka_svr:     1,
            couchbase_svr: 1,
            couchbase_clt: 1,
            zookeeper_svr: 1,
            zookeeper_clt: 1;
    void *policy_hdl;           /* 策略句柄 */
    uint16_t policy_ver;        /* 策略版本 */

    rcu_map_t dlp_cfg_map;      /* DLP配置映射 */
    rcu_map_t waf_cfg_map;      /* WAF配置映射 */
    rcu_map_t dlp_rid_map;      /* DLP规则ID映射 */
    rcu_map_t waf_rid_map;      /* WAF规则ID映射 */
    void *dlp_detector;         /* DLP检测器 */
    uint16_t dlp_detect_ver;    /* DLP检测版本 */
    bool dlp_inside;            /* DLP内部标志 */
    bool waf_inside;            /* WAF内部标志 */
    bool nbe;                   /* NBE标志 */
} io_ep_t;

/* MAC地址映射结构，关联MAC地址与端点 */
typedef struct io_mac_ {
    struct cds_lfht_node node;
    struct ether_addr mac;      /* MAC地址 */
    io_ep_t *ep;                /* 关联的端点 */
    uint8_t broadcast:1,        /* 广播地址标志 */
            unicast:  1;        /* 单播地址标志 */
} io_mac_t;

/* IPv4子网定义 */
typedef struct io_subnet4_ {
    uint32_t ip;                /* 网络地址 */
    uint32_t mask;              /* 子网掩码 */
} io_subnet4_t;

/* 内部IPv4子网列表 */
typedef struct io_internal_subnet4_ {
    int count;
    io_subnet4_t list[0];
} io_internal_subnet4_t;

/* 特殊IP类型定义 */
#define SPEC_INTERNAL_TUNNELIP "tunnelip"   /* 隧道IP */
#define SPEC_INTERNAL_SVCIP "svcip"         /* 服务IP */
#define SPEC_INTERNAL_HOSTIP "hostip"       /* 主机IP */
#define SPEC_INTERNAL_DEVIP "devip"         /* 设备IP */
#define SPEC_INTERNAL_UWLIP "uwlip"         /* 未管理工作负载IP */
#define SPEC_INTERNAL_EXTIP "extip"         /* 外部IP */

/* 特殊IP类型枚举 */
enum {
    DP_IPTYPE_NONE = 0,
    DP_IPTYPE_TUNNELIP,
    DP_IPTYPE_SVCIP,
    DP_IPTYPE_HOSTIP,
    DP_IPTYPE_DEVIP,
    DP_IPTYPE_UWLIP,
    DP_IPTYPE_EXTIP,
};

/* 特殊IPv4子网定义，包含IP类型信息 */
typedef struct io_spec_subnet4_ {
    uint32_t ip;
    uint32_t mask;
    uint8_t iptype;             /* IP类型 */
} io_spec_subnet4_t;

/* 特殊内部IPv4子网列表 */
typedef struct io_spec_internal_subnet4_ {
    int count;
    io_spec_subnet4_t list[0];
} io_spec_internal_subnet4_t;

/* IO上下文，包含数据包处理的环境信息 */
typedef struct io_ctx_ {
    void *dp_ctx;               /* DP上下文指针 */
    uint32_t tick;              /* 时间戳 */
    uint32_t stats_slot;        /* 统计时间槽 */
    struct ether_addr ep_mac;   /* 端点MAC地址 */
    bool large_frame;           /* 大帧支持标志 */
    bool tap;                   /* TAP模式标志 */
    bool tc;                    /* TC模式标志 */
    bool quar;                  /* 隔离模式标志 */
    bool nfq;                   /* NFQUEUE模式标志 */
} io_ctx_t;

/* IO回调函数集合，定义DP层与外部模块的接口 */
typedef struct io_callback_ {
    int (*debug) (bool print_ts, const char *fmt, va_list args);        /* 调试输出 */
    int (*send_packet) (io_ctx_t *ctx, uint8_t *data, int len);         /* 发送数据包 */
    int (*send_ctrl_json) (json_t *root);                               /* 发送JSON控制消息 */
    int (*send_ctrl_binary) (void *buf, int len);                       /* 发送二进制控制消息 */
    int (*threat_log) (DPMsgThreatLog *log);                            /* 威胁日志回调 */
    int (*traffic_log) (DPMsgSession *log);                             /* 流量日志回调 */
    int (*connect_report) (DPMsgSession *log, DPMonitorMetric *metric, int count_session, int count_violate); /* 连接报告回调 */
} io_callback_t;

/* DPI配置结构 */
typedef struct dpi_config_ {
    bool enable_cksum;          /* 启用校验和验证 */
    bool promisc;               /* 混杂模式 */
    bool thrt_ssl_tls_1dot0;    /* SSL/TLS 1.0威胁检测 */
    bool thrt_ssl_tls_1dot1;    /* SSL/TLS 1.1威胁检测 */

    io_mac_t dummy_mac;         /* 虚拟MAC地址 */
    io_ep_t dummy_ep;           /* 虚拟端点 */
} io_config_t;

#define DPI_INIT 0

typedef void (*dpi_stats_callback_fct)(io_stats_t *stats, io_stats_t *s);

// in
void dpi_setup(io_callback_t *cb, io_config_t *cfg);
void dpi_init(int reason);
int dpi_recv_packet(io_ctx_t *context, uint8_t *pkt, int len);
void dpi_timeout(uint32_t tick);

void dpi_handle_ctrl_req(int req, io_ctx_t *context);
void dpi_handle_dlp_ctrl_req(int req);
void dpi_get_device_counter(DPMsgDeviceCounter *c);
void dpi_count_session(DPMsgSessionCount *c);
void dpi_get_stats(io_stats_t *stats, dpi_stats_callback_fct cb);


#define GET_EP_FROM_MAC_MAP(buf)  (io_ep_t *)(buf + sizeof(io_mac_t) * 3)
typedef struct dpi_policy_app_rule_ {
    uint32_t rule_id;
    uint32_t app;
    uint8_t action;
} dpi_policy_app_rule_t;

#define MAX_FQDN_LEN DP_POLICY_FQDN_NAME_MAX_LEN
typedef struct dpi_policy_rule_ {
    uint32_t id;
    uint32_t sip;
    uint32_t sip_r;
    uint32_t dip;
    uint32_t dip_r;
    uint16_t dport;
    uint16_t dport_r;
    uint16_t proto;
    uint8_t action;
    bool ingress;
    bool vh;
    char fqdn[MAX_FQDN_LEN];
    uint32_t num_apps;
    dpi_policy_app_rule_t *app_rules;
} dpi_policy_rule_t;

typedef struct dpi_policy_ {
    int num_macs;
    struct ether_addr *mac_list;
    int def_action;
    int apply_dir;
    int num_rules;
    dpi_policy_rule_t *rule_list;
} dpi_policy_t;

int dpi_policy_cfg(int cmd, dpi_policy_t *policy, int flag);
void dp_policy_destroy(void *policy_hdl);
void dpi_fqdn_entry_mark_delete(const char *name);
void dpi_fqdn_entry_delete_marked();

/*
 * -----------------------------------------------------
 * --- FQDN definition ---------------------------------
 * -----------------------------------------------------
 */
typedef struct fqdn_record_ {
    char name[MAX_FQDN_LEN];
    uint32_t code;
    uint32_t flag;
#define FQDN_RECORD_TO_DELETE      0x00000001
#define FQDN_RECORD_DELETED        0x00000002
#define FQDN_RECORD_WILDCARD       0x00000004
    uint32_t ip_cnt;
    uint32_t record_updated;//used for wildcard fqdn
    struct cds_list_head iplist;//FQDN->IP(s) mapping
    bool vh;
} fqdn_record_t;

typedef struct fqdn_record_item_ {
    struct cds_list_head node;
    fqdn_record_t *r;
} fqdn_record_item_t;

typedef struct fqdn_name_entry_ {
    struct cds_lfht_node node;
    fqdn_record_t *r;
} fqdn_name_entry_t;

typedef struct fqdn_ipv4_entry_ {
    struct cds_lfht_node node;
    uint32_t ip;
    struct cds_list_head rlist;//IP->FQDN(s) mapping
} fqdn_ipv4_entry_t;

typedef struct fqdn_ipv4_item_ {
    struct cds_list_head node;
    uint32_t ip;
} fqdn_ipv4_item_t;

#define DPI_FQDN_DELETE_QLEN      32
#define DPI_FQDN_MAX_ENTRIES      DP_POLICY_FQDN_MAX_ENTRIES
typedef struct dpi_fqdn_hdl_ {
    rcu_map_t fqdn_name_map;
    rcu_map_t fqdn_ipv4_map;
    bitmap *bm;
    int code_cnt;
    int del_name_cnt;
    int del_ipv4_cnt;
    fqdn_name_entry_t *del_name_list[DPI_FQDN_DELETE_QLEN];
    fqdn_ipv4_entry_t *del_ipv4_list[DPI_FQDN_DELETE_QLEN];
    struct cds_list_head del_rlist;
} dpi_fqdn_hdl_t;

typedef struct fqdn_iter_ctx_ {
    dpi_fqdn_hdl_t *hdl;
    bool more;
} fqdn_iter_ctx_t;

uint32_t config_fqdn_ipv4_mapping(dpi_fqdn_hdl_t *hdl, char *name, uint32_t ip, bool vh);

/*
 * -----------------------------------------
 * --- ip-fqdn storage definition ----------
 * -----------------------------------------
 */
#define IP_FQDN_STORAGE_ENTRY_TIMEOUT 1800 //sec
typedef struct dpi_ip_fqdn_storage_record_ {
    uint32_t ip;
    char     name[MAX_FQDN_LEN];
    uint32_t record_updated;
} dpi_ip_fqdn_storage_record_t;

typedef struct dpi_ip_fqdn_storage_entry_ {
    struct cds_lfht_node node;
    timer_entry_t ts_entry;

    dpi_ip_fqdn_storage_record_t *r;
} dpi_ip_fqdn_storage_entry_t;

//dlp
#define MAX_DLP_RULE_NAME_LEN DP_DLP_RULE_NAME_MAX_LEN
#define MAX_DLP_RULE_PATTERN_LEN DP_DLP_RULE_PATTERN_MAX_LEN
#define MAX_DLPCFG_DELETE 256

typedef struct dpi_dlp_rule_pattern_ {
    char rule_pattern[MAX_DLP_RULE_PATTERN_LEN];
} dpi_dlp_rule_pattern_t;

typedef struct dpi_dlp_rule_entry_ {
    char rulename[MAX_DLP_RULE_NAME_LEN];
    uint32_t sigid;
    int num_dlp_rule_pats;
    dpi_dlp_rule_pattern_t *dlp_rule_pat_list;
} dpi_dlp_rule_entry_t;

typedef struct io_dlp_cfg_ {
    struct cds_lfht_node node;
    uint32_t sigid;
    uint8_t action;
    bool enable;
    struct cds_list_head sig_user_list;
} io_dlp_cfg_t;

typedef struct io_dlp_ruleid_ {
    struct cds_lfht_node node;
    uint32_t rid;
    bool enable;
} io_dlp_ruleid_t;

typedef struct dpi_dlpbld_ {
    int num_macs;
    struct ether_addr *mac_list;
    int num_del_macs;
    struct ether_addr *del_mac_list;
    int apply_dir;
    int num_dlp_rules;
    dpi_dlp_rule_entry_t *dlp_rule_list;
} dpi_dlpbld_t;

typedef struct dpi_dlpbld_mac_ {
    int num_old_macs;
    struct ether_addr *old_mac_list;
    int num_del_macs;
    struct ether_addr *del_mac_list;
    int num_add_macs;
    struct ether_addr *add_mac_list;
} dpi_dlpbld_mac_t;

int dpi_sig_bld(dpi_dlpbld_t *dlpsig, int flag);
int dpi_sig_bld_update_mac(dpi_dlpbld_mac_t *dlpbld_mac);
void dp_dlp_destroy(void *dlp_detector);

#define CTRL_REQ_TIMEOUT 4
#define CTRL_DLP_REQ_TIMEOUT 2
extern pthread_cond_t g_ctrl_req_cond;
extern pthread_mutex_t g_ctrl_req_lock;
extern int dp_data_wait_ctrl_req_thr(int req, int thr_id);
extern pthread_cond_t g_dlp_ctrl_req_cond;
extern pthread_mutex_t g_dlp_ctrl_req_lock;
extern int dp_dlp_wait_ctrl_req_thr(int req);
extern void dp_ctrl_release_ip_fqdn_storage(dpi_ip_fqdn_storage_entry_t *entry);

#endif
