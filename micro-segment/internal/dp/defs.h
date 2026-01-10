/**
 * @file defs.h
 * @brief 基础定义文件 - Agent和Controller共享的常量定义
 * 
 * 定义Agent和DP进程间共享的常量和枚举：
 *   - DPI动作类型和威胁严重级别
 *   - 会话状态和应用协议标识
 *   - 威胁检测ID和解析器类型
 *   - 消息类型和数据结构
 *   - 会话标志位定义
 */

#ifndef __DEFS_H__
#define __DEFS_H__

#include <stdint.h>
#include <netinet/tcp.h>

// Definitions are used by both agent and controller, value cannot be changed.

#define DP_MSG_SIZE 8192

/* DPI处理动作定义 */
#define DPI_ACTION_NONE   0     /* 无动作 */
#define DPI_ACTION_ALLOW  1     /* 允许通过 */
#define DPI_ACTION_DROP   2     /* 丢弃数据包 */
#define DPI_ACTION_RESET  3     /* 发送RST重置连接 */
#define DPI_ACTION_BYPASS 4     /* 绕过后续检测 */
#define DPI_ACTION_BLOCK  5     /* 阻断会话 */
#define DPI_ACTION_MAX    6

/* 威胁严重级别定义 */
#define THRT_SEVERITY_INFO     1    /* 信息级别 */
#define THRT_SEVERITY_LOW      2    /* 低危 */
#define THRT_SEVERITY_MEDIUM   3    /* 中危 */
#define THRT_SEVERITY_HIGH     4    /* 高危 */
#define THRT_SEVERITY_CRITICAL 5    /* 严重 */
#define THRT_SEVERITY_MAX      6

/* TCP会话状态定义（对应标准TCP状态） */
#define SESS_STATE_NONE        0
#define SESS_STATE_ESTABLISHED TCP_ESTABLISHED
#define SESS_STATE_SYN_SENT    TCP_SYN_SENT
#define SESS_STATE_SYN_RECV    TCP_SYN_RECV
#define SESS_STATE_FIN_WAIT1   TCP_FIN_WAIT1
#define SESS_STATE_FIN_WAIT2   TCP_FIN_WAIT2
#define SESS_STATE_TIME_WAIT   TCP_TIME_WAIT
#define SESS_STATE_CLOSE       TCP_CLOSE
#define SESS_STATE_CLOSE_WAIT  TCP_CLOSE_WAIT
#define SESS_STATE_LAST_ACK    TCP_LAST_ACK
#define SESS_STATE_LISTEN      TCP_LISTEN
#define SESS_STATE_CLOSING     TCP_CLOSING

/* 基础应用协议标识 */
#define DPI_APP_BASE_START            DPI_APP_HTTP
#define DPI_APP_HTTP                  1001
#define DPI_APP_SSL                   1002
#define DPI_APP_SSH                   1003
#define DPI_APP_DNS                   1004
#define DPI_APP_DHCP                  1005
#define DPI_APP_NTP                   1006
#define DPI_APP_TFTP                  1007
#define DPI_APP_ECHO                  1008
#define DPI_APP_RTSP                  1009
#define DPI_APP_SIP                   1010

/* 应用服务协议标识 */
#define DPI_APP_PROTO_MARK            DPI_APP_MYSQL
#define DPI_APP_MYSQL                 2001
#define DPI_APP_REDIS                 2002
#define DPI_APP_ZOOKEEPER             2003
#define DPI_APP_CASSANDRA             2004
#define DPI_APP_MONGODB               2005
#define DPI_APP_POSTGRESQL            2006
#define DPI_APP_KAFKA                 2007
#define DPI_APP_COUCHBASE             2008
#define DPI_APP_WORDPRESS             2009
#define DPI_APP_ACTIVEMQ              2010
#define DPI_APP_COUCHDB               2011
#define DPI_APP_ELASTICSEARCH         2012
#define DPI_APP_MEMCACHED             2013
#define DPI_APP_RABBITMQ              2014
#define DPI_APP_RADIUS                2015
#define DPI_APP_VOLTDB                2016
#define DPI_APP_CONSUL                2017
#define DPI_APP_SYSLOG                2018
#define DPI_APP_ETCD                  2019
#define DPI_APP_SPARK                 2020
#define DPI_APP_APACHE                2021
#define DPI_APP_NGINX                 2022
#define DPI_APP_JETTY                 2023
#define DPI_APP_NODEJS                2024
#define DPI_APP_ERLANG_EPMD           2025 /* Erlang端口映射守护进程 */
#define DPI_APP_TNS                   2026 /* Oracle TNS */
#define DPI_APP_TDS                   2027 /* Microsoft TDS */
#define DPI_APP_GRPC                  2028
#define DPI_APP_MAX                   2029

