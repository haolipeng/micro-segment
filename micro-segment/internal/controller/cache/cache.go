// Package cache 提供Controller缓存管理
// 从NeuVector controller/cache简化提取
package cache

import (
	"net"
	"sync"
	"time"

	pb "github.com/micro-segment/api/proto"
	controller "github.com/micro-segment/internal/controller"
	"github.com/micro-segment/internal/controller/graph"
)

// Cache Controller缓存
type Cache struct {
	mutex sync.RWMutex

	// 工作负载缓存
	workloads map[string]*WorkloadCache

	// 组缓存
	groups map[string]*GroupCache

	// 策略缓存
	policies map[uint32]*PolicyCache

	// 主机缓存
	hosts map[string]*HostCache

	// Agent缓存
	agents map[string]*AgentCache

	// 网络拓扑图
	wlGraph *graph.Graph

	// 连接缓存
	connections map[string]*ConnectionCache
}

// WorkloadCache 工作负载缓存
type WorkloadCache struct {
	Workload    *controller.Workload
	Groups      []string
	PolicyMode  controller.PolicyMode
	LastSeenAt  time.Time
}

// GroupCache 组缓存
type GroupCache struct {
	Group      *controller.Group
	Members    map[string]bool
	UsedByPolicy map[uint32]bool
}

// PolicyCache 策略缓存
type PolicyCache struct {
	Rule       *controller.PolicyRule
	Order      int
}

// HostCache 主机缓存
type HostCache struct {
	Host       *controller.Host
	Workloads  []string
}

// AgentCache Agent缓存
type AgentCache struct {
	Agent      *controller.Agent
	Online     bool
	LastSeenAt time.Time
}

// ConnectionCache 连接缓存
type ConnectionCache struct {
	Connection *controller.Connection
	GraphKey   string
}

// NewCache 创建新缓存
func NewCache() *Cache {
	return &Cache{
		workloads:   make(map[string]*WorkloadCache),
		groups:      make(map[string]*GroupCache),
		policies:    make(map[uint32]*PolicyCache),
		hosts:       make(map[string]*HostCache),
		agents:      make(map[string]*AgentCache),
		wlGraph:     graph.NewGraph(),
		connections: make(map[string]*ConnectionCache),
	}
}

// --- 工作负载管理 ---

// AddWorkload 添加工作负载
func (c *Cache) AddWorkload(wl *controller.Workload) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.workloads[wl.ID] = &WorkloadCache{
		Workload:   wl,
		PolicyMode: wl.PolicyMode,
		LastSeenAt: time.Now(),
	}
}

// GetWorkload 获取工作负载
func (c *Cache) GetWorkload(id string) *controller.Workload {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if cache, ok := c.workloads[id]; ok {
		return cache.Workload
	}
	return nil
}

// DeleteWorkload 删除工作负载
func (c *Cache) DeleteWorkload(id string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.workloads, id)
	c.wlGraph.DeleteNode(id)
}

// ListWorkloads 列出所有工作负载
func (c *Cache) ListWorkloads() []*controller.Workload {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]*controller.Workload, 0, len(c.workloads))
	for _, cache := range c.workloads {
		result = append(result, cache.Workload)
	}
	return result
}

// --- 组管理 ---

// AddGroup 添加组
func (c *Cache) AddGroup(group *controller.Group) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.groups[group.Name] = &GroupCache{
		Group:        group,
		Members:      make(map[string]bool),
		UsedByPolicy: make(map[uint32]bool),
	}
}

// GetGroup 获取组
func (c *Cache) GetGroup(name string) *controller.Group {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if cache, ok := c.groups[name]; ok {
		return cache.Group
	}
	return nil
}

// DeleteGroup 删除组
func (c *Cache) DeleteGroup(name string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.groups, name)
}

// ListGroups 列出所有组
func (c *Cache) ListGroups() []*controller.Group {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]*controller.Group, 0, len(c.groups))
	for _, cache := range c.groups {
		result = append(result, cache.Group)
	}
	return result
}

// AddGroupMember 添加组成员
func (c *Cache) AddGroupMember(groupName, workloadID string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if cache, ok := c.groups[groupName]; ok {
		cache.Members[workloadID] = true
	}
}

// RemoveGroupMember 移除组成员
func (c *Cache) RemoveGroupMember(groupName, workloadID string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if cache, ok := c.groups[groupName]; ok {
		delete(cache.Members, workloadID)
	}
}

