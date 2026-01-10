// Package network 实现基于Traffic Control的Docker容器流量捕获
// 基于NeuVector的真实实现方式
package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
)

const (
	// NeuVector bridge接口名称
	NV_BRIDGE_NAME = "nv-br"
	
	// TC优先级基础值
	TC_PREF_BASE = 10000
	TC_PREF_MAX  = 65536
)

// TCTrafficCapture 基于Traffic Control的流量捕获管理器
type TCTrafficCapture struct {
	mutex       sync.RWMutex
	containers  map[string]*TCContainerInfo // 容器网络信息
	prefs       map[uint]bool               // TC优先级使用情况
	portMap     map[string]*TCPortInfo      // 端口映射信息
	bridgeReady bool                        // Bridge是否就绪
}

// TCContainerInfo 容器网络信息
type TCContainerInfo struct {
	ID         string                    // 容器ID
	Name       string                    // 容器名称
	Pid        int                       // 容器PID
	VethPairs  map[string]*VethPairInfo  // veth pair信息
	TCRules    []string                  // TC规则列表
}

// VethPairInfo veth pair信息
type VethPairInfo struct {
	OriginalName string           // 原始接口名称
	InternalName string           // 内部接口名称（容器内）
	ExternalName string           // 外部接口名称（主机侧）
	OriginalMAC  net.HardwareAddr // 原始MAC地址
	NVMAC        net.HardwareAddr // NeuVector分配的MAC地址
	BroadcastMAC net.HardwareAddr // 广播MAC地址
	Index        uint             // 接口索引
}

// TCPortInfo TC端口信息
type TCPortInfo struct {
	Index uint // 端口索引
	Pref  uint // TC优先级
}

// NewTCTrafficCapture 创建TC流量捕获管理器
// 初始化容器映射和NeuVector bridge
func NewTCTrafficCapture() *TCTrafficCapture {
	tc := &TCTrafficCapture{
		containers: make(map[string]*TCContainerInfo),
		prefs:      make(map[uint]bool),
		portMap:    make(map[string]*TCPortInfo),
	}
	
	// 初始化NeuVector bridge
	if err := tc.initNVBridge(); err != nil {
		log.WithError(err).Error("Failed to initialize NV bridge")
	}
	
	return tc
}

// initNVBridge 初始化NeuVector bridge
// 创建nv-br网桥用于接收mirror流量
func (tc *TCTrafficCapture) initNVBridge() error {
	log.Info("Initializing NeuVector bridge for traffic capture")
	
	// 检查bridge是否已存在
	if link, err := netlink.LinkByName(NV_BRIDGE_NAME); err == nil {
		// 清理现有bridge
		tc.cleanupBridge(link)
	}
	
	// 创建新的bridge
	bridge := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{
			Name: NV_BRIDGE_NAME,
			MTU:  1500,
		},
	}
	
	if err := netlink.LinkAdd(bridge); err != nil {
		return fmt.Errorf("failed to create bridge: %v", err)
	}
	
	// 启用bridge
	if err := netlink.LinkSetUp(bridge); err != nil {
		return fmt.Errorf("failed to bring up bridge: %v", err)
	}
	
	// 添加ingress qdisc到bridge
	if err := tc.addQDisc(NV_BRIDGE_NAME); err != nil {
		log.WithError(err).Warn("Failed to add qdisc to bridge")
	}
	
	// 禁用offload功能
	tc.disableOffload(NV_BRIDGE_NAME)
	
	tc.bridgeReady = true
	log.Info("NeuVector bridge initialized successfully")
	
	return nil
}

// cleanupBridge 清理bridge
// 删除qdisc和bridge接口
func (tc *TCTrafficCapture) cleanupBridge(bridge netlink.Link) {
	// 删除qdisc
	tc.delQDisc(NV_BRIDGE_NAME)
	
	// 关闭bridge
	netlink.LinkSetDown(bridge)
	
	// 删除bridge
	netlink.LinkDel(bridge)
}

// addQDisc 添加ingress qdisc
// 为指定接口添加入口流量控制队列
func (tc *TCTrafficCapture) addQDisc(port string) error {
	cmd := fmt.Sprintf("tc qdisc add dev %s ingress", port)
	return tc.executeCommand(cmd)
}

// addQDiscInNamespace 在指定网络命名空间中添加ingress qdisc
// 在容器网络命名空间中配置流量控制队列
func (tc *TCTrafficCapture) addQDiscInNamespace(pid int, port string) error {
	cmd := fmt.Sprintf("nsenter -t %d -n tc qdisc add dev %s ingress", pid, port)
	return tc.executeCommand(cmd)
}

