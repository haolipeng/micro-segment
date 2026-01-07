// Package connection 提供连接聚合功能
// 从NeuVector agent简化提取
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

// connectionMapMax 连接映射最大容量（扩大到131K）
const connectionMapMax int = 2048 * 64

// connectionListMax 单次传输最大连接数
const connectionListMax int = 2048 * 4

// reportInterval 上报间隔（秒）
const reportInterval uint32 = 5

// Aggregator 连接聚合器
type Aggregator struct {
	mutex          sync.Mutex
	connectionMap  map[string]*agent.Connection
	connsCache     []*agent.ConnectionData
	connsCacheMux  sync.Mutex
	threatLogCache []*threatLogEntry
	threatMutex    sync.Mutex

	// 回调函数
	onConnections func([]*agent.Connection)
	onThreatLogs  func([]*agent.ThreatLog)

	// Agent信息
	agentID  string
	hostID   string

	// 运行状态
	running bool
	stopCh  chan struct{}
}

type threatLogEntry struct {
	mac  net.HardwareAddr
	slog *agent.ThreatLog
}

// NewAggregator 创建聚合器
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

// SetOnConnections 设置连接上报回调
func (a *Aggregator) SetOnConnections(cb func([]*agent.Connection)) {
	a.onConnections = cb
}

// SetOnThreatLogs 设置威胁日志回调
func (a *Aggregator) SetOnThreatLogs(cb func([]*agent.ThreatLog)) {
	a.onThreatLogs = cb
}

// Start 启动聚合器
func (a *Aggregator) Start() {
	a.running = true
	go a.timerLoop()
}

// Stop 停止聚合器
func (a *Aggregator) Stop() {
	a.running = false
	close(a.stopCh)
}

// timerLoop 定时器循环
func (a *Aggregator) timerLoop() {
	ticker := time.NewTicker(time.Second * time.Duration(reportInterval))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.flush()
		case <-a.stopCh:
			return
		}
	}
}

// flush 刷新数据
func (a *Aggregator) flush() {
	a.putThreatLogs()
	a.updateConnections()
	a.putConnections()
}

// AddConnection 添加连接数据
func (a *Aggregator) AddConnection(data *agent.ConnectionData) {
	a.connsCacheMux.Lock()
	a.connsCache = append(a.connsCache, data)
	a.connsCacheMux.Unlock()
}

// AddThreatLog 添加威胁日志
func (a *Aggregator) AddThreatLog(mac net.HardwareAddr, slog *agent.ThreatLog) {
	a.threatMutex.Lock()
	a.threatLogCache = append(a.threatLogCache, &threatLogEntry{mac: mac, slog: slog})
	a.threatMutex.Unlock()
}

// updateConnections 更新连接映射
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

// keyTCPUDPConnection 生成TCP/UDP连接key
func keyTCPUDPConnection(conn *agent.Connection) string {
	return fmt.Sprintf("%v-%v-%v-%v-%v-%v-%v",
		conn.ClientIP, conn.ServerIP, conn.ServerPort, conn.IPProto, conn.Ingress, conn.PolicyId, conn.Application)
}

// keyOtherConnection 生成其他协议连接key
func keyOtherConnection(conn *agent.Connection) string {
	return fmt.Sprintf("%v-%v-%v-%v-%v",
		conn.ClientIP, conn.ServerIP, conn.Ingress, conn.PolicyId, conn.Application)
}

// updateConnectionMap 更新连接映射
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
		// 更新已存在的连接
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

// putConnections 上报连接
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

// putThreatLogs 上报威胁日志
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

// GetConnectionCount 获取当前连接数
func (a *Aggregator) GetConnectionCount() int {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	return len(a.connectionMap)
}

// GetMaxConnections 获取最大连接数
func (a *Aggregator) GetMaxConnections() int {
	return connectionMapMax
}
