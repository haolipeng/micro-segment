/*
Package connection 提供连接聚合功能

连接聚合器负责收集、合并和批量上报网络连接信息：
  - 连接数据的缓存和聚合
  - 威胁日志的收集和上报
  - 定时批量传输机制
  - 连接映射表的管理和优化

从NeuVector agent简化提取，保留核心聚合逻辑
*/
package connection

import (
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/micro-segment/internal/agent"
)

// connectionMapMax 连接映射最大容量（扩大到131K以支持大规模环境）
const connectionMapMax int = 2048 * 64

// connectionListMax 单次传输最大连接数，避免消息过大
const connectionListMax int = 2048 * 4

// reportInterval 上报间隔（秒），定期将聚合数据发送给Controller
const reportInterval uint32 = 5

// Aggregator 连接聚合器，负责收集和批量上报连接信息
type Aggregator struct {
	mutex          sync.Mutex                    // 连接映射表锁
	connectionMap  map[string]*agent.Connection  // 连接聚合映射表
	connsCache     []*agent.ConnectionData       // 连接数据缓存
	connsCacheMux  sync.Mutex                    // 缓存锁
	threatLogCache []*threatLogEntry             // 威胁日志缓存
	threatMutex    sync.Mutex                    // 威胁日志锁

	// 回调函数
	onConnections func([]*agent.Connection) // 连接上报回调
	onThreatLogs  func([]*agent.ThreatLog)  // 威胁日志上报回调

	// Agent信息
	agentID  string // Agent标识
	hostID   string // 主机标识

	// 运行状态
	running bool
	stopCh  chan struct{}
}

// threatLogEntry 威胁日志条目，包含MAC地址和日志内容
type threatLogEntry struct {
	mac  net.HardwareAddr  // 端点MAC地址
	slog *agent.ThreatLog  // 威胁日志详情
}

// NewAggregator 创建新的连接聚合器实例
func NewAggregator(agentID, hostID string) *Aggregator {
	return &Aggregator{
		connectionMap:  make(map[string]*agent.Connection),
		connsCache:     make([]*agent.ConnectionData, 0),
		threatLogCache: make([]*threatLogEntry, 0),
		agentID:        agentID,
		hostID:         hostID,
		stopCh:         make(chan struct{}),
	}
}

// SetOnConnections 设置连接数据上报回调函数
func (a *Aggregator) SetOnConnections(cb func([]*agent.Connection)) {
	a.onConnections = cb
}

// SetOnThreatLogs 设置威胁日志上报回调函数
func (a *Aggregator) SetOnThreatLogs(cb func([]*agent.ThreatLog)) {
	a.onThreatLogs = cb
}

// Start 启动聚合器，开始定时上报循环
func (a *Aggregator) Start() {
	a.running = true
	go a.timerLoop()
}

// Stop 停止聚合器，清理资源
func (a *Aggregator) Stop() {
	a.running = false
	close(a.stopCh)
}

// timerLoop 定时器循环，定期刷新和上报数据
func (a *Aggregator) timerLoop() {
	ticker := time.NewTicker(time.Second * time.Duration(reportInterval))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.flush() // 定时刷新数据
		case <-a.stopCh:
			return
		}
	}
}

// flush 刷新缓存数据，执行威胁日志上报、连接更新和连接上报
func (a *Aggregator) flush() {
	a.putThreatLogs()    // 上报威胁日志
	a.updateConnections() // 更新连接映射
	a.putConnections()   // 上报连接数据
}

// AddConnection 添加连接数据到缓存，由DP回调调用
func (a *Aggregator) AddConnection(data *agent.ConnectionData) {
	a.connsCacheMux.Lock()
	a.connsCache = append(a.connsCache, data)
	a.connsCacheMux.Unlock()
}

// AddThreatLog 添加威胁日志到缓存
func (a *Aggregator) AddThreatLog(mac net.HardwareAddr, slog *agent.ThreatLog) {
	a.threatMutex.Lock()
	a.threatLogCache = append(a.threatLogCache, &threatLogEntry{mac: mac, slog: slog})
	a.threatMutex.Unlock()
}

