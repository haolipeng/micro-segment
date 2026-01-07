// Package network 实现Docker容器流量捕获
package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

const (
	// iptables链名
	NV_INPUT_CHAIN  = "NV_INPUT"
	NV_OUTPUT_CHAIN = "NV_OUTPUT"
	
	// NFQUEUE队列号
	DEFAULT_NFQUEUE_NUM = 0
)

// TrafficCapture 流量捕获管理器
type TrafficCapture struct {
	mutex       sync.RWMutex
	containers  map[string]*ContainerNetInfo // 容器网络信息
	nfqueueNum  int                          // NFQUEUE队列号
	dpConnected bool                         // DP连接状态
}

// ContainerNetInfo 容器网络信息
type ContainerNetInfo struct {
	ID        string            // 容器ID
	Name      string            // 容器名称
	Pid       int               // 容器PID
	Interfaces map[string]*IfaceInfo // 网络接口信息
	Rules     []string          // iptables规则
}

// IfaceInfo 网络接口信息
type IfaceInfo struct {
	Name    string           // 接口名称
	MAC     net.HardwareAddr // MAC地址
	IPs     []net.IP         // IP地址列表
	Peer    string           // veth peer接口名称
	InHost  bool             // 是否在主机命名空间
}

// NewTrafficCapture 创建流量捕获管理器
func NewTrafficCapture() *TrafficCapture {
	tc := &TrafficCapture{
		containers: make(map[string]*ContainerNetInfo),
		nfqueueNum: DEFAULT_NFQUEUE_NUM,
	}
	
	// 初始化iptables链
	if err := tc.initIptablesChains(); err != nil {
		log.WithError(err).Error("Failed to initialize iptables chains")
	}
	
	return tc
}

// initIptablesChains 初始化iptables链
func (tc *TrafficCapture) initIptablesChains() error {
	log.Info("Initializing iptables chains for traffic capture")
	
	// 创建自定义链
	commands := []string{
		// 创建NV_INPUT链
		fmt.Sprintf("iptables -t filter -N %s 2>/dev/null || true", NV_INPUT_CHAIN),
		// 创建NV_OUTPUT链  
		fmt.Sprintf("iptables -t filter -N %s 2>/dev/null || true", NV_OUTPUT_CHAIN),
		// 在INPUT链中跳转到NV_INPUT
		fmt.Sprintf("iptables -t filter -C INPUT -j %s 2>/dev/null || iptables -t filter -I INPUT -j %s", NV_INPUT_CHAIN, NV_INPUT_CHAIN),
		// 在OUTPUT链中跳转到NV_OUTPUT
		fmt.Sprintf("iptables -t filter -C OUTPUT -j %s 2>/dev/null || iptables -t filter -I OUTPUT -j %s", NV_OUTPUT_CHAIN, NV_OUTPUT_CHAIN),
		// 添加RETURN规则到链末尾
		fmt.Sprintf("iptables -t filter -A %s -j RETURN 2>/dev/null || true", NV_INPUT_CHAIN),
		fmt.Sprintf("iptables -t filter -A %s -j RETURN 2>/dev/null || true", NV_OUTPUT_CHAIN),
	}
	
	for _, cmd := range commands {
		if err := tc.executeCommand(cmd); err != nil {
			log.WithFields(log.Fields{"cmd": cmd, "error": err}).Warn("Command failed")
		}
	}
	
	return nil
}