// --- 策略管理 ---

// AddPolicy 添加策略
func (c *Cache) AddPolicy(rule *controller.PolicyRule, order int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.policies[rule.ID] = &PolicyCache{
		Rule:  rule,
		Order: order,
	}

	// 更新组的策略引用
	if cache, ok := c.groups[rule.From]; ok {
		cache.UsedByPolicy[rule.ID] = true
	}
	if cache, ok := c.groups[rule.To]; ok {
		cache.UsedByPolicy[rule.ID] = true
	}
}

// GetPolicy 获取策略
func (c *Cache) GetPolicy(id uint32) *controller.PolicyRule {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if cache, ok := c.policies[id]; ok {
		return cache.Rule
	}
	return nil
}

// DeletePolicy 删除策略
func (c *Cache) DeletePolicy(id uint32) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if cache, ok := c.policies[id]; ok {
		// 移除组的策略引用
		if gc, ok := c.groups[cache.Rule.From]; ok {
			delete(gc.UsedByPolicy, id)
		}
		if gc, ok := c.groups[cache.Rule.To]; ok {
			delete(gc.UsedByPolicy, id)
		}
	}
	delete(c.policies, id)
}

// ListPolicies 列出所有策略
func (c *Cache) ListPolicies() []*controller.PolicyRule {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]*controller.PolicyRule, 0, len(c.policies))
	for _, cache := range c.policies {
		result = append(result, cache.Rule)
	}
	return result
}

// --- 连接管理 ---

// UpdateConnection 更新连接
func (c *Cache) UpdateConnection(conn *controller.Connection) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 生成连接key
	key := c.connectionKey(conn)

	// 更新连接缓存
	c.connections[key] = &ConnectionCache{
		Connection: conn,
		GraphKey:   key,
	}

	// 更新网络拓扑图
	attr := &GraphAttr{
		Bytes:        conn.Bytes,
		Sessions:     conn.Sessions,
		Severity:     conn.Severity,
		PolicyAction: conn.PolicyAction,
	}
	c.wlGraph.AddLink(conn.ClientWL, "graph", conn.ServerWL, attr)
}

// connectionKey 生成连接key
func (c *Cache) connectionKey(conn *controller.Connection) string {
	return conn.ClientWL + "-" + conn.ServerWL
}

// GraphAttr 图属性
type GraphAttr struct {
	Bytes        uint64
	Sessions     uint32
	Severity     uint8
	PolicyAction uint8
}

// --- 网络拓扑图 ---

// GetNetworkGraph 获取网络拓扑图
func (c *Cache) GetNetworkGraph() *controller.NetworkGraph {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	nodes := make([]controller.GraphNode, 0)
	links := make([]controller.GraphLink, 0)

	// 收集所有节点
	for _, cache := range c.workloads {
		nodes = append(nodes, controller.GraphNode{
			ID:         cache.Workload.ID,
			Name:       cache.Workload.Name,
			Kind:       "workload",
			Domain:     cache.Workload.Domain,
			Service:    cache.Workload.Service,
			PolicyMode: string(cache.Workload.PolicyMode),
		})
	}

	// 收集所有链接
	for _, cache := range c.connections {
		conn := cache.Connection
		links = append(links, controller.GraphLink{
			From:         conn.ClientWL,
			To:           conn.ServerWL,
			Bytes:        conn.Bytes,
			Sessions:     conn.Sessions,
			Severity:     conn.Severity,
			PolicyAction: conn.PolicyAction,
		})
	}

	return &controller.NetworkGraph{
		Nodes: nodes,
		Links: links,
	}
}

// GetGraphNodeCount 获取图节点数量
func (c *Cache) GetGraphNodeCount() int {
	return c.wlGraph.GetNodeCount()
}

// GetGraphLinkCount 获取图链接数量
func (c *Cache) GetGraphLinkCount() int {
	return c.wlGraph.GetLinkCount()
}

// --- 主机管理 ---

// AddHost 添加主机
func (c *Cache) AddHost(host *controller.Host) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.hosts[host.ID] = &HostCache{
		Host:      host,
		Workloads: make([]string, 0),
	}
}

// GetHost 获取主机
func (c *Cache) GetHost(id string) *controller.Host {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if cache, ok := c.hosts[id]; ok {
		return cache.Host
	}
	return nil
}

