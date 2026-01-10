// Package network 网络管理器，整合TC流量捕获和容器监控
package network

import (
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Manager 网络管理器
type Manager struct {
	tcCapture        *TCTrafficCapture
	containerMonitor *ContainerMonitor
	mutex           sync.RWMutex
	running         bool
	stats           *NetworkStats
}

// NetworkStats 网络统计信息
type NetworkStats struct {
	CapturedContainers int       `json:"captured_containers"`
	ActiveRules        int       `json:"active_rules"`
	LastUpdate         time.Time `json:"last_update"`
	TotalPackets       uint64    `json:"total_packets"`
	TotalBytes         uint64    `json:"total_bytes"`
}

// NewManager 创建网络管理器
// 初始化TC流量捕获和容器监控组件
func NewManager() (*Manager, error) {
	log.Info("Initializing TC-based network manager")
	
	// 创建TC流量捕获器
	tcCapture := NewTCTrafficCapture()
	
	// 创建容器监控器
	containerMonitor, err := NewContainerMonitor(tcCapture)
	if err != nil {
		return nil, fmt.Errorf("failed to create container monitor: %v", err)
	}
	
	manager := &Manager{
		tcCapture:        tcCapture,
		containerMonitor: containerMonitor,
		stats: &NetworkStats{
			LastUpdate: time.Now(),
		},
	}
	
	return manager, nil
}

// Start 启动网络管理器
// 启动容器监控和统计更新循环
func (m *Manager) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if m.running {
		return fmt.Errorf("network manager is already running")
	}
	
	log.Info("Starting TC-based network manager")
	
	// 启动容器监控
	if err := m.containerMonitor.Start(); err != nil {
		return fmt.Errorf("failed to start container monitor: %v", err)
	}
	
	// 启动统计更新
	go m.statsUpdateLoop()
	
	m.running = true
	
	log.Info("TC-based network manager started successfully")
	return nil
}

// Stop 停止网络管理器
// 停止监控并清理TC流量捕获规则
func (m *Manager) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	
	if !m.running {
		return nil
	}
	
	log.Info("Stopping TC-based network manager")
	
	// 停止容器监控
	if err := m.containerMonitor.Stop(); err != nil {
		log.WithError(err).Warn("Failed to stop container monitor")
	}
	
	// 清理TC流量捕获规则
	if err := m.tcCapture.Cleanup(); err != nil {
		log.WithError(err).Warn("Failed to cleanup TC traffic capture")
	}
	
	m.running = false
	
	log.Info("TC-based network manager stopped")
	return nil
}

// IsRunning 检查管理器是否运行中
// 线程安全地返回管理器运行状态
func (m *Manager) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// SetDPConnected 设置DP连接状态
// 更新DP连接状态，TC方案通过bridge mirror数据包
func (m *Manager) SetDPConnected(connected bool) {
	log.WithField("connected", connected).Info("DP connection status updated for TC capture")
	// TC方案不需要特殊的DP连接处理，因为数据包通过bridge mirror到DP
}

// GetStats 获取网络统计信息
// 返回当前网络捕获和处理统计数据
func (m *Manager) GetStats() *NetworkStats {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// 更新统计信息
	m.updateStats()
	
	return m.stats
}

// GetCapturedContainers 获取正在捕获的容器列表
// 返回当前配置了TC规则的容器ID列表
func (m *Manager) GetCapturedContainers() []string {
	return m.tcCapture.GetCapturedContainers()
}

// GetRunningContainers 获取运行中的容器列表
// 从Docker API获取当前运行的容器信息
func (m *Manager) GetRunningContainers() ([]*ContainerEvent, error) {
	return m.containerMonitor.ListRunningContainers()
}

// GetContainerInfo 获取容器信息
// 查询指定容器的详细信息和网络配置
func (m *Manager) GetContainerInfo(containerID string) (*ContainerEvent, error) {
	return m.containerMonitor.GetContainerInfo(containerID)
}