#define DPI_APP_UNKNOWN               0    /* 未知应用 */
#define DPI_APP_NOT_CHECKED           1    /* 未检测（仅用于报告） */

/* 协议解析器类型定义 */
#define DPI_PARSER_HTTP               0
#define DPI_PARSER_SSL                1
#define DPI_PARSER_SSH                2
#define DPI_PARSER_DNS                3
#define DPI_PARSER_DHCP               4
#define DPI_PARSER_NTP                5
#define DPI_PARSER_TFTP               6
#define DPI_PARSER_ECHO               7
#define DPI_PARSER_MYSQL              8
#define DPI_PARSER_REDIS              9
#define DPI_PARSER_ZOOKEEPER          10
#define DPI_PARSER_CASSANDRA          11
#define DPI_PARSER_MONGODB            12
#define DPI_PARSER_POSTGRESQL         13
#define DPI_PARSER_KAFKA              14
#define DPI_PARSER_COUCHBASE          15
#define DPI_PARSER_SPARK              16
#define DPI_PARSER_TNS                17
#define DPI_PARSER_TDS                18
#define DPI_PARSER_GRPC               19
#define DPI_PARSER_MAX                20

/* 基于流量的威胁检测ID */
#define THRT_ID_SYN_FLOOD       1001    /* SYN洪水攻击 */
#define THRT_ID_ICMP_FLOOD      1002    /* ICMP洪水攻击 */
#define THRT_ID_IP_SRC_SESSION  1003    /* IP源会话异常 */

/* 基于模式的威胁检测ID */
#define THRT_ID_BAD_PACKET           2001    /* 恶意数据包 */
#define THRT_ID_IP_TEARDROP          2002    /* IP分片攻击 */
#define THRT_ID_TCP_SYN_DATA         2003    /* TCP SYN数据攻击 */
#define THRT_ID_TCP_SPLIT_HDSHK      2004    /* TCP分割握手攻击 */
#define THRT_ID_TCP_NODATA           2005    /* TCP无数据攻击 */
#define THRT_ID_PING_DEATH           2006    /* Ping死亡攻击 */
#define THRT_ID_DNS_LOOP_PTR         2007    /* DNS循环指针攻击 */
#define THRT_ID_SSH_VER_1            2008    /* SSH版本1漏洞 */
#define THRT_ID_SSL_HEARTBLEED       2009    /* SSL心脏滴血漏洞 */
#define THRT_ID_SSL_CIPHER_OVF       2010    /* SSL密码溢出 */
#define THRT_ID_SSL_VER_2OR3         2011    /* SSL版本2/3漏洞 */
#define THRT_ID_SSL_TLS_1DOT0        2012    /* TLS 1.0漏洞 */
#define THRT_ID_HTTP_NEG_LEN         2013    /* HTTP负长度攻击 */
#define THRT_ID_HTTP_SMUGGLING       2014    /* HTTP走私攻击 */
#define THRT_ID_HTTP_SLOWLORIS       2015    /* HTTP慢速攻击 */
#define THRT_ID_TCP_SMALL_WINDOW     2016    /* TCP小窗口攻击 */
#define THRT_ID_DNS_OVERFLOW         2017    /* DNS溢出攻击 */
#define THRT_ID_MYSQL_ACCESS_DENY    2018    /* MySQL访问拒绝 */
#define THRT_ID_DNS_ZONE_TRANSFER    2019    /* DNS区域传输攻击 */
#define THRT_ID_ICMP_TUNNELING       2020    /* ICMP隧道攻击 */
#define THRT_ID_DNS_TYPE_NULL        2021    /* DNS空类型攻击 */
#define THRT_ID_SQL_INJECTION        2022    /* SQL注入攻击 */
#define THRT_ID_APACHE_STRUTS_RCE    2023    /* Apache Struts RCE */
#define THRT_ID_DNS_TUNNELING        2024    /* DNS隧道攻击 */
#define THRT_ID_TCP_SMALL_MSS        2025    /* TCP小MSS攻击 */
#define THRT_ID_K8S_EXTIP_MITM       2026    /* K8s外部IP中间人攻击 */
#define THRT_ID_SSL_TLS_1DOT1        2027    /* TLS 1.1漏洞 */
#define THRT_ID_MAX                  2028