// StartContainerCapture 开始捕获容器流量
func (tc *TrafficCapture) StartContainerCapture(containerID, containerName string, pid int) error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	log.WithFields(log.Fields{
		"container": containerName,
		"id":        containerID,
		"pid":       pid,
	}).Info("Starting container traffic capture")
	
	// 获取容器网络接口信息
	netInfo, err := tc.getContainerNetworkInfo(containerID, containerName, pid)
	if err != nil {
		return fmt.Errorf("failed to get container network info: %v", err)
	}
	
	// 为每个接口设置NFQUEUE规则
	for ifaceName, iface := range netInfo.Interfaces {
		if err := tc.setupNFQueueRules(ifaceName, iface, pid); err != nil {
			log.WithError(err).WithField("interface", ifaceName).Error("Failed to setup NFQUEUE rules")
			continue
		}
	}
	
	tc.containers[containerID] = netInfo
	
	log.WithFields(log.Fields{
		"container":  containerName,
		"interfaces": len(netInfo.Interfaces),
	}).Info("Container traffic capture started")
	
	return nil
}

// StopContainerCapture 停止捕获容器流量
func (tc *TrafficCapture) StopContainerCapture(containerID string) error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	netInfo, exists := tc.containers[containerID]
	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}
	
	log.WithField("container", netInfo.Name).Info("Stopping container traffic capture")
	
	// 删除iptables规则
	for _, rule := range netInfo.Rules {
		deleteCmd := strings.Replace(rule, "-I ", "-D ", 1)
		deleteCmd = strings.Replace(deleteCmd, "-A ", "-D ", 1)
		if err := tc.executeCommand(deleteCmd); err != nil {
			log.WithFields(log.Fields{"rule": deleteCmd, "error": err}).Warn("Failed to delete rule")
		}
	}
	
	delete(tc.containers, containerID)
	
	log.WithField("container", netInfo.Name).Info("Container traffic capture stopped")
	return nil
}

// getContainerNetworkInfo 获取容器网络信息
func (tc *TrafficCapture) getContainerNetworkInfo(containerID, containerName string, pid int) (*ContainerNetInfo, error) {
	netInfo := &ContainerNetInfo{
		ID:         containerID,
		Name:       containerName,
		Pid:        pid,
		Interfaces: make(map[string]*IfaceInfo),
		Rules:      make([]string, 0),
	}
	
	// 进入容器网络命名空间获取接口信息
	cmd := fmt.Sprintf("nsenter -t %d -n ip link show", pid)
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get container interfaces: %v", err)
	}
	
	// 解析网络接口
	interfaces := tc.parseNetworkInterfaces(string(output))
	for _, iface := range interfaces {
		// 跳过loopback接口
		if iface.Name == "lo" {
			continue
		}
		
		// 获取接口IP地址
		ips, err := tc.getInterfaceIPs(pid, iface.Name)
		if err != nil {
			log.WithError(err).WithField("interface", iface.Name).Warn("Failed to get interface IPs")
		}
		iface.IPs = ips
		
		netInfo.Interfaces[iface.Name] = iface
	}
	
	return netInfo, nil
}

// parseNetworkInterfaces 解析网络接口信息
func (tc *TrafficCapture) parseNetworkInterfaces(output string) []*IfaceInfo {
	var interfaces []*IfaceInfo
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ": ") && !strings.HasPrefix(line, " ") {
			// 解析接口行: "2: eth0@if3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500"
			parts := strings.Split(line, ": ")
			if len(parts) >= 2 {
				ifaceName := strings.Split(parts[1], "@")[0]
				
				iface := &IfaceInfo{
					Name: ifaceName,
				}
				
				// 获取MAC地址
				if mac, err := tc.getInterfaceMAC(ifaceName); err == nil {
					iface.MAC = mac
				}
				
				interfaces = append(interfaces, iface)
			}
		}
	}
	
	return interfaces
}

// getInterfaceIPs 获取接口IP地址
func (tc *TrafficCapture) getInterfaceIPs(pid int, ifaceName string) ([]net.IP, error) {
	cmd := fmt.Sprintf("nsenter -t %d -n ip addr show %s", pid, ifaceName)
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return nil, err
	}
	
	var ips []net.IP
	lines := strings.Split(string(output), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			// 解析IP: "inet 172.17.0.2/16 brd 172.17.255.255 scope global eth0"
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				ipStr := strings.Split(parts[1], "/")[0]
				if ip := net.ParseIP(ipStr); ip != nil {
					ips = append(ips, ip)
				}
			}
		}
	}
	
	return ips, nil
}

