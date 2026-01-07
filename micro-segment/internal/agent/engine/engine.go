// Package engine 提供Agent引擎
// 从NeuVector agent简化提取
package engine

import (
	"net"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/micro-segment/internal/agent"
	"github.com/micro-segment/internal/agent/connection"
	"github.com/micro-segment/internal/agent/dp"
	agentgrpc "github.com/micro-segment/internal/agent/grpc"
	"github.com/micro-segment/internal/agent/policy"
)

// Engine Agent引擎
type Engine struct {
	mutex sync.RWMutex

	// 配置
	config *Config

	// 组件
	aggregator *connection.Aggregator
	dpClient   *dp.DPClient
	grpcClient *agentgrpc.Client
	policy     *policy.NetworkPolicy

	// 状态
	host       *agent.Host
	agentInfo  *agent.Agent
	workloads  map[string]*agent.Workload
	hostIPs    map[string]bool
	subnets    map[string]*agent.Subnet

	// 默认策略模式
	defaultPolicyMode agent.PolicyMode

	// 运行状态
	running bool
	stopCh  chan struct{}
}

// Config 引擎配置
type Config struct {
	AgentID        string
	HostID         string
	HostName       string
	DPSocketPath   string
	GRPCAddr       string
	NetworkManager interface{} // 网络管理器接口
}

// NewEngine 创建引擎
func NewEngine(config *Config) *Engine {
	e := &Engine{
		config:            config,
		workloads:         make(map[string]*agent.Workload),
		hostIPs:           make(map[string]bool),
		subnets:           make(map[string]*agent.Subnet),
		defaultPolicyMode: agent.PolicyModeMonitor, // 默认Monitor模式
		stopCh:            make(chan struct{}),
	}

	// 初始化组件
	e.aggregator = connection.NewAggregator(config.AgentID, config.HostID)
	e.dpClient = dp.NewDPClient(config.DPSocketPath)
	e.grpcClient = agentgrpc.NewClient(config.GRPCAddr, config.AgentID, config.HostID, config.HostName, "0.1.0")
	e.policy = policy.NewNetworkPolicy(e.dpClient)

	// 设置回调
	e.aggregator.SetOnConnections(e.onConnections)
	e.aggregator.SetOnThreatLogs(e.onThreatLogs)

	return e
}

// Start 启动引擎
func (e *Engine) Start() error {
	log.Info("Starting agent engine")

	// 连接DP
	if err := e.dpClient.Connect(); err != nil {
		log.WithError(err).Warn("Failed to connect to DP")
		// 不阻止启动，DP可能稍后启动
	}

	// 设置DP回调
	e.dpClient.SetOnConnection(e.onDPConnection)
	e.dpClient.SetOnThreatLog(e.onDPThreatLog)

	// 连接Controller
	if err := e.grpcClient.Connect(); err != nil {
		log.WithError(err).Warn("Failed to connect to Controller")
		// 不阻止启动，Controller可能稍后启动
	} else {
		// 注册Agent
		if err := e.grpcClient.Register(); err != nil {
			log.WithError(err).Warn("Failed to register agent")
		}
	}

	// 启动聚合器
	e.aggregator.Start()

	e.running = true
	log.Info("Agent engine started")
	return nil
}

// Stop 停止引擎
func (e *Engine) Stop() {
	log.Info("Stopping agent engine")

	e.running = false
	close(e.stopCh)

	e.aggregator.Stop()
	e.dpClient.Disconnect()
	e.grpcClient.Disconnect()

	log.Info("Agent engine stopped")
}

// onConnections 连接上报回调
func (e *Engine) onConnections(conns []*agent.Connection) {
	log.WithField("count", len(conns)).Debug("Reporting connections")
	
	// 发送到Controller
	if e.grpcClient.IsConnected() {
		if err := e.grpcClient.ReportConnections(conns); err != nil {
			log.WithError(err).Warn("Failed to report connections")
		}
	}
}

// onThreatLogs 威胁日志回调
func (e *Engine) onThreatLogs(logs []*agent.ThreatLog) {
	log.WithField("count", len(logs)).Debug("Reporting threat logs")
	
	// 发送到Controller
	if e.grpcClient.IsConnected() {
		if err := e.grpcClient.ReportThreats(logs); err != nil {
			log.WithError(err).Warn("Failed to report threats")
		}
	}
}