// delQDisc 删除ingress qdisc
// 移除指定接口的入口流量控制队列
func (tc *TCTrafficCapture) delQDisc(port string) error {
	cmd := fmt.Sprintf("tc qdisc del dev %s ingress", port)
	return tc.executeCommand(cmd)
}

// disableOffload 禁用网络offload功能
// 关闭硬件加速功能确保TC规则正常工作
func (tc *TCTrafficCapture) disableOffload(port string) {
	offloadFeatures := []string{
		"rx-checksumming",
		"tx-checksumming", 
		"scatter-gather",
		"tcp-segmentation-offload",
		"udp-fragmentation-offload",
		"generic-segmentation-offload",
		"generic-receive-offload",
		"large-receive-offload",
	}
	
	for _, feature := range offloadFeatures {
		cmd := fmt.Sprintf("ethtool -K %s %s off", port, feature)
		tc.executeCommand(cmd) // 忽略错误
	}
}

// StartContainerCapture 开始捕获容器流量
// 为容器创建veth pair和TC mirror规则
func (tc *TCTrafficCapture) StartContainerCapture(containerID, containerName string, pid int) error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	if !tc.bridgeReady {
		return fmt.Errorf("NV bridge not ready")
	}
	
	log.WithFields(log.Fields{
		"container": containerName,
		"id":        containerID,
		"pid":       pid,
	}).Info("Starting TC-based container traffic capture")
	
	// 检查是否已经在捕获
	if _, exists := tc.containers[containerID]; exists {
		log.WithField("container", containerName).Debug("Container already being captured")
		return nil
	}
	
	// 清理可能存在的旧veth pair
	tc.cleanupContainerInterfaces(pid)
	
	// 获取容器网络接口
	interfaces, err := tc.getContainerInterfaces(pid)
	if err != nil {
		return fmt.Errorf("failed to get container interfaces: %v", err)
	}
	
	containerInfo := &TCContainerInfo{
		ID:        containerID,
		Name:      containerName,
		Pid:       pid,
		VethPairs: make(map[string]*VethPairInfo),
		TCRules:   make([]string, 0),
	}
	
	// 为每个接口创建veth pair和TC规则
	for _, iface := range interfaces {
		if iface == "lo" {
			continue // 跳过loopback接口
		}
		
		vethPair, err := tc.createVethPair(pid, iface, containerInfo)
		if err != nil {
			log.WithError(err).WithField("interface", iface).Error("Failed to create veth pair")
			continue
		}
		
		containerInfo.VethPairs[iface] = vethPair
		
		// 设置TC规则
		if err := tc.setupTCRules(vethPair, containerInfo); err != nil {
			log.WithError(err).WithField("interface", iface).Error("Failed to setup TC rules")
		}
	}
	
	tc.containers[containerID] = containerInfo
	
	log.WithFields(log.Fields{
		"container":   containerName,
		"veth_pairs":  len(containerInfo.VethPairs),
		"tc_rules":    len(containerInfo.TCRules),
	}).Info("Container traffic capture started")
	
	return nil
}

// getContainerInterfaces 获取容器网络接口列表
// 解析容器内的网络接口名称
func (tc *TCTrafficCapture) getContainerInterfaces(pid int) ([]string, error) {
	cmd := fmt.Sprintf("nsenter -t %d -n ip link show", pid)
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return nil, err
	}
	
	var interfaces []string
	lines := strings.Split(string(output), "\n")
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ": ") && !strings.HasPrefix(line, " ") {
			// 解析接口行: "2: eth0@if3: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500"
			parts := strings.Split(line, ": ")
			if len(parts) >= 2 {
				ifaceName := strings.Split(parts[1], "@")[0]
				interfaces = append(interfaces, ifaceName)
			}
		}
	}
	
	return interfaces, nil
}