// getInterfaceMAC 获取接口MAC地址
func (tc *TrafficCapture) getInterfaceMAC(ifaceName string) (net.HardwareAddr, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	return iface.HardwareAddr, nil
}

// setupNFQueueRules 设置NFQUEUE规则
func (tc *TrafficCapture) setupNFQueueRules(ifaceName string, iface *IfaceInfo, pid int) error {
	log.WithField("interface", ifaceName).Debug("Setting up NFQUEUE rules")
	
	// 在容器网络命名空间中设置规则
	rules := []string{
		// 捕获入站流量
		fmt.Sprintf("nsenter -t %d -n iptables -I %s -i %s -j NFQUEUE --queue-num %d --queue-bypass",
			pid, NV_INPUT_CHAIN, ifaceName, tc.nfqueueNum),
		// 捕获出站流量
		fmt.Sprintf("nsenter -t %d -n iptables -I %s -o %s -j NFQUEUE --queue-num %d --queue-bypass",
			pid, NV_OUTPUT_CHAIN, ifaceName, tc.nfqueueNum),
	}
	
	// 执行规则
	for _, rule := range rules {
		if err := tc.executeCommand(rule); err != nil {
			return fmt.Errorf("failed to execute rule %s: %v", rule, err)
		}
		
		// 保存规则用于后续删除
		if netInfo, exists := tc.containers[iface.Name]; exists {
			netInfo.Rules = append(netInfo.Rules, rule)
		}
	}
	
	return nil
}

// executeCommand 执行shell命令
func (tc *TrafficCapture) executeCommand(command string) error {
	log.WithField("cmd", command).Debug("Executing command")
	
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		log.WithFields(log.Fields{
			"cmd":    command,
			"output": string(output),
			"error":  err,
		}).Debug("Command execution failed")
		return err
	}
	
	return nil
}

// SetDPConnected 设置DP连接状态
func (tc *TrafficCapture) SetDPConnected(connected bool) {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	tc.dpConnected = connected
	log.WithField("connected", connected).Info("DP connection status updated")
}

// GetCapturedContainers 获取正在捕获的容器列表
func (tc *TrafficCapture) GetCapturedContainers() []string {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()
	
	var containers []string
	for id, info := range tc.containers {
		containers = append(containers, fmt.Sprintf("%s (%s)", info.Name, id[:12]))
	}
	
	return containers
}

// Cleanup 清理所有规则
func (tc *TrafficCapture) Cleanup() error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	log.Info("Cleaning up traffic capture rules")
	
	// 停止所有容器的流量捕获
	for containerID := range tc.containers {
		if err := tc.StopContainerCapture(containerID); err != nil {
			log.WithError(err).WithField("container", containerID).Warn("Failed to stop container capture")
		}
	}
	
	// 删除自定义链
	cleanupCommands := []string{
		fmt.Sprintf("iptables -t filter -D INPUT -j %s 2>/dev/null || true", NV_INPUT_CHAIN),
		fmt.Sprintf("iptables -t filter -D OUTPUT -j %s 2>/dev/null || true", NV_OUTPUT_CHAIN),
		fmt.Sprintf("iptables -t filter -F %s 2>/dev/null || true", NV_INPUT_CHAIN),
		fmt.Sprintf("iptables -t filter -F %s 2>/dev/null || true", NV_OUTPUT_CHAIN),
		fmt.Sprintf("iptables -t filter -X %s 2>/dev/null || true", NV_INPUT_CHAIN),
		fmt.Sprintf("iptables -t filter -X %s 2>/dev/null || true", NV_OUTPUT_CHAIN),
	}
	
	for _, cmd := range cleanupCommands {
		tc.executeCommand(cmd)
	}
	
	log.Info("Traffic capture cleanup completed")
	return nil
}