// ListHosts 列出所有主机
func (c *Cache) ListHosts() []*controller.Host {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]*controller.Host, 0, len(c.hosts))
	for _, cache := range c.hosts {
		result = append(result, cache.Host)
	}
	return result
}

// --- Agent管理 ---

// AddAgent 添加Agent
func (c *Cache) AddAgent(agent *controller.Agent) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.agents[agent.ID] = &AgentCache{
		Agent:      agent,
		Online:     true,
		LastSeenAt: time.Now(),
	}
}

// GetAgent 获取Agent
func (c *Cache) GetAgent(id string) *controller.Agent {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if cache, ok := c.agents[id]; ok {
		return cache.Agent
	}
	return nil
}

// ListAgents 列出所有Agent
func (c *Cache) ListAgents() []*controller.Agent {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	result := make([]*controller.Agent, 0, len(c.agents))
	for _, cache := range c.agents {
		result = append(result, cache.Agent)
	}
	return result
}

// UpdateAgentStatus 更新Agent状态
func (c *Cache) UpdateAgentStatus(id string, online bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if cache, ok := c.agents[id]; ok {
		cache.Online = online
		cache.LastSeenAt = time.Now()
	}
}

// UpdateWorkloadFromProto 从proto更新工作负载
func (c *Cache) UpdateWorkloadFromProto(wl *pb.Workload) {
	if wl == nil {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 转换接口
	ifaces := make(map[string][]controller.IPAddr)
	for _, iface := range wl.Ifaces {
		addrs := make([]controller.IPAddr, 0, len(iface.Addrs))
		for _, addr := range iface.Addrs {
			addrs = append(addrs, controller.IPAddr{
				IP:    net.ParseIP(addr.Ip),
				Scope: addr.Scope,
			})
		}
		ifaces[iface.Name] = addrs
	}

	// 转换策略模式
	var mode controller.PolicyMode
	switch wl.PolicyMode {
	case "Protect":
		mode = controller.PolicyModeProtect
	default:
		mode = controller.PolicyModeMonitor
	}

	c.workloads[wl.Id] = &WorkloadCache{
		Workload: &controller.Workload{
			ID:         wl.Id,
			Name:       wl.Name,
			HostID:     wl.HostId,
			HostName:   wl.HostName,
			Domain:     wl.Domain,
			Service:    wl.Service,
			Image:      wl.Image,
			PolicyMode: mode,
			Running:    wl.Running,
			Ifaces:     ifaces,
		},
		PolicyMode: mode,
		LastSeenAt: time.Now(),
	}
}

// UpdateConnectionFromProto 从proto更新连接
func (c *Cache) UpdateConnectionFromProto(conn *pb.Connection) {
	if conn == nil {
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	ctrlConn := &controller.Connection{
		ClientWL:     conn.ClientWl,
		ServerWL:     conn.ServerWl,
		ClientIP:     net.IP(conn.ClientIp),
		ServerIP:     net.IP(conn.ServerIp),
		ClientPort:   uint16(conn.ClientPort),
		ServerPort:   uint16(conn.ServerPort),
		IPProto:      uint8(conn.IpProto),
		Application:  conn.Application,
		Bytes:        conn.Bytes,
		Sessions:     conn.Sessions,
		FirstSeenAt:  conn.FirstSeenAt,
		LastSeenAt:   conn.LastSeenAt,
		ThreatID:     conn.ThreatId,
		Severity:     uint8(conn.Severity),
		PolicyAction: uint8(conn.PolicyAction),
		PolicyID:     conn.PolicyId,
		Ingress:      conn.Ingress,
		ExternalPeer: conn.ExternalPeer,
		LocalPeer:    conn.LocalPeer,
	}

	key := ctrlConn.ClientWL + "-" + ctrlConn.ServerWL
	c.connections[key] = &ConnectionCache{
		Connection: ctrlConn,
		GraphKey:   key,
	}

	// 更新网络拓扑图
	attr := &GraphAttr{
		Bytes:        ctrlConn.Bytes,
		Sessions:     ctrlConn.Sessions,
		Severity:     ctrlConn.Severity,
		PolicyAction: ctrlConn.PolicyAction,
	}
	c.wlGraph.AddLink(ctrlConn.ClientWL, "graph", ctrlConn.ServerWL, attr)
}