// createVethPair 创建veth pair
// 为容器接口创建对应的veth pair用于流量mirror
func (tc *TCTrafficCapture) createVethPair(pid int, originalIface string, containerInfo *TCContainerInfo) (*VethPairInfo, error) {
	log.WithField("interface", originalIface).Debug("Creating veth pair")
	
	// 生成接口名称
	internalName := fmt.Sprintf("nv-in-%s", originalIface)
	externalName := fmt.Sprintf("nv-ex-%s", originalIface)
	
	// 获取原始接口信息
	originalMAC, err := tc.getInterfaceMAC(pid, originalIface)
	if err != nil {
		return nil, fmt.Errorf("failed to get original MAC: %v", err)
	}
	
	// 获取可用的接口索引
	index := tc.getAvailableIndex()
	
	// 生成NeuVector MAC地址 (4e:65:75:56 - "NeuV")
	nvMAC := net.HardwareAddr{
		0x4e, 0x65, 0x75, 0x56,
		uint8((index >> 8) & 0xff),
		uint8(index & 0xff),
	}
	
	// 生成广播MAC地址
	bcMAC := net.HardwareAddr{
		0xff, 0xff, 0xff, 0x00,
		uint8((index >> 8) & 0xff),
		uint8(index & 0xff),
	}
	
	// 在容器命名空间中重命名原始接口
	if err := tc.renameInterface(pid, originalIface, externalName); err != nil {
		return nil, fmt.Errorf("failed to rename interface: %v", err)
	}
	
	// 创建veth pair
	if err := tc.createVethPairInNamespace(pid, originalIface, internalName, index); err != nil {
		return nil, fmt.Errorf("failed to create veth pair: %v", err)
	}
	
	// 配置接口
	if err := tc.configureVethPair(pid, originalIface, internalName, externalName, originalMAC, nvMAC); err != nil {
		return nil, fmt.Errorf("failed to configure veth pair: %v", err)
	}
	
	vethPair := &VethPairInfo{
		OriginalName: originalIface,
		InternalName: internalName,
		ExternalName: externalName,
		OriginalMAC:  originalMAC,
		NVMAC:        nvMAC,
		BroadcastMAC: bcMAC,
		Index:        index,
	}
	
	return vethPair, nil
}

// getInterfaceMAC 获取接口MAC地址
// 从容器网络命名空间获取接口MAC地址
func (tc *TCTrafficCapture) getInterfaceMAC(pid int, iface string) (net.HardwareAddr, error) {
	// 方法1: 尝试从/sys/class/net读取
	cmd := fmt.Sprintf("nsenter -t %d -n cat /sys/class/net/%s/address", pid, iface)
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err == nil {
		macStr := strings.TrimSpace(string(output))
		return net.ParseMAC(macStr)
	}
	
	// 方法2: 从ip link show解析MAC地址
	cmd = fmt.Sprintf("nsenter -t %d -n ip link show %s", pid, iface)
	output, err = exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get interface info: %v", err)
	}
	
	// 解析输出: "2: eth0@if12: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP"
	//          "    link/ether 56:7e:4d:73:ab:e8 brd ff:ff:ff:ff:ff:ff link-netnsid 0"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "link/ether ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return net.ParseMAC(parts[1])
			}
		}
	}
	
	return nil, fmt.Errorf("MAC address not found in ip link output")
}

// getAvailableIndex 获取可用的接口索引
// 分配唯一的接口索引用于MAC地址生成
func (tc *TCTrafficCapture) getAvailableIndex() uint {
	for i := uint(1); i < TC_PREF_MAX; i++ {
		if !tc.prefs[i] {
			tc.prefs[i] = true
			return i
		}
	}
	return 0
}

// renameInterface 重命名接口
// 在容器命名空间中重命名网络接口
func (tc *TCTrafficCapture) renameInterface(pid int, oldName, newName string) error {
	cmd := fmt.Sprintf("nsenter -t %d -n ip link set %s down", pid, oldName)
	if err := tc.executeCommand(cmd); err != nil {
		return err
	}
	
	cmd = fmt.Sprintf("nsenter -t %d -n ip link set %s name %s", pid, oldName, newName)
	return tc.executeCommand(cmd)
}

// createVethPairInNamespace 在命名空间中创建veth pair
// 创建veth pair并将peer端移动到主机命名空间
func (tc *TCTrafficCapture) createVethPairInNamespace(pid int, localName, peerName string, index uint) error {
	// 在容器命名空间中创建veth pair
	cmd := fmt.Sprintf("nsenter -t %d -n ip link add %s type veth peer name %s", 
		pid, localName, peerName)
	if err := tc.executeCommand(cmd); err != nil {
		return err
	}
	
	// 将peer接口移动到主机网络命名空间
	cmd = fmt.Sprintf("nsenter -t %d -n ip link set %s netns 1", pid, peerName)
	return tc.executeCommand(cmd)
}

