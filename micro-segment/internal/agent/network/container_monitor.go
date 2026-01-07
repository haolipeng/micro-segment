// Package network 实现Docker容器监控
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

// ContainerMonitor Docker容器监控器
type ContainerMonitor struct {
	client    *client.Client
	tcCapture *TCTrafficCapture
	ctx       context.Context
	cancel    context.CancelFunc
}

// ContainerEvent 容器事件
type ContainerEvent struct {
	Type        string            // start, stop, die
	ContainerID string            // 容器ID
	Name        string            // 容器名称
	Image       string            // 镜像名称
	Labels      map[string]string // 标签
	Pid         int               // 容器PID
}

// NewContainerMonitor 创建容器监控器
func NewContainerMonitor(tcCapture *TCTrafficCapture) (*ContainerMonitor, error) {
	// 连接Docker daemon
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %v", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	monitor := &ContainerMonitor{
		client:    cli,
		tcCapture: tcCapture,
		ctx:       ctx,
		cancel:    cancel,
	}
	
	return monitor, nil
}

// Start 启动容器监控
func (cm *ContainerMonitor) Start() error {
	log.Info("Starting Docker container monitor")
	
	// 监控现有容器
	if err := cm.monitorExistingContainers(); err != nil {
		log.WithError(err).Warn("Failed to monitor existing containers")
	}
	
	// 监控容器事件
	go cm.monitorContainerEvents()
	
	return nil
}

// Stop 停止容器监控
func (cm *ContainerMonitor) Stop() error {
	log.Info("Stopping Docker container monitor")
	
	cm.cancel()
	
	if cm.client != nil {
		return cm.client.Close()
	}
	
	return nil
}

// monitorExistingContainers 监控现有运行的容器
func (cm *ContainerMonitor) monitorExistingContainers() error {
	log.Info("Scanning existing containers")
	
	containers, err := cm.client.ContainerList(cm.ctx, types.ContainerListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list containers: %v", err)
	}
	
	for _, container := range containers {
		if container.State == "running" {
			// 获取容器详细信息
			inspect, err := cm.client.ContainerInspect(cm.ctx, container.ID)
			if err != nil {
				log.WithError(err).WithField("container", container.ID).Warn("Failed to inspect container")
				continue
			}
			
			// 跳过系统容器和特殊容器
			if cm.shouldSkipContainer(&inspect) {
				continue
			}
			
			event := &ContainerEvent{
				Type:        "start",
				ContainerID: container.ID,
				Name:        strings.TrimPrefix(container.Names[0], "/"),
				Image:       container.Image,
				Labels:      container.Labels,
				Pid:         inspect.State.Pid,
			}
			
			cm.handleContainerEvent(event)
		}
	}
	
	log.WithField("count", len(containers)).Info("Existing containers scanned")
	return nil
}

// monitorContainerEvents 监控容器事件
func (cm *ContainerMonitor) monitorContainerEvents() {
	log.Info("Starting container event monitoring")
	
	// 设置事件过滤器
	eventFilters := filters.NewArgs()
	eventFilters.Add("type", "container")
	eventFilters.Add("event", "start")
	eventFilters.Add("event", "stop")
	eventFilters.Add("event", "die")
	
	eventOptions := types.EventsOptions{
		Filters: eventFilters,
	}
	
	eventChan, errChan := cm.client.Events(cm.ctx, eventOptions)
	
	for {
		select {
		case event := <-eventChan:
			cm.processDockerEvent(event)
			
		case err := <-errChan:
			if err != nil {
				log.WithError(err).Error("Docker event stream error")
				// 重新连接
				time.Sleep(5 * time.Second)
				eventChan, errChan = cm.client.Events(cm.ctx, eventOptions)
			}
			
		case <-cm.ctx.Done():
			log.Info("Container event monitoring stopped")
			return
		}
	}
}

// processDockerEvent 处理Docker事件
func (cm *ContainerMonitor) processDockerEvent(event events.Message) {
	log.WithFields(log.Fields{
		"action":    event.Action,
		"container": event.Actor.ID[:12],
		"image":     event.Actor.Attributes["image"],
	}).Debug("Docker event received")
	
	// 获取容器详细信息
	inspect, err := cm.client.ContainerInspect(cm.ctx, event.Actor.ID)
	if err != nil {
		log.WithError(err).WithField("container", event.Actor.ID).Warn("Failed to inspect container")
		return
	}
	
	// 跳过系统容器
	if cm.shouldSkipContainer(&inspect) {
		return
	}
	
	containerEvent := &ContainerEvent{
		Type:        string(event.Action),
		ContainerID: event.Actor.ID,
		Name:        strings.TrimPrefix(inspect.Name, "/"),
		Image:       inspect.Config.Image,
		Labels:      inspect.Config.Labels,
		Pid:         inspect.State.Pid,
	}
	
	cm.handleContainerEvent(containerEvent)
}

