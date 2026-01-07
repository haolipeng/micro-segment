// Package grpc 提供Agent的gRPC客户端
package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	log "github.com/sirupsen/logrus"

	pb "github.com/micro-segment/api/proto"
	"github.com/micro-segment/internal/agent"
)

// Client gRPC客户端
type Client struct {
	mutex      sync.RWMutex
	conn       *grpc.ClientConn
	client     pb.ControllerServiceClient
	serverAddr string
	connected  bool

	// Agent信息
	agentID  string
	hostID   string
	hostName string
	version  string

	// 心跳
	heartbeatInterval time.Duration
	stopCh            chan struct{}
}

// NewClient 创建gRPC客户端
func NewClient(serverAddr, agentID, hostID, hostName, version string) *Client {
	return &Client{
		serverAddr:        serverAddr,
		agentID:           agentID,
		hostID:            hostID,
		hostName:          hostName,
		version:           version,
		heartbeatInterval: 10 * time.Second,
		stopCh:            make(chan struct{}),
	}
}

// Connect 连接到Controller
func (c *Client) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected {
		return nil
	}

	conn, err := grpc.Dial(c.serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}

	c.conn = conn
	c.client = pb.NewControllerServiceClient(conn)
	c.connected = true

	log.WithField("server", c.serverAddr).Info("Connected to Controller")
	return nil
}

// Disconnect 断开连接
func (c *Client) Disconnect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return
	}

	close(c.stopCh)
	c.conn.Close()
	c.connected = false
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.connected
}

// Register 注册Agent
func (c *Client) Register() error {
	c.mutex.RLock()
	if !c.connected {
		c.mutex.RUnlock()
		return fmt.Errorf("not connected")
	}
	client := c.client
	c.mutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Register(ctx, &pb.AgentInfo{
		AgentId:  c.agentID,
		HostId:   c.hostID,
		HostName: c.hostName,
		Version:  c.version,
	})
	if err != nil {
		return fmt.Errorf("register failed: %v", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("register failed: %s", resp.Message)
	}

	log.WithFields(log.Fields{
		"cluster_id":      resp.ClusterId,
		"report_interval": resp.ReportInterval,
	}).Info("Agent registered")

	// 启动心跳
	go c.heartbeatLoop()

	return nil
}

// heartbeatLoop 心跳循环
func (c *Client) heartbeatLoop() {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.sendHeartbeat()
		case <-c.stopCh:
			return
		}
	}
}

// sendHeartbeat 发送心跳
func (c *Client) sendHeartbeat() {
	c.mutex.RLock()
	if !c.connected {
		c.mutex.RUnlock()
		return
	}
	client := c.client
	c.mutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.Heartbeat(ctx, &pb.HeartbeatRequest{
		AgentId:   c.agentID,
		Timestamp: uint64(time.Now().Unix()),
	})
	if err != nil {
		log.WithError(err).Warn("Heartbeat failed")
	}
}