// configureVethPair 配置veth pair
// 设置MAC地址、IP配置和bridge连接
func (tc *TCTrafficCapture) configureVethPair(pid int, localName, peerName, externalName string, 
	originalMAC, nvMAC net.HardwareAddr) error {
	
	// 获取原始接口的IP配置
	ipConfig, err := tc.getInterfaceIPConfig(pid, externalName)
	if err != nil {
		log.WithError(err).Warn("Failed to get IP config, network may not work properly")
	}
	
	// 配置本地接口（容器内）
	commands := []string{
		// 设置MAC地址
		fmt.Sprintf("nsenter -t %d -n ip link set %s address %s", 
			pid, localName, originalMAC.String()),
		// 启用接口
		fmt.Sprintf("nsenter -t %d -n ip link set %s up", pid, localName),
		
		// 启用外部接口（容器内）
		fmt.Sprintf("nsenter -t %d -n ip link set %s up", pid, externalName),
	}
	
	// 如果获取到IP配置，将其应用到新的eth0接口
	if ipConfig != nil {
		// 将IP地址从nv-ex-eth0移动到eth0
		if ipConfig.IPAddr != "" {
			commands = append(commands, 
				fmt.Sprintf("nsenter -t %d -n ip addr del %s dev %s", pid, ipConfig.IPAddr, externalName),
				fmt.Sprintf("nsenter -t %d -n ip addr add %s dev %s", pid, ipConfig.IPAddr, localName),
			)
		}
		// 恢复默认路由
		if ipConfig.Gateway != "" {
			commands = append(commands, 
				fmt.Sprintf("nsenter -t %d -n ip route add default via %s dev %s", pid, ipConfig.Gateway, localName),
			)
		}
	}
	
	// 配置peer接口（主机侧）
	hostCommands := []string{
		fmt.Sprintf("ip link set %s address %s", peerName, nvMAC.String()),
		fmt.Sprintf("ip link set %s up", peerName),
		fmt.Sprintf("ip link set %s master %s", peerName, NV_BRIDGE_NAME),
	}
	
	// 执行容器内命令
	for _, cmd := range commands {
		if err := tc.executeCommand(cmd); err != nil {
			log.WithFields(log.Fields{"cmd": cmd, "error": err}).Warn("Container command failed")
		}
	}
	
	// 执行主机命令
	for _, cmd := range hostCommands {
		if err := tc.executeCommand(cmd); err != nil {
			log.WithFields(log.Fields{"cmd": cmd, "error": err}).Warn("Host command failed")
		}
	}
	
	return nil
}

// setupTCRules 设置Traffic Control规则
// 配置流量mirror规则将数据包复制到NV bridge
func (tc *TCTrafficCapture) setupTCRules(vethPair *VethPairInfo, containerInfo *TCContainerInfo) error {
	log.WithField("interface", vethPair.OriginalName).Debug("Setting up TC rules")
	
	// 为容器内的接口添加qdisc
	tc.addQDiscInNamespace(containerInfo.Pid, vethPair.OriginalName)
	tc.addQDiscInNamespace(containerInfo.Pid, vethPair.ExternalName)
	
	// 为主机侧接口添加qdisc
	tc.addQDisc(vethPair.InternalName) // 这个现在在主机侧
	
	// 获取TC优先级
	pref := tc.getAvailablePref(vethPair.Index)
	if pref == 0 {
		return fmt.Errorf("no available TC preference")
	}
	
	tc.portMap[vethPair.InternalName] = &TCPortInfo{
		Index: vethPair.Index,
		Pref:  pref,
	}
	tc.portMap[vethPair.ExternalName] = &TCPortInfo{
		Index: vethPair.Index,
		Pref:  pref,
	}
	
	// 设置容器内的TC规则（外部→内部）
	ingressRules := []string{
		fmt.Sprintf("nsenter -t %d -n tc filter add dev %s pref %d parent ffff: protocol all "+
			"u32 match u8 0 0 "+
			"action mirred egress mirror dev %s",
			containerInfo.Pid, vethPair.ExternalName, TC_PREF_BASE+2, vethPair.OriginalName),
	}
	
	// 设置容器内的TC规则（内部→外部）
	egressRules := []string{
		fmt.Sprintf("nsenter -t %d -n tc filter add dev %s pref %d parent ffff: protocol all "+
			"u32 match u8 0 0 "+
			"action mirred egress mirror dev %s",
			containerInfo.Pid, vethPair.OriginalName, TC_PREF_BASE+2, vethPair.ExternalName),
	}
	
	// 设置主机侧TC规则（mirror到NV bridge）
	hostRules := []string{
		fmt.Sprintf("tc filter add dev %s pref %d parent ffff: protocol all "+
			"u32 match u8 0 0 "+
			"action mirred egress mirror dev %s",
			vethPair.InternalName, TC_PREF_BASE+1, NV_BRIDGE_NAME),
	}
	
	// 设置NV bridge规则（丢弃来自enforcer的数据包）
	bridgeRules := []string{
		fmt.Sprintf("tc filter add dev %s pref %d parent ffff: protocol all "+
			"u32 match u16 0x%02x%02x 0xffff at -14 match u32 0x%02x%02x%02x%02x 0xffffffff at -12 "+
			"action drop",
			NV_BRIDGE_NAME, pref,
			vethPair.NVMAC[0], vethPair.NVMAC[1], vethPair.NVMAC[2], vethPair.NVMAC[3], vethPair.NVMAC[4], vethPair.NVMAC[5]),
	}
	
	// 执行所有规则
	allRules := append(ingressRules, egressRules...)
	allRules = append(allRules, hostRules...)
	allRules = append(allRules, bridgeRules...)
	
	for _, rule := range allRules {
		if err := tc.executeCommand(rule); err != nil {
			log.WithFields(log.Fields{"rule": rule, "error": err}).Warn("Failed to add TC rule")
		} else {
			containerInfo.TCRules = append(containerInfo.TCRules, rule)
		}
	}
	
	return nil
}

