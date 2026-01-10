/*
Package agent 提供Agent层核心类型定义

定义微隔离系统中Agent层使用的核心数据结构：
  - 策略模式和动作类型
  - 连接信息和威胁日志
  - 工作负载和主机信息
  - 网络策略规则
  - 容器事件和元数据

从NeuVector agent简化提取，移除所有外部依赖
*/
package agent

import (
	"net"
	"time"
)

// PolicyMode 策略执行模式
type PolicyMode string

const (
	// PolicyModeMonitor 监控模式 - 只记录不阻断
	PolicyModeMonitor PolicyMode = "Monitor"
	// PolicyModeProtect 防护模式 - 执行阻断动作
	PolicyModeProtect PolicyMode = "Protect"
)

// PolicyAction 策略执行动作
type PolicyAction uint8

const (
	PolicyActionOpen    PolicyAction = 0 // 开放
	PolicyActionAllow   PolicyAction = 1 // 允许
	PolicyActionDeny    PolicyAction = 2 // 拒绝
	PolicyActionViolate PolicyAction = 3 // 违规
)

// Connection 网络连接信息，记录两个端点间的通信详情
type Connection struct {
	AgentID      string        // Agent标识
	HostID       string        // 主机标识
	ClientWL     string        // 客户端工作负载
	ServerWL     string        // 服务端工作负载
	ClientIP     net.IP        // 客户端IP
	ServerIP     net.IP        // 服务端IP
	ClientPort   uint16        // 客户端端口
	ServerPort   uint16        // 服务端端口
	IPProto      uint8         // IP协议号
	Application  uint32        // 应用协议标识
	Bytes        uint64        // 传输字节数
	Sessions     uint32        // 会话数量
	Violates     uint32        // 违规次数
	FirstSeenAt  uint32        // 首次发现时间
	LastSeenAt   uint32        // 最后发现时间
	ThreatID     uint32        // 威胁ID
	Severity     uint8         // 严重级别
	PolicyAction uint8         // 策略动作
	PolicyId     uint32        // 策略ID
	Ingress      bool          // 是否为入站连接
	ExternalPeer bool          // 是否为外部对等端
	LocalPeer    bool          // 是否为本地对等端
	Scope        string        // 作用域
	Network      string        // 网络名称
}

// ConnectionData 连接数据，包含MAC地址和连接信息
type ConnectionData struct {
	EPMAC net.HardwareAddr // 端点MAC地址
	Conn  *Connection      // 连接详情
}

// ThreatLog 威胁检测日志，记录安全威胁事件
type ThreatLog struct {
	ID           string    // 日志唯一标识
	ThreatID     uint32    // 威胁类型ID
	ThreatName   string    // 威胁名称
	Severity     string    // 严重级别
	ClientWL     string    // 客户端工作负载
	ServerWL     string    // 服务端工作负载
	ClientIP     net.IP    // 客户端IP
	ServerIP     net.IP    // 服务端IP
	ServerPort   uint16    // 服务端端口
	IPProto      uint8     // IP协议号
	PktIngress   bool      // 数据包是否为入站
	LocalPeer    bool      // 是否为本地对等端
	HostID       string    // 主机ID
	HostName     string    // 主机名
	AgentID      string    // Agent ID
	AgentName    string    // Agent名称
	WorkloadID   string    // 工作负载ID
	WorkloadName string    // 工作负载名称
	ReportedAt   time.Time // 报告时间
}

// Workload 工作负载定义，表示一个被保护的应用实例
type Workload struct {
	ID         string                  // 工作负载唯一标识
	Name       string                  // 工作负载名称
	HostID     string                  // 所属主机ID
	HostName   string                  // 所属主机名
	Domain     string                  // 域名
	Service    string                  // 服务名称
	PolicyMode PolicyMode              // 策略模式
	Running    bool                    // 运行状态
	Pid        int                     // 进程ID
	Ifaces     map[string][]IPAddr     // 网络接口映射
}

// IPAddr IP地址信息，包含地址、网络和网关配置
type IPAddr struct {
	IP      net.IP     // IP地址
	IPNet   net.IPNet  // 网络地址段
	Scope   string     // 地址作用域
	Gateway string     // 网关地址
}

// Host 主机信息，描述Agent运行的物理或虚拟主机
type Host struct {
	ID       string                  // 主机唯一标识
	Name     string                  // 主机名称
	Platform string                  // 平台类型
	Ifaces   map[string][]IPAddr     // 网络接口映射
}

// Agent 代理程序信息
type Agent struct {
	ID       string // Agent唯一标识
	Name     string // Agent名称
	HostID   string // 所属主机ID
	HostName string // 所属主机名
	Domain   string // 域名
	Version  string // 版本号
}

// PolicyRule 网络策略规则，定义流量控制规则
type PolicyRule struct {
	ID           uint32        // 规则唯一标识
	From         string        // 源地址或组
	To           string        // 目标地址或组
	Ports        string        // 端口范围
	Applications []uint32      // 应用协议列表
	Action       PolicyAction  // 执行动作
	Ingress      bool          // 是否为入站规则
}

// ContainerEvent 容器生命周期事件类型
type ContainerEvent int

const (
	EventContainerStart  ContainerEvent = iota // 容器启动
	EventContainerStop                         // 容器停止
	EventContainerDelete                       // 容器删除
)

// ContainerMeta 容器元数据信息
type ContainerMeta struct {
	ID       string            // 容器ID
	Name     string            // 容器名称
	Image    string            // 镜像名称
	Pid      int               // 主进程ID
	Running  bool              // 运行状态
	Labels   map[string]string // 标签映射
	Envs     []string          // 环境变量列表
	Networks map[string]string // 网络配置映射
}

// Subnet 子网定义
type Subnet struct {
	Subnet net.IPNet // 子网地址段
	Scope  string    // 子网作用域
}

// ProtoPort 协议端口组合
type ProtoPort struct {
	Port    uint16 // 端口号
	IPProto uint8  // IP协议号
}

// App 应用信息，描述端口上运行的应用
type App struct {
	Port        uint16 // 端口号
	IPProto     uint8  // IP协议号
	Application uint32 // 应用类型标识
	Server      uint32 // 服务器类型标识
}
