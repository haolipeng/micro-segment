// Package agent 提供Agent层核心类型定义
// 从NeuVector agent简化提取，移除所有外部依赖
package agent

import (
	"net"
	"time"
)

// PolicyMode 策略模式
type PolicyMode string

const (
	// PolicyModeMonitor 监控模式
	PolicyModeMonitor PolicyMode = "Monitor"
	// PolicyModeProtect 防护模式
	PolicyModeProtect PolicyMode = "Protect"
)

// PolicyAction 策略动作
type PolicyAction uint8

const (
	PolicyActionOpen    PolicyAction = 0
	PolicyActionAllow   PolicyAction = 1
	PolicyActionDeny    PolicyAction = 2
	PolicyActionViolate PolicyAction = 3
)

// Connection 连接信息
type Connection struct {
	AgentID      string
	HostID       string
	ClientWL     string
	ServerWL     string
	ClientIP     net.IP
	ServerIP     net.IP
	ClientPort   uint16
	ServerPort   uint16
	IPProto      uint8
	Application  uint32
	Bytes        uint64
	Sessions     uint32
	Violates     uint32
	FirstSeenAt  uint32
	LastSeenAt   uint32
	ThreatID     uint32
	Severity     uint8
	PolicyAction uint8
	PolicyId     uint32
	Ingress      bool
	ExternalPeer bool
	LocalPeer    bool
	Scope        string
	Network      string
}

// ConnectionData 连接数据（包含MAC）
type ConnectionData struct {
	EPMAC net.HardwareAddr
	Conn  *Connection
}

// ThreatLog 威胁日志
type ThreatLog struct {
	ID         string
	ThreatID   uint32
	ThreatName string
	Severity   string
	ClientWL   string
	ServerWL   string
	ClientIP   net.IP
	ServerIP   net.IP
	ServerPort uint16
	IPProto    uint8
	PktIngress bool
	LocalPeer  bool
	HostID     string
	HostName   string
	AgentID    string
	AgentName  string
	WorkloadID string
	WorkloadName string
	ReportedAt time.Time
}

// Workload 工作负载
type Workload struct {
	ID         string
	Name       string
	HostID     string
	HostName   string
	Domain     string
	Service    string
	PolicyMode PolicyMode
	Running    bool
	Pid        int
	Ifaces     map[string][]IPAddr
}

// IPAddr IP地址
type IPAddr struct {
	IP      net.IP
	IPNet   net.IPNet
	Scope   string
	Gateway string
}

// Host 主机信息
type Host struct {
	ID       string
	Name     string
	Platform string
	Ifaces   map[string][]IPAddr
}

// Agent 代理信息
type Agent struct {
	ID       string
	Name     string
	HostID   string
	HostName string
	Domain   string
	Version  string
}

// PolicyRule 策略规则
type PolicyRule struct {
	ID           uint32
	From         string
	To           string
	Ports        string
	Applications []uint32
	Action       PolicyAction
	Ingress      bool
}

// ContainerEvent 容器事件类型
type ContainerEvent int

const (
	EventContainerStart ContainerEvent = iota
	EventContainerStop
	EventContainerDelete
)

// ContainerMeta 容器元数据
type ContainerMeta struct {
	ID       string
	Name     string
	Image    string
	Pid      int
	Running  bool
	Labels   map[string]string
	Envs     []string
	Networks map[string]string
}

// Subnet 子网
type Subnet struct {
	Subnet net.IPNet
	Scope  string
}

// ProtoPort 协议端口
type ProtoPort struct {
	Port    uint16
	IPProto uint8
}

// App 应用
type App struct {
	Port        uint16
	IPProto     uint8
	Application uint32
	Server      uint32
}
