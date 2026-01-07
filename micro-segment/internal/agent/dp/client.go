// Package dp 提供与DP层通信的客户端
// 从NeuVector agent/dp简化提取
package dp

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
)

// DPClient DP层客户端
type DPClient struct {
	mutex      sync.Mutex
	socketPath string
	conn       net.Conn
	connected  bool

	// 回调
	onConnection func(*DPConnection)
	onThreatLog  func(*DPThreatLog)
}

// DPConnection DP连接数据
type DPConnection struct {
	ClientIP     net.IP
	ServerIP     net.IP
	ClientPort   uint16
	ServerPort   uint16
	IPProto      uint8
	Application  uint32
	Bytes        uint64
	Sessions     uint32
	FirstSeenAt  uint32
	LastSeenAt   uint32
	ThreatID     uint32
	Severity     uint8
	PolicyAction uint8
	PolicyId     uint32
	Ingress      bool
	ExternalPeer bool
	EPMAC        net.HardwareAddr
}

// DPThreatLog DP威胁日志
type DPThreatLog struct {
	ThreatID   uint32
	Severity   uint8
	ClientIP   net.IP
	ServerIP   net.IP
	ServerPort uint16
	IPProto    uint8
	PktIngress bool
	EPMAC      net.HardwareAddr
}

// DPPolicy DP策略
type DPPolicy struct {
	ID           uint32
	SrcIP        net.IP
	DstIP        net.IP
	SrcIPMask    net.IPMask
	DstIPMask    net.IPMask
	Port         uint16
	PortMask     uint16
	IPProto      uint8
	Action       uint8
	Ingress      bool
	Application  uint32
}

// NewDPClient 创建DP客户端
func NewDPClient(socketPath string) *DPClient {
	return &DPClient{
		socketPath: socketPath,
	}
}

// Connect 连接到DP
func (c *DPClient) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.connected {
		return nil
	}

	// DP uses Unix datagram socket (SOCK_DGRAM), so we use "unixgram"
	addr := &net.UnixAddr{Name: c.socketPath, Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to connect to DP: %v", err)
	}

	c.conn = conn
	c.connected = true

	go c.readLoop()

	log.WithField("socket", c.socketPath).Info("Connected to DP")
	return nil
}

// Disconnect 断开连接
func (c *DPClient) Disconnect() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return
	}

	c.conn.Close()
	c.connected = false
}

// IsConnected 检查是否已连接
func (c *DPClient) IsConnected() bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.connected
}

// SetOnConnection 设置连接回调
func (c *DPClient) SetOnConnection(cb func(*DPConnection)) {
	c.onConnection = cb
}

// SetOnThreatLog 设置威胁日志回调
func (c *DPClient) SetOnThreatLog(cb func(*DPThreatLog)) {
	c.onThreatLog = cb
}

// readLoop 读取循环
func (c *DPClient) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			log.WithError(err).Error("DP read error")
			c.mutex.Lock()
			c.connected = false
			c.mutex.Unlock()
			return
		}

		c.handleMessage(buf[:n])
	}
}

// handleMessage 处理消息
func (c *DPClient) handleMessage(data []byte) {
	var msg DPMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.WithError(err).Error("Failed to parse DP message")
		return
	}

	switch msg.Type {
	case "connection":
		if c.onConnection != nil {
			var conn DPConnection
			if err := json.Unmarshal(msg.Data, &conn); err == nil {
				c.onConnection(&conn)
			}
		}
	case "threat":
		if c.onThreatLog != nil {
			var threat DPThreatLog
			if err := json.Unmarshal(msg.Data, &threat); err == nil {
				c.onThreatLog(&threat)
			}
		}
	}
}

// DPMessage DP消息
type DPMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// SendPolicy 发送策略到DP
func (c *DPClient) SendPolicy(policies []*DPPolicy) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected to DP")
	}

	msg := struct {
		Type     string      `json:"type"`
		Policies []*DPPolicy `json:"policies"`
	}{
		Type:     "policy",
		Policies: policies,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(data)
	return err
}

// AddMAC 添加MAC地址
func (c *DPClient) AddMAC(mac net.HardwareAddr, workloadID string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected to DP")
	}

	msg := struct {
		Type       string `json:"type"`
		MAC        string `json:"mac"`
		WorkloadID string `json:"workload_id"`
	}{
		Type:       "add_mac",
		MAC:        mac.String(),
		WorkloadID: workloadID,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(data)
	return err
}

// DelMAC 删除MAC地址
func (c *DPClient) DelMAC(mac net.HardwareAddr) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected to DP")
	}

	msg := struct {
		Type string `json:"type"`
		MAC  string `json:"mac"`
	}{
		Type: "del_mac",
		MAC:  mac.String(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(data)
	return err
}

// ConfigSubnets 配置内部子网
func (c *DPClient) ConfigSubnets(subnets []net.IPNet) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.connected {
		return fmt.Errorf("not connected to DP")
	}

	subnetStrs := make([]string, len(subnets))
	for i, subnet := range subnets {
		subnetStrs[i] = subnet.String()
	}

	msg := struct {
		Type    string   `json:"type"`
		Subnets []string `json:"subnets"`
	}{
		Type:    "config_subnets",
		Subnets: subnetStrs,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.conn.Write(data)
	return err
}
