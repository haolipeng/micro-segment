// Package controller 提供Controller层核心类型定义
package controller

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
	// PolicyActionOpen 开放
	PolicyActionOpen PolicyAction = 0
	// PolicyActionAllow 允许
	PolicyActionAllow PolicyAction = 1
	// PolicyActionDeny 拒绝
	PolicyActionDeny PolicyAction = 2
	// PolicyActionViolate 违规
	PolicyActionViolate PolicyAction = 3
)

// Group 容器组
type Group struct {
	Name        string            `json:"name"`
	Comment     string            `json:"comment,omitempty"`
	Domain      string            `json:"domain,omitempty"`
	PolicyMode  PolicyMode        `json:"policy_mode"`
	Members     []string          `json:"members,omitempty"`
	Criteria    []GroupCriteria   `json:"criteria,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// GroupCriteria 组匹配条件
type GroupCriteria struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Op    string `json:"op"`
}

// PolicyRule 策略规则
type PolicyRule struct {
	ID           uint32       `json:"id"`
	Comment      string       `json:"comment,omitempty"`
	From         string       `json:"from"`
	To           string       `json:"to"`
	Ports        string       `json:"ports,omitempty"`
	Applications []uint32     `json:"applications,omitempty"`
	Action       string       `json:"action"`
	Disable      bool         `json:"disable"`
	Priority     uint32       `json:"priority"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// Connection 连接信息
type Connection struct {
	ClientWL     string    `json:"client_wl"`
	ServerWL     string    `json:"server_wl"`
	ClientIP     net.IP    `json:"client_ip"`
	ServerIP     net.IP    `json:"server_ip"`
	ClientPort   uint16    `json:"client_port"`
	ServerPort   uint16    `json:"server_port"`
	IPProto      uint8     `json:"ip_proto"`
	Application  uint32    `json:"application"`
	Bytes        uint64    `json:"bytes"`
	Sessions     uint32    `json:"sessions"`
	FirstSeenAt  uint32    `json:"first_seen_at"`
	LastSeenAt   uint32    `json:"last_seen_at"`
	ThreatID     uint32    `json:"threat_id,omitempty"`
	Severity     uint8     `json:"severity,omitempty"`
	PolicyAction uint8     `json:"policy_action"`
	PolicyID     uint32    `json:"policy_id"`
	Ingress      bool      `json:"ingress"`
	ExternalPeer bool      `json:"external_peer"`
	LocalPeer    bool      `json:"local_peer"`
}

// Workload 工作负载
type Workload struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Domain      string            `json:"domain,omitempty"`
	HostID      string            `json:"host_id"`
	HostName    string            `json:"host_name,omitempty"`
	Image       string            `json:"image,omitempty"`
	Service     string            `json:"service,omitempty"`
	PolicyMode  PolicyMode        `json:"policy_mode"`
	Running     bool              `json:"running"`
	Ifaces      map[string][]IPAddr `json:"ifaces,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// IPAddr IP地址
type IPAddr struct {
	IP    net.IP `json:"ip"`
	Scope string `json:"scope"`
}

// Host 主机
type Host struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Platform string            `json:"platform,omitempty"`
	Ifaces   map[string][]IPAddr `json:"ifaces,omitempty"`
}

// Agent 代理
type Agent struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	HostID   string    `json:"host_id"`
	HostName string    `json:"host_name,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
}

// Violation 违规记录
type Violation struct {
	ID           string    `json:"id"`
	ClientWL     string    `json:"client_wl"`
	ServerWL     string    `json:"server_wl"`
	ClientIP     string    `json:"client_ip"`
	ServerIP     string    `json:"server_ip"`
	ServerPort   uint16    `json:"server_port"`
	IPProto      uint8     `json:"ip_proto"`
	Application  string    `json:"application,omitempty"`
	PolicyAction string    `json:"policy_action"`
	PolicyID     uint32    `json:"policy_id"`
	Sessions     uint32    `json:"sessions"`
	ReportedAt   time.Time `json:"reported_at"`
	Level        string    `json:"level"`
}

// ThreatLog 威胁日志
type ThreatLog struct {
	ID         string    `json:"id"`
	ThreatID   uint32    `json:"threat_id"`
	ThreatName string    `json:"threat_name"`
	Severity   string    `json:"severity"`
	ClientWL   string    `json:"client_wl"`
	ServerWL   string    `json:"server_wl"`
	ClientIP   string    `json:"client_ip"`
	ServerIP   string    `json:"server_ip"`
	ServerPort uint16    `json:"server_port"`
	IPProto    uint8     `json:"ip_proto"`
	ReportedAt time.Time `json:"reported_at"`
}

// GraphNode 图节点
type GraphNode struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // workload, group, external, host
	Domain   string `json:"domain,omitempty"`
	Service  string `json:"service,omitempty"`
	PolicyMode string `json:"policy_mode,omitempty"`
}

// GraphLink 图链接
type GraphLink struct {
	From         string `json:"from"`
	To           string `json:"to"`
	Bytes        uint64 `json:"bytes"`
	Sessions     uint32 `json:"sessions"`
	Severity     uint8  `json:"severity,omitempty"`
	PolicyAction uint8  `json:"policy_action"`
}

// NetworkGraph 网络拓扑图
type NetworkGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}