// ForceStartCapture 强制开始捕获指定容器
// 手动为指定容器创建veth pair和TC规则
func (m *Manager) ForceStartCapture(containerID string) error {
	containerInfo, err := m.containerMonitor.GetContainerInfo(containerID)
	if err != nil {
		return fmt.Errorf("failed to get container info: %v", err)
	}
	
	if containerInfo.Pid <= 0 {
		return fmt.Errorf("container %s has no valid PID", containerID)
	}
	
	return m.tcCapture.StartContainerCapture(containerID, containerInfo.Name, containerInfo.Pid)
}

// ForceStopCapture 强制停止捕获指定容器
// 手动清理指定容器的TC规则和veth pair
func (m *Manager) ForceStopCapture(containerID string) error {
	return m.tcCapture.StopContainerCapture(containerID)
}

// statsUpdateLoop 统计信息更新循环
// 定期更新网络捕获统计数据
func (m *Manager) statsUpdateLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			if !m.IsRunning() {
				return
			}
			m.updateStats()
		}
	}
}

// updateStats 更新统计信息
// 收集当前捕获状态和性能数据
func (m *Manager) updateStats() {
	capturedContainers := m.tcCapture.GetCapturedContainers()
	
	m.stats.CapturedContainers = len(capturedContainers)
	m.stats.LastUpdate = time.Now()
	
	// TODO: 从DP获取实际的包和字节统计
	// m.stats.TotalPackets = dpStats.TotalPackets
	// m.stats.TotalBytes = dpStats.TotalBytes
}

// GetNetworkTopology 获取网络拓扑信息
// 返回容器网络拓扑和捕获状态概览
func (m *Manager) GetNetworkTopology() (map[string]interface{}, error) {
	containers, err := m.GetRunningContainers()
	if err != nil {
		return nil, err
	}
	
	topology := map[string]interface{}{
		"containers": containers,
		"captured":   m.GetCapturedContainers(),
		"stats":      m.GetStats(),
		"timestamp":  time.Now(),
		"method":     "traffic_control",
	}
	
	return topology, nil
}

// ValidateSetup 验证网络设置
// 检查TC、Docker等必需工具的可用性
func (m *Manager) ValidateSetup() error {
	log.Info("Validating TC-based network setup")
	
	// 检查tc命令是否可用
	if err := m.tcCapture.executeCommand("tc -Version"); err != nil {
		return fmt.Errorf("tc command not available: %v", err)
	}
	
	// 检查ip命令是否可用
	if err := m.tcCapture.executeCommand("ip -Version"); err != nil {
		return fmt.Errorf("ip command not available: %v", err)
	}
	
	// 检查nsenter是否可用
	if err := m.tcCapture.executeCommand("nsenter --version"); err != nil {
		return fmt.Errorf("nsenter not available: %v", err)
	}
	
	// 检查ethtool是否可用
	if err := m.tcCapture.executeCommand("ethtool --version"); err != nil {
		log.Warn("ethtool not available, network offload features cannot be disabled")
	}
	
	// 检查Docker是否可用
	containers, err := m.containerMonitor.ListRunningContainers()
	if err != nil {
		return fmt.Errorf("docker not accessible: %v", err)
	}
	
	log.WithField("containers", len(containers)).Info("TC-based network setup validation passed")
	return nil
}

// GetDebugInfo 获取调试信息
// 返回详细的网络管理器状态和配置信息
func (m *Manager) GetDebugInfo() map[string]interface{} {
	debugInfo := map[string]interface{}{
		"running":             m.IsRunning(),
		"captured_containers": m.GetCapturedContainers(),
		"stats":              m.GetStats(),
		"timestamp":          time.Now(),
		"method":             "traffic_control",
		"bridge_ready":       m.tcCapture.bridgeReady,
	}
	
	// 获取运行中的容器
	if containers, err := m.GetRunningContainers(); err == nil {
		debugInfo["running_containers"] = containers
	}
	
	return debugInfo
}