// getAvailablePref 获取可用的TC优先级
// 分配唯一的TC规则优先级避免冲突
func (tc *TCTrafficCapture) getAvailablePref(portIndex uint) uint {
	pref := portIndex % TC_PREF_MAX
	
	if !tc.prefs[pref] {
		tc.prefs[pref] = true
		return pref
	}
	
	// 查找最小可用优先级
	for pref = 1; pref < TC_PREF_MAX; pref++ {
		if !tc.prefs[pref] {
			tc.prefs[pref] = true
			return pref
		}
	}
	
	return 0
}

// StopContainerCapture 停止捕获容器流量
// 清理容器的TC规则和veth pair配置
func (tc *TCTrafficCapture) StopContainerCapture(containerID string) error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	containerInfo, exists := tc.containers[containerID]
	if !exists {
		return fmt.Errorf("container %s not found", containerID)
	}
	
	log.WithField("container", containerInfo.Name).Info("Stopping TC-based container traffic capture")
	
	// 删除TC规则
	for _, rule := range containerInfo.TCRules {
		deleteRule := strings.Replace(rule, "add", "del", 1)
		if err := tc.executeCommand(deleteRule); err != nil {
			log.WithFields(log.Fields{"rule": deleteRule, "error": err}).Warn("Failed to delete TC rule")
		}
	}
	
	// 清理veth pairs
	for _, vethPair := range containerInfo.VethPairs {
		tc.cleanupVethPair(vethPair)
	}
	
	// 释放优先级
	for ifaceName, vethPair := range containerInfo.VethPairs {
		if portInfo, exists := tc.portMap[vethPair.InternalName]; exists {
			tc.prefs[portInfo.Pref] = false
			delete(tc.portMap, vethPair.InternalName)
		}
		if portInfo, exists := tc.portMap[vethPair.ExternalName]; exists {
			tc.prefs[portInfo.Pref] = false
			delete(tc.portMap, vethPair.ExternalName)
		}
		// 释放接口索引
		tc.prefs[vethPair.Index] = false
		_ = ifaceName // 避免未使用变量警告
	}
	
	delete(tc.containers, containerID)
	
	log.WithField("container", containerInfo.Name).Info("Container traffic capture stopped")
	return nil
}

// cleanupVethPair 清理veth pair
// 删除qdisc和veth pair接口
func (tc *TCTrafficCapture) cleanupVethPair(vethPair *VethPairInfo) {
	// 删除qdisc
	tc.delQDisc(vethPair.InternalName)
	tc.delQDisc(vethPair.ExternalName)
	
	// 删除veth pair（删除一端会自动删除另一端）
	cmd := fmt.Sprintf("ip link del %s", vethPair.InternalName)
	tc.executeCommand(cmd)
}

// IPConfig 接口IP配置信息
type IPConfig struct {
	IPAddr  string // IP地址/掩码，如 "172.17.0.2/16"
	Gateway string // 网关地址
}