// onDPConnection DP连接回调
func (e *Engine) onDPConnection(conn *dp.DPConnection) {
	// 转换为agent.Connection
	agentConn := &agent.Connection{
		ClientIP:     conn.ClientIP,
		ServerIP:     conn.ServerIP,
		ClientPort:   conn.ClientPort,
		ServerPort:   conn.ServerPort,
		IPProto:      conn.IPProto,
		Application:  conn.Application,
		Bytes:        conn.Bytes,
		Sessions:     conn.Sessions,
		FirstSeenAt:  conn.FirstSeenAt,
		LastSeenAt:   conn.LastSeenAt,
		ThreatID:     conn.ThreatID,
		Severity:     conn.Severity,
		PolicyAction: conn.PolicyAction,
		PolicyId:     conn.PolicyId,
		Ingress:      conn.Ingress,
		ExternalPeer: conn.ExternalPeer,
	}

	// 添加到聚合器
	e.aggregator.AddConnection(&agent.ConnectionData{
		EPMAC: conn.EPMAC,
		Conn:  agentConn,
	})
}

// onDPThreatLog DP威胁日志回调
func (e *Engine) onDPThreatLog(threat *dp.DPThreatLog) {
	// 转换为agent.ThreatLog
	agentThreat := &agent.ThreatLog{
		ThreatID:   threat.ThreatID,
		Severity:   severityToString(threat.Severity),
		ClientIP:   threat.ClientIP,
		ServerIP:   threat.ServerIP,
		ServerPort: threat.ServerPort,
		IPProto:    threat.IPProto,
		PktIngress: threat.PktIngress,
		ReportedAt: time.Now(),
	}

	// 添加到聚合器
	e.aggregator.AddThreatLog(threat.EPMAC, agentThreat)
}

// severityToString 转换严重级别
func severityToString(severity uint8) string {
	switch severity {
	case 1:
		return "Low"
	case 2:
		return "Medium"
	case 3:
		return "High"
	case 4:
		return "Critical"
	default:
		return "Info"
	}
}

// AddWorkload 添加工作负载
func (e *Engine) AddWorkload(wl *agent.Workload) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.workloads[wl.ID] = wl
	log.WithFields(log.Fields{
		"id":   wl.ID,
		"name": wl.Name,
	}).Info("Workload added")

	// 注册MAC到DP
	for _, addrs := range wl.Ifaces {
		for _, addr := range addrs {
			// TODO: 获取MAC地址并注册
			_ = addr
		}
	}
}

// RemoveWorkload 移除工作负载
func (e *Engine) RemoveWorkload(id string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if wl, ok := e.workloads[id]; ok {
		delete(e.workloads, id)
		log.WithFields(log.Fields{
			"id":   wl.ID,
			"name": wl.Name,
		}).Info("Workload removed")
	}
}

// GetWorkload 获取工作负载
func (e *Engine) GetWorkload(id string) *agent.Workload {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.workloads[id]
}

// ListWorkloads 列出所有工作负载
func (e *Engine) ListWorkloads() []*agent.Workload {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	result := make([]*agent.Workload, 0, len(e.workloads))
	for _, wl := range e.workloads {
		result = append(result, wl)
	}
	return result
}

// UpdatePolicies 更新策略
func (e *Engine) UpdatePolicies(rules []*agent.PolicyRule) {
	e.policy.UpdateRules(rules)
}

// SetDefaultPolicyMode 设置默认策略模式
func (e *Engine) SetDefaultPolicyMode(mode agent.PolicyMode) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.defaultPolicyMode = mode
	log.WithField("mode", mode).Info("Default policy mode changed")
}

// GetDefaultPolicyMode 获取默认策略模式
func (e *Engine) GetDefaultPolicyMode() agent.PolicyMode {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.defaultPolicyMode
}

// IsLocalIP 检查是否为本地IP
func (e *Engine) IsLocalIP(ip net.IP) bool {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return e.hostIPs[ip.String()]
}

// IsInternalIP 检查是否为内部IP
func (e *Engine) IsInternalIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, subnet := range e.subnets {
		if subnet.Subnet.Contains(ip) {
			return true
		}
	}
	return false
}

// UpdateSubnets 更新内部子网
func (e *Engine) UpdateSubnets(subnets map[string]*agent.Subnet) {
	e.mutex.Lock()
	e.subnets = subnets
	e.mutex.Unlock()

	// 同步到DP
	subnetList := make([]net.IPNet, 0, len(subnets))
	for _, subnet := range subnets {
		subnetList = append(subnetList, subnet.Subnet)
	}
	e.dpClient.ConfigSubnets(subnetList)
}

// GetStats 获取统计信息
func (e *Engine) GetStats() map[string]interface{} {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return map[string]interface{}{
		"workloads":        len(e.workloads),
		"policies":         e.policy.GetRuleCount(),
		"connections":      e.aggregator.GetConnectionCount(),
		"max_connections":  e.aggregator.GetMaxConnections(),
		"dp_connected":     e.dpClient.IsConnected(),
		"default_mode":     e.defaultPolicyMode,
	}
}