// updateConnections 处理缓存的连接数据，更新到聚合映射表
func (a *Aggregator) updateConnections() {
	a.connsCacheMux.Lock()
	conns := a.connsCache
	a.connsCache = make([]*agent.ConnectionData, 0)
	a.connsCacheMux.Unlock()

	for _, data := range conns {
		conn := data.Conn
		conn.AgentID = a.agentID
		conn.HostID = a.hostID
		a.updateConnectionMap(conn)
	}
}

// keyTCPUDPConnection 为TCP/UDP连接生成唯一键
func keyTCPUDPConnection(conn *agent.Connection) string {
	return fmt.Sprintf("%v-%v-%v-%v-%v-%v-%v",
		conn.ClientIP, conn.ServerIP, conn.ServerPort, conn.IPProto, conn.Ingress, conn.PolicyId, conn.Application)
}

// keyOtherConnection 为其他协议连接生成唯一键
func keyOtherConnection(conn *agent.Connection) string {
	return fmt.Sprintf("%v-%v-%v-%v-%v",
		conn.ClientIP, conn.ServerIP, conn.Ingress, conn.PolicyId, conn.Application)
}

// updateConnectionMap 更新连接聚合映射表，合并相同连接的统计信息
func (a *Aggregator) updateConnectionMap(conn *agent.Connection) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	var key string
	if conn.IPProto == syscall.IPPROTO_TCP || conn.IPProto == syscall.IPPROTO_UDP {
		key = keyTCPUDPConnection(conn)
	} else {
		key = keyOtherConnection(conn)
	}

	if entry, exist := a.connectionMap[key]; exist {
		// 更新已存在的连接统计信息
		entry.Bytes += conn.Bytes
		entry.Sessions += conn.Sessions
		entry.Violates += conn.Violates
		if entry.LastSeenAt <= conn.LastSeenAt {
			entry.LastSeenAt = conn.LastSeenAt
			entry.PolicyAction = conn.PolicyAction
			entry.PolicyId = conn.PolicyId
		}
		if entry.Severity < conn.Severity {
			entry.Severity = conn.Severity
			entry.ThreatID = conn.ThreatID
		}
	} else if len(a.connectionMap) < connectionMapMax || conn.PolicyAction > uint8(agent.PolicyActionAllow) {
		// 新连接：容量未满或高优先级（VIOLATE/DENY）
		a.connectionMap[key] = conn
	} else {
		log.WithFields(log.Fields{
			"conn": conn, "len": len(a.connectionMap),
		}).Debug("Connection map full -- drop")
	}
}

// putConnections 批量上报连接数据给Controller
func (a *Aggregator) putConnections() {
	var list []*agent.Connection
	var keys []string

	a.mutex.Lock()
	for key, conn := range a.connectionMap {
		list = append(list, conn)
		keys = append(keys, key)
		delete(a.connectionMap, key)

		if len(list) == connectionListMax {
			break
		}
	}
	a.mutex.Unlock()

	if len(list) > 0 && a.onConnections != nil {
		a.onConnections(list)
	}
}

// putThreatLogs 批量上报威胁日志给Controller
func (a *Aggregator) putThreatLogs() {
	a.threatMutex.Lock()
	tmp := a.threatLogCache
	a.threatLogCache = make([]*threatLogEntry, 0)
	a.threatMutex.Unlock()

	if len(tmp) > 0 && a.onThreatLogs != nil {
		logs := make([]*agent.ThreatLog, 0, len(tmp))
		for _, entry := range tmp {
			logs = append(logs, entry.slog)
		}
		a.onThreatLogs(logs)
	}
}

// GetConnectionCount 获取当前连接映射表中的连接数量
func (a *Aggregator) GetConnectionCount() int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return len(a.connectionMap)
}

// GetMaxConnections 获取连接映射表的最大容量
func (a *Aggregator) GetMaxConnections() int {
	return connectionMapMax
}