/* DP消息类型定义 */
#define DP_KIND_APP_UPDATE              1    /* 应用更新消息 */
#define DP_KIND_SESSION_LIST            2    /* 会话列表消息 */
#define DP_KIND_SESSION_COUNT           3    /* 会话计数消息 */
#define DP_KIND_DEVICE_COUNTER          4    /* 设备计数器消息 */
#define DP_KIND_METER_LIST              5    /* 流量计量列表消息 */
#define DP_KIND_THREAT_LOG              6    /* 威胁日志消息 */
#define DP_KIND_CONNECTION              7    /* 连接消息 */
#define DP_KIND_MAC_STATS               8    /* MAC统计消息 */
#define DP_KIND_DEVICE_STATS            9    /* 设备统计消息 */
#define DP_KIND_KEEP_ALIVE              10   /* 心跳保活消息 */
#define DP_KIND_FQDN_UPDATE             11   /* FQDN更新消息 */
#define DP_KIND_IP_FQDN_STORAGE_UPDATE  12   /* IP-FQDN存储更新 */
#define DP_KIND_IP_FQDN_STORAGE_RELEASE 13   /* IP-FQDN存储释放 */

/* DP消息头结构 */
typedef struct {
    uint8_t  Kind;      /* 消息类型 */
    uint8_t  More;      /* 是否有更多消息 */
    uint16_t Length;    /* 消息长度（包含头部） */
} DPMsgHdr;

/* 应用信息消息结构 */
typedef struct {
    uint16_t Port;          /* 端口号 */
    uint16_t Proto;         /* 协议类型 */
    uint16_t Server;        /* 服务器类型 */
    uint16_t Application;   /* 应用类型 */
    uint8_t  IPProto;       /* IP协议号 */
} DPMsgApp;

/* 应用消息头结构 */
typedef struct {
    uint8_t  MAC[6];        /* MAC地址 */
    uint16_t Ports;         /* 端口数量 */
    // DPMsgApp Apps[0];    /* 应用列表 */
} DPMsgAppHdr;

/* 会话计数消息结构 */
typedef struct {
    uint32_t CurSess;       /* 当前总会话数 */
    uint32_t CurTCPSess;    /* 当前TCP会话数 */
    uint32_t CurUDPSess;    /* 当前UDP会话数 */
    uint32_t CurICMPSess;   /* 当前ICMP会话数 */
    uint32_t CurIPSess;     /* 当前IP会话数 */
} DPMsgSessionCount;

/* 会话标志位定义 */
#define DPSESS_FLAG_INGRESS       0x0001    /* 入站会话 */
#define DPSESS_FLAG_TAP           0x0002    /* TAP模式会话 */
#define DPSESS_FLAG_MID           0x0004    /* 中间状态会话 */
#define DPSESS_FLAG_EXTERNAL      0x0008    /* 外部对等端 */
#define DPSESS_FLAG_XFF           0x0010    /* 虚拟XFF连接 */
#define DPSESS_FLAG_SVC_EXTIP     0x0020    /* 服务外部IP */
#define DPSESS_FLAG_MESH_TO_SVR   0x0040    /* 网格到服务器流量 */
#define DPSESS_FLAG_LINK_LOCAL    0x0080    /* 链路本地地址 */
#define DPSESS_FLAG_TMP_OPEN      0x0100    /* 临时开放连接 */
#define DPSESS_FLAG_UWLIP         0x0200    /* 未管理工作负载连接 */
#define DPSESS_FLAG_CHK_NBE       0x0400 // check nbe
#define DPSESS_FLAG_NBE_SNS       0x0800 // same ns nbe

#define DP_POLICY_APPLY_EGRESS  0x1
#define DP_POLICY_APPLY_INGRESS 0x2

#define DP_POLICY_ACTION_OPEN          0
// #define DP_POLICY_ACTION_LEARN         1  // 已移除：不支持学习模式
#define DP_POLICY_ACTION_ALLOW         2
#define DP_POLICY_ACTION_CHECK_VH      3
#define DP_POLICY_ACTION_CHECK_NBE     4
#define DP_POLICY_ACTION_CHECK_APP     5
#define DP_POLICY_ACTION_VIOLATE       6
#define DP_POLICY_ACTION_DENY          7

#define DP_POLICY_APP_ANY      0
#define DP_POLICY_APP_UNKNOWN  0xffffffff