// getInterfaceIPConfig 获取接口的IP配置
// 解析容器接口的IP地址和网关信息
func (tc *TCTrafficCapture) getInterfaceIPConfig(pid int, iface string) (*IPConfig, error) {
	config := &IPConfig{}
	
	// 获取IP地址
	cmd := fmt.Sprintf("nsenter -t %d -n ip addr show %s", pid, iface)
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return nil, err
	}
	
	// 解析IP地址: "inet 172.17.0.2/16 brd 172.17.255.255 scope global nv-ex-eth0"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") && !strings.Contains(line, "127.0.0.1") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				config.IPAddr = parts[1] // 172.17.0.2/16
				break
			}
		}
	}
	
	// 获取默认路由
	cmd = fmt.Sprintf("nsenter -t %d -n ip route show default", pid)
	output, err = exec.Command("sh", "-c", cmd).Output()
	if err == nil {
		// 解析默认路由: "default via 172.17.0.1 dev nv-ex-eth0"
		line := strings.TrimSpace(string(output))
		if strings.HasPrefix(line, "default via ") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				config.Gateway = parts[2] // 172.17.0.1
			}
		}
	}
	
	if config.IPAddr == "" {
		return nil, fmt.Errorf("no IP address found")
	}
	
	return config, nil
}
// cleanupContainerInterfaces 清理容器接口
// 删除容器和主机侧的nv-开头接口
func (tc *TCTrafficCapture) cleanupContainerInterfaces(pid int) {
	// 清理容器中的nv-接口
	cmd := fmt.Sprintf("nsenter -t %d -n ip link show", pid)
	output, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return
	}
	
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ": nv-") && !strings.HasPrefix(line, " ") {
			// 解析接口行: "3: nv-in-eth0@eth0: <BROADCAST,MULTICAST,M-DOWN>"
			parts := strings.Split(line, ": ")
			if len(parts) >= 2 {
				ifaceName := strings.Split(parts[1], "@")[0]
				// 删除nv-开头的接口
				deleteCmd := fmt.Sprintf("nsenter -t %d -n ip link del %s", pid, ifaceName)
				tc.executeCommand(deleteCmd) // 忽略错误
			}
		}
	}
	
	// 清理主机侧的nv-接口
	hostOutput, err := exec.Command("ip", "link", "show").Output()
	if err != nil {
		return
	}
	
	hostLines := strings.Split(string(hostOutput), "\n")
	for _, line := range hostLines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ": nv-") && !strings.HasPrefix(line, " ") {
			parts := strings.Split(line, ": ")
			if len(parts) >= 2 {
				ifaceName := strings.Split(parts[1], "@")[0]
				// 删除nv-开头的接口（除了nv-br）
				if ifaceName != NV_BRIDGE_NAME {
					deleteCmd := fmt.Sprintf("ip link del %s", ifaceName)
					tc.executeCommand(deleteCmd) // 忽略错误
				}
			}
		}
	}
}
// executeCommand 执行系统命令
// 执行TC和网络配置命令并记录日志
func (tc *TCTrafficCapture) executeCommand(command string) error {
	log.WithField("cmd", command).Debug("Executing TC command")
	
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		log.WithFields(log.Fields{
			"cmd":    command,
			"output": string(output),
			"error":  err,
		}).Debug("TC command execution failed")
		return err
	}
	
	return nil
}

// GetCapturedContainers 获取正在捕获的容器列表
// 返回当前配置了TC规则的容器名称列表
func (tc *TCTrafficCapture) GetCapturedContainers() []string {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()
	
	var containers []string
	for id, info := range tc.containers {
		containers = append(containers, fmt.Sprintf("%s (%s)", info.Name, id[:12]))
	}
	
	return containers
}

// Cleanup 清理所有TC规则和bridge
// 停止所有容器捕获并清理NV bridge
func (tc *TCTrafficCapture) Cleanup() error {
	tc.mutex.Lock()
	defer tc.mutex.Unlock()
	
	log.Info("Cleaning up TC traffic capture")
	
	// 停止所有容器的流量捕获
	for containerID := range tc.containers {
		tc.StopContainerCapture(containerID)
	}
	
	// 清理NV bridge
	if link, err := netlink.LinkByName(NV_BRIDGE_NAME); err == nil {
		tc.cleanupBridge(link)
	}
	
	tc.bridgeReady = false
	
	log.Info("TC traffic capture cleanup completed")
	return nil
}