// handleContainerEvent 处理容器事件
func (cm *ContainerMonitor) handleContainerEvent(event *ContainerEvent) {
	log.WithFields(log.Fields{
		"action":    event.Type,
		"container": event.Name,
		"id":        event.ContainerID[:12],
		"pid":       event.Pid,
	}).Info("Processing container event")
	
	switch event.Type {
	case "start":
		// 容器启动，开始流量捕获
		if event.Pid > 0 {
			if err := cm.tcCapture.StartContainerCapture(event.ContainerID, event.Name, event.Pid); err != nil {
				log.WithError(err).WithField("container", event.Name).Error("Failed to start TC traffic capture")
			}
		} else {
			log.WithField("container", event.Name).Warn("Container has no PID, skipping TC traffic capture")
		}
		
	case "stop", "die":
		// 容器停止，停止流量捕获
		if err := cm.tcCapture.StopContainerCapture(event.ContainerID); err != nil {
			log.WithError(err).WithField("container", event.Name).Warn("Failed to stop TC traffic capture")
		}
	}
}

// shouldSkipContainer 判断是否应该跳过容器
func (cm *ContainerMonitor) shouldSkipContainer(inspect *types.ContainerJSON) bool {
	// 跳过暂停容器
	if strings.Contains(inspect.Config.Image, "pause") {
		return true
	}
	
	// 跳过系统容器
	systemImages := []string{
		"k8s.gcr.io/pause",
		"registry.k8s.io/pause",
		"gcr.io/google_containers/pause",
		"quay.io/coreos/etcd",
		"calico/node",
		"calico/cni",
		"flannel/flannel",
		"weaveworks/weave",
	}
	
	for _, sysImage := range systemImages {
		if strings.Contains(inspect.Config.Image, sysImage) {
			return true
		}
	}
	
	// 跳过特权容器（可选）
	if inspect.HostConfig.Privileged {
		log.WithField("container", inspect.Name).Debug("Skipping privileged container")
		return true
	}
	
	// 跳过主机网络模式容器
	if inspect.HostConfig.NetworkMode == "host" {
		log.WithField("container", inspect.Name).Debug("Skipping host network container")
		return true
	}
	
	return false
}

// GetContainerInfo 获取容器信息
func (cm *ContainerMonitor) GetContainerInfo(containerID string) (*ContainerEvent, error) {
	inspect, err := cm.client.ContainerInspect(cm.ctx, containerID)
	if err != nil {
		return nil, err
	}
	
	return &ContainerEvent{
		Type:        "info",
		ContainerID: containerID,
		Name:        strings.TrimPrefix(inspect.Name, "/"),
		Image:       inspect.Config.Image,
		Labels:      inspect.Config.Labels,
		Pid:         inspect.State.Pid,
	}, nil
}

// ListRunningContainers 列出正在运行的容器
func (cm *ContainerMonitor) ListRunningContainers() ([]*ContainerEvent, error) {
	containers, err := cm.client.ContainerList(cm.ctx, types.ContainerListOptions{})
	if err != nil {
		return nil, err
	}
	
	var events []*ContainerEvent
	for _, container := range containers {
		if container.State == "running" {
			inspect, err := cm.client.ContainerInspect(cm.ctx, container.ID)
			if err != nil {
				continue
			}
			
			if cm.shouldSkipContainer(&inspect) {
				continue
			}
			
			event := &ContainerEvent{
				Type:        "running",
				ContainerID: container.ID,
				Name:        strings.TrimPrefix(container.Names[0], "/"),
				Image:       container.Image,
				Labels:      container.Labels,
				Pid:         inspect.State.Pid,
			}
			
			events = append(events, event)
		}
	}
	
	return events, nil
}

// GetContainerStats 获取容器统计信息
func (cm *ContainerMonitor) GetContainerStats(containerID string) (map[string]interface{}, error) {
	stats, err := cm.client.ContainerStats(cm.ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer stats.Body.Close()
	
	var statsData map[string]interface{}
	if err := json.NewDecoder(stats.Body).Decode(&statsData); err != nil {
		return nil, err
	}
	
	return statsData, nil
}