#define DP_POLICY_FQDN_MAX_ENTRIES  2048
#define DP_POLICY_FQDN_NAME_MAX_LEN 256

#define CFG_ADD       1
#define CFG_MODIFY    2
#define CFG_DELETE    3

#define MSG_START    0x1
#define MSG_END      0x2

#define MAX_SIG_NAME_LEN 512 + 10
#define DP_DLP_RULE_NAME_MAX_LEN MAX_SIG_NAME_LEN
#define DP_DLP_RULE_PATTERN_MAX_LEN 512

typedef struct {
    uint32_t ID;
    uint8_t  EPMAC[6];
    uint16_t EtherType;
    uint8_t  ClientMAC[6];
    uint8_t  ServerMAC[6];
    uint8_t  ClientIP[16];
    uint8_t  ServerIP[16];
    uint16_t ClientPort;
    uint16_t ServerPort;
    uint8_t  ICMPCode;
    uint8_t  ICMPType;
    uint8_t  IPProto;
    uint8_t  Padding;
    uint32_t ClientPkts;
    uint32_t ServerPkts;
    uint32_t ClientBytes;
    uint32_t ServerBytes;
    uint32_t ClientAsmPkts;
    uint32_t ServerAsmPkts;
    uint32_t ClientAsmBytes;
    uint32_t ServerAsmBytes;
    uint8_t  ClientState;
    uint8_t  ServerState;
    uint16_t Idle;
    uint32_t Age;
    uint16_t Life;
    uint16_t Application;
    uint32_t ThreatID;
    uint32_t PolicyId;
    uint8_t  PolicyAction;
    uint8_t  Severity;
    uint16_t Flags;
    uint8_t  XffIP[16];
    uint16_t XffApp;
    uint16_t XffPort;
} DPMsgSession;

typedef struct {
    uint32_t EpSessCurIn;
    uint32_t EpSessIn12;
    uint64_t EpByteIn12;
} DPMonitorMetric;

typedef struct {
    uint16_t Sessions;
    uint16_t Reserved;
    // DPMsgSession Sessions[0];
} DPMsgSessionHdr;

#define DPMETER_FLAG_IPV4    0x01
#define DPMETER_FLAG_TAP     0x02

#define METER_ID_SYN_FLOOD      0
#define METER_ID_ICMP_FLOOD     1
#define METER_ID_IP_SRC_SESSION 2
#define METER_ID_TCP_NODATA     3

typedef struct {
    uint8_t  EPMAC[6];
    uint16_t Idle;
    uint32_t Count;
    uint32_t LastCount;
    uint8_t  PeerIP[16];
    uint8_t  MeterID;
    uint8_t  Flags;
    uint8_t  Span;
    uint32_t UpperLimit;
    uint32_t LowerLimit;
} DPMsgMeter;

typedef struct {
    uint16_t Meters;
    uint16_t Reserved;
    // DPMsgMeter Meters[0];
} DPMsgMeterHdr;

typedef struct {
    uint64_t RXPackets;
    uint64_t RXDropPackets;
    uint64_t TXPackets;
    uint64_t TXDropPackets;
    uint64_t ErrorPackets;
    uint64_t NoWorkloadPackets;
    uint64_t IPv4Packets;
    uint64_t IPv6Packets;
    uint64_t TCPPackets;
    uint64_t TCPNoSessionPackets;
    uint64_t UDPPackets;
    uint64_t ICMPPackets;
    uint64_t OtherPackets;
    uint64_t Assemblys;
    uint64_t FreedAssemblys;
    uint64_t Fragments;
    uint64_t FreedFragments;
    uint64_t TimeoutFragments;
    uint64_t TotalSessions;
    uint64_t TCPSessions;
    uint64_t UDPSessions;
    uint64_t ICMPSessions;
    uint64_t IPSessions;
    uint64_t DropMeters;
    uint64_t ProxyMeters;
    uint64_t CurMeters;
    uint64_t CurLogCaches;
    uint64_t ParserSessions[DPI_PARSER_MAX];
    uint64_t ParserPackets[DPI_PARSER_MAX];
    uint32_t PolicyType1Rules;
    uint32_t PolicyType2Rules;
    uint32_t PolicyDomains;
    uint32_t PolicyDomainIPs;
    uint64_t LimitDropConns;
    uint64_t LimitPassConns;
} DPMsgDeviceCounter;