// ReportConnections 上报连接
func (c *Client) ReportConnections(conns []*agent.Connection) error {
	c.mutex.RLock()
	if !c.connected {
		c.mutex.RUnlock()
		return fmt.Errorf("not connected")
	}
	client := c.client
	c.mutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pbConns := make([]*pb.Connection, 0, len(conns))
	for _, conn := range conns {
		pbConns = append(pbConns, &pb.Connection{
			ClientWl:     conn.ClientWL,
			ServerWl:     conn.ServerWL,
			ClientIp:     conn.ClientIP,
			ServerIp:     conn.ServerIP,
			ClientPort:   uint32(conn.ClientPort),
			ServerPort:   uint32(conn.ServerPort),
			IpProto:      uint32(conn.IPProto),
			Application:  conn.Application,
			Bytes:        conn.Bytes,
			Sessions:     conn.Sessions,
			FirstSeenAt:  conn.FirstSeenAt,
			LastSeenAt:   conn.LastSeenAt,
			ThreatId:     conn.ThreatID,
			Severity:     uint32(conn.Severity),
			PolicyAction: uint32(conn.PolicyAction),
			PolicyId:     conn.PolicyId,
			Ingress:      conn.Ingress,
			ExternalPeer: conn.ExternalPeer,
			LocalPeer:    conn.LocalPeer,
			Scope:        conn.Scope,
			Network:      conn.Network,
			Violates:     conn.Violates,
		})
	}

	resp, err := client.ReportConnections(ctx, &pb.ConnectionReport{
		AgentId:     c.agentID,
		HostId:      c.hostID,
		Connections: pbConns,
	})
	if err != nil {
		return fmt.Errorf("report connections failed: %v", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("report connections failed: %s", resp.Message)
	}

	return nil
}

// ReportThreats 上报威胁日志
func (c *Client) ReportThreats(threats []*agent.ThreatLog) error {
	c.mutex.RLock()
	if !c.connected {
		c.mutex.RUnlock()
		return fmt.Errorf("not connected")
	}
	client := c.client
	c.mutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pbThreats := make([]*pb.ThreatLog, 0, len(threats))
	for _, threat := range threats {
		pbThreats = append(pbThreats, &pb.ThreatLog{
			Id:         threat.ID,
			ThreatId:   threat.ThreatID,
			ThreatName: threat.ThreatName,
			Severity:   threat.Severity,
			ClientWl:   threat.ClientWL,
			ServerWl:   threat.ServerWL,
			ClientIp:   threat.ClientIP,
			ServerIp:   threat.ServerIP,
			ServerPort: uint32(threat.ServerPort),
			IpProto:    uint32(threat.IPProto),
			PktIngress: threat.PktIngress,
			LocalPeer:  threat.LocalPeer,
			ReportedAt: uint64(threat.ReportedAt.Unix()),
		})
	}

	resp, err := client.ReportThreats(ctx, &pb.ThreatReport{
		AgentId: c.agentID,
		HostId:  c.hostID,
		Threats: pbThreats,
	})
	if err != nil {
		return fmt.Errorf("report threats failed: %v", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("report threats failed: %s", resp.Message)
	}

	return nil
}

// ReportWorkload 上报工作负载变更
func (c *Client) ReportWorkload(eventType string, wl *agent.Workload) error {
	c.mutex.RLock()
	if !c.connected {
		c.mutex.RUnlock()
		return fmt.Errorf("not connected")
	}
	client := c.client
	c.mutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 转换接口
	ifaces := make([]*pb.NetworkInterface, 0)
	for name, addrs := range wl.Ifaces {
		pbAddrs := make([]*pb.IPAddress, 0, len(addrs))
		for _, addr := range addrs {
			pbAddrs = append(pbAddrs, &pb.IPAddress{
				Ip:      addr.IP.String(),
				Scope:   addr.Scope,
				Gateway: addr.Gateway,
			})
		}
		ifaces = append(ifaces, &pb.NetworkInterface{
			Name:  name,
			Addrs: pbAddrs,
		})
	}

	resp, err := client.ReportWorkload(ctx, &pb.WorkloadEvent{
		AgentId:   c.agentID,
		EventType: eventType,
		Workload: &pb.Workload{
			Id:         wl.ID,
			Name:       wl.Name,
			HostId:     wl.HostID,
			HostName:   wl.HostName,
			Domain:     wl.Domain,
			Service:    wl.Service,
			PolicyMode: string(wl.PolicyMode),
			Running:    wl.Running,
			Pid:        int32(wl.Pid),
			Ifaces:     ifaces,
		},
	})
	if err != nil {
		return fmt.Errorf("report workload failed: %v", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("report workload failed: %s", resp.Message)
	}

	return nil
}

// GetPolicies 获取策略
func (c *Client) GetPolicies(workloadIDs []string) ([]*agent.PolicyRule, error) {
	c.mutex.RLock()
	if !c.connected {
		c.mutex.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	client := c.client
	c.mutex.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.GetPolicies(ctx, &pb.PolicyRequest{
		AgentId:     c.agentID,
		WorkloadIds: workloadIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("get policies failed: %v", err)
	}

	rules := make([]*agent.PolicyRule, 0, len(resp.Rules))
	for _, r := range resp.Rules {
		rules = append(rules, &agent.PolicyRule{
			ID:           r.Id,
			From:         r.From,
			To:           r.To,
			Ports:        r.Ports,
			Applications: r.Applications,
			Action:       agent.PolicyAction(r.Action),
			Ingress:      r.Ingress,
		})
	}

	return rules, nil
}

// ipToBytes 转换IP为字节
func ipToBytes(ip net.IP) []byte {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4
	}
	return ip
}