typedef struct {
    uint32_t Interval;
    uint32_t Padding;

    uint32_t SessionIn;
    uint32_t SessionOut;
    uint32_t SessionCurIn;
    uint32_t SessionCurOut;
    uint64_t PacketIn;
    uint64_t PacketOut;
    uint64_t ByteIn;
    uint64_t ByteOut;

    uint32_t SessionIn1;
    uint32_t SessionOut1;
    uint64_t PacketIn1;
    uint64_t PacketOut1;
    uint64_t ByteIn1;
    uint64_t ByteOut1;

    uint32_t SessionIn12;
    uint32_t SessionOut12;
    uint64_t PacketIn12;
    uint64_t PacketOut12;
    uint64_t ByteIn12;
    uint64_t ByteOut12;

    uint32_t SessionIn60;
    uint32_t SessionOut60;
    uint64_t PacketIn60;
    uint64_t PacketOut60;
    uint64_t ByteIn60;
    uint64_t ByteOut60;
} DPMsgStats;

#define DPLOG_MAX_MSG_LEN         64
#define DPLOG_MAX_PKT_LEN       2048

#define DPLOG_FLAG_PKT_INGRESS  0x01
#define DPLOG_FLAG_SESS_INGRESS 0x02
#define DPLOG_FLAG_TAP          0x04

typedef struct {
    uint32_t ThreatID;
    uint32_t ReportedAt;
    uint32_t Count;
    uint8_t  Action;
    uint8_t  Severity;
    uint8_t  IPProto;
    uint8_t  Flags;
    uint8_t  EPMAC[6];
    uint16_t EtherType;
    uint8_t  SrcIP[16];
    uint8_t  DstIP[16];
    uint16_t SrcPort;
    uint16_t DstPort;
    uint8_t  ICMPCode;
    uint8_t  ICMPType;
    uint16_t Application;
    uint16_t PktLen;    // Packet content length copied into 'Packet'
    uint16_t CapLen;    // Captured packet length on the wire
    char Msg[DPLOG_MAX_MSG_LEN];
    char Packet[DPLOG_MAX_PKT_LEN];
    uint32_t DlpNameHash;
} DPMsgThreatLog;

#define DPCONN_FLAG_INGRESS       0x0001
#define DPCONN_FLAG_EXTERNAL      0x0002
#define DPCONN_FLAG_XFF           0x0004
#define DPCONN_FLAG_SVC_EXTIP     0x0008
#define DPCONN_FLAG_MESH_TO_SVR   0x0010
#define DPCONN_FLAG_LINK_LOCAL    0x0020
#define DPCONN_FLAG_TMP_OPEN      0x0040
#define DPCONN_FLAG_UWLIP         0x0080
#define DPCONN_FLAG_CHK_NBE       0x0100
#define DPCONN_FLAG_NBE_SNS       0x0200

typedef struct {
    uint8_t  EPMAC[6];
    uint8_t  IPProto;
    uint8_t  Padding;
    uint16_t ServerPort;
    uint16_t ClientPort;
    uint8_t  ClientIP[16];
    uint8_t  ServerIP[16];
    uint16_t EtherType;
    uint16_t Flags;
    uint32_t Bytes;  // Delta to last sent
    uint32_t Sessions;
    uint32_t FirstSeenAt;
    uint32_t LastSeenAt;
    uint16_t Application;
    uint8_t  PolicyAction;
    uint8_t  Severity;
    uint32_t PolicyId;
    uint32_t Violates;
    uint32_t ThreatID;
    uint32_t EpSessCurIn;
    uint32_t EpSessIn12;
    uint64_t EpByteIn12;
} DPMsgConnect;

typedef struct {
    uint16_t Connects;
    uint16_t Reserved;
    // DPMsgConnect Connect[0];
} DPMsgConnectHdr;

typedef struct {
    uint8_t  FqdnIP[16];
} DPMsgFqdnIp;

#define DPFQDN_IP_FLAG_VH       0x01
typedef struct {
    char  FqdnName[DP_POLICY_FQDN_NAME_MAX_LEN];
    uint16_t IpCnt;
    uint16_t Reserved;
    uint8_t Flags;
} DPMsgFqdnIpHdr;

typedef struct {
    uint8_t  IP[16];
    char     Name[DP_POLICY_FQDN_NAME_MAX_LEN];
} DPMsgIpFqdnStorageUpdateHdr;

typedef struct {
    uint8_t  IP[16];
} DPMsgIpFqdnStorageReleaseHdr;

#endif
