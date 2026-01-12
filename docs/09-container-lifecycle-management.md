# 容器生命周期管理

## 一、概述

容器生命周期管理负责：
- 容器启动时自动拦截网络流量
- 容器停止时清理 TC 规则和数据平面配置
- 容器删除时清理所有相关资源
- 处理容器重启和网络变更

## 二、生命周期状态机

```
┌─────────────────────────────────────────────────────────────────┐
│                      容器生命周期状态机                           │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│                        ┌─────────┐                              │
│                        │ Created │                              │
│                        └────┬────┘                              │
│                             │ docker start                      │
│                             ▼                                   │
│   ┌──────────────────────────────────────────────────────┐     │
│   │                    Running                            │     │
│   │  ┌──────────────────────────────────────────────┐    │     │
│   │  │ 1. taskAddContainer()                        │    │     │
│   │  │    - 获取容器元数据                           │    │     │
│   │  │    - 创建容器数据结构                         │    │     │
│   │  │    - 发送集群事件                            │    │     │
│   │  ├──────────────────────────────────────────────┤    │     │
│   │  │ 2. taskInterceptContainer()                  │    │     │
│   │  │    - 执行端口拦截                            │    │     │
│   │  │    - 配置 TC 规则                            │    │     │
│   │  │    - 配置数据平面                            │    │     │
│   │  │    - 应用网络策略                            │    │     │
│   │  └──────────────────────────────────────────────┘    │     │
│   └──────────────────────────────────────────────────────┘     │
│                             │ docker stop                       │
│                             ▼                                   │
│   ┌──────────────────────────────────────────────────────┐     │
│   │                    Stopped                            │     │
│   │  ┌──────────────────────────────────────────────┐    │     │
│   │  │ taskStopContainer()                          │    │     │
│   │  │    - 清理 TC 规则                            │    │     │
│   │  │    - 删除数据平面 MAC 配置                    │    │     │
│   │  │    - 清理拦截端口对                          │    │     │
│   │  │    - 发送停止事件                            │    │     │
│   │  └──────────────────────────────────────────────┘    │     │
│   └──────────────────────────────────────────────────────┘     │
│                             │ docker rm                         │
│                             ▼                                   │
│   ┌──────────────────────────────────────────────────────┐     │
│   │                    Deleted                            │     │
│   │  ┌──────────────────────────────────────────────┐    │     │
│   │  │ taskDelContainer()                           │    │     │
│   │  │    - 删除容器记录                            │    │     │
│   │  │    - 清理网络策略                            │    │     │
│   │  │    - 清理 PID/MAC 映射                       │    │     │
│   │  └──────────────────────────────────────────────┘    │     │
│   └──────────────────────────────────────────────────────┘     │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 三、容器启动处理

### 3.1 taskAddContainer - 添加容器

**源码位置**: `agent/engine.go:1950-2090`

```go
func taskAddContainer(id string, info *container.ContainerMetaExtra) {
    // ========== 步骤 1: 验证容器不存在 ==========
    if _, ok := gInfoReadActiveContainer(id); ok {
        log.WithField("id", id).Debug("Container already exists")
        return
    }

    // ========== 步骤 2: 获取容器信息 ==========
    if info == nil {
        var err error
        // 重试获取容器信息
        for retry := 0; retry < 2; retry++ {
            info, err = global.RT.GetContainer(id)
            if err == nil {
                break
            }
            time.Sleep(time.Millisecond * 100)
        }
        if err != nil {
            log.WithError(err).Error("Failed to get container info")
            return
        }
    }

    // ========== 步骤 3: 检查是否为 NeuVector 容器 ==========
    if isNeuVectorContainer(info) {
        handleNeuVectorContainer(id, info)
        return
    }

    // ========== 步骤 4: 检查是否需要跳过 ==========
    if shouldSkipContainer(info) {
        log.WithField("id", id).Debug("Skipping container")
        return
    }

    // ========== 步骤 5: 创建容器数据结构 ==========
    c := &containerData{
        id:         id,
        name:       info.Name,
        pid:        info.Pid,
        info:       info,
        intcpPairs: make([]*pipe.InterceptPair, 0),
        appMap:     make(map[share.CLUSProtoPort]*share.CLUSApp),
        portMap:    make(map[share.CLUSProtoPort]*share.CLUSMappedPort),
    }

    // 检查网络模式
    c.hostMode = isHostNetworkMode(info)
    c.capIntcp = !c.hostMode && canIntercept(info)

    // ========== 步骤 6: 保存容器 ==========
    gInfoWriteActiveContainer(id, c)

    // ========== 步骤 7: 发送集群事件 ==========
    ev := ClusterEvent{
        event: EV_ADD_CONTAINER,
        id:    id,
        info:  info,
    }
    ClusterEventChan <- &ev

    // ========== 步骤 8: 触发拦截任务 ==========
    if c.capIntcp {
        task := &ContainerTask{
            task: TASK_INTERCEPT_CONTAINER,
            id:   id,
            info: info,
        }
        ContainerTaskChan <- task
    }
}
```

### 3.2 taskInterceptContainer - 拦截容器网络

**源码位置**: `agent/engine.go:1839-1949`

```go
func taskInterceptContainer(id string, info *container.ContainerMetaExtra) {
    // ========== 步骤 1: 获取容器数据 ==========
    c, ok := gInfoReadActiveContainer(id)
    if !ok {
        log.WithField("id", id).Error("Container not found")
        return
    }

    // ========== 步骤 2: 检查拦截能力 ==========
    if !c.capIntcp {
        log.WithField("id", id).Debug("Container cannot be intercepted")
        return
    }

    // 检查 PID 是否有效
    if c.pid <= 0 {
        log.WithField("id", id).Error("Invalid container PID")
        return
    }

    // ========== 步骤 3: 确定工作模式 ==========
    inline := shouldUseInlineMode(c)
    quar := shouldQuarantine(c)

    // ========== 步骤 4: 执行端口拦截 ==========
    pairs, err := pipe.InterceptContainerPorts(c.pid, c.intcpPairs)
    if err != nil {
        log.WithError(err).Error("Failed to intercept container ports")
        return
    }

    // ========== 步骤 5: 配置 TC 规则 ==========
    for _, pair := range pairs {
        if inline {
            // FWD 模式 (流量必须经过 enforcer)
            pipe.FwdPortPair(pair)
        } else {
            // TAP 模式 (镜像流量)
            pipe.TapPortPair(pair)
        }
    }

    // ========== 步骤 6: 配置数据平面 ==========
    for _, pair := range pairs {
        // 添加 MAC 规则
        dp.DPCtrlAddMAC(nvSvcPort, pair.MAC, pair.UCMAC, pair.BCMAC)

        // 添加端口对
        dp.DPCtrlAddPortPair(pair.inPort, pair.exPort, pair.MAC)
    }

    // ========== 步骤 7: 更新容器数据 ==========
    c.intcpPairs = pairs
    c.inline = inline
    c.quar = quar

    // ========== 步骤 8: 应用网络策略 ==========
    applyNetworkPolicy(c)

    log.WithFields(log.Fields{
        "id":     id,
        "pairs":  len(pairs),
        "inline": inline,
    }).Info("Container intercepted successfully")
}
```

### 3.3 端口拦截核心逻辑

**源码位置**: `agent/pipe/port.go:744-872`

```go
func InterceptContainerPorts(pid int, existPairs []*InterceptPair) ([]*InterceptPair, error) {
    // 锁定线程（命名空间操作需要）
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // 保存当前命名空间
    curNs, _ := netns.Get()
    defer netns.Set(curNs)
    defer curNs.Close()

    // 获取容器命名空间
    nsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
    containerNs, err := netns.GetFromPath(nsPath)
    if err != nil {
        return nil, fmt.Errorf("failed to get container netns: %v", err)
    }
    defer containerNs.Close()

    // 获取 Enforcer 命名空间
    dstNs, _ := netns.GetFromPath(workingNsPath)
    defer dstNs.Close()

    // 切换到容器命名空间，执行端口拉取
    netns.Set(containerNs)
    pairs, err := pullAllContainerPorts(pid, int(dstNs), existPairs)
    if err != nil {
        return nil, err
    }

    // 切换到 Enforcer 命名空间，配置规则
    netns.Set(dstNs)
    for _, pair := range pairs {
        // 附加端口并获取 UCMAC
        pair.UCMAC, pair.BCMAC = piper.AttachPortPair(pair)

        // 激活端口
        link, _ := netlink.LinkByName(pair.inPort)
        netlink.LinkSetUp(link)

        link, _ = netlink.LinkByName(pair.exPort)
        netlink.LinkSetUp(link)
    }

    return pairs, nil
}
```

## 四、容器停止处理

### 4.1 taskStopContainer - 停止容器

**源码位置**: `agent/engine.go:2092-2198`

```go
func taskStopContainer(id string, pid int) {
    // ========== 步骤 1: 验证容器存在 ==========
    c, ok := gInfoReadActiveContainer(id)
    if !ok {
        log.WithField("id", id).Debug("Container not found, skipping stop")
        return
    }

    // 对于 containerd，验证 PID 匹配
    if pid > 0 && c.pid != pid {
        log.WithFields(log.Fields{
            "id":       id,
            "expected": c.pid,
            "actual":   pid,
        }).Debug("PID mismatch, skipping stop")
        return
    }

    // ========== 步骤 2: 更新容器状态 ==========
    if c.info != nil {
        c.info.Running = false
        c.info.FinishedAt = time.Now()
    }

    // ========== 步骤 3: 发送集群停止事件 ==========
    ev := ClusterEvent{
        event: EV_STOP_CONTAINER,
        id:    id,
        info:  c.info,
    }
    ClusterEventChan <- &ev

    // ========== 步骤 4: 清理网络拦截 ==========
    if len(c.intcpPairs) > 0 {
        // 停止接口监控
        stopInterfaceMonitor(c)

        // 清理 TC 规则和端口
        pipe.CleanupContainer(c.pid, c.intcpPairs)

        // 删除数据平面 MAC 规则
        for _, pair := range c.intcpPairs {
            dp.DPCtrlDelMAC(nvSvcPort, pair.MAC)
        }

        // 删除 TAP 端口 (如果是非内联模式)
        if !c.inline {
            netns := fmt.Sprintf("/proc/%d/ns/net", c.pid)
            for _, pair := range c.intcpPairs {
                dp.DPCtrlDelTapPort(netns, pair.Port)
            }
        }

        // 清空拦截对列表
        c.intcpPairs = c.intcpPairs[:0]
    }

    // ========== 步骤 5: 清理策略相关 ==========
    // 清理 NFQ 规则
    if len(c.appMap) > 0 {
        cleanupNFQRules(c)
        c.appMap = make(map[share.CLUSProtoPort]*share.CLUSApp)
    }

    // 清理代理网格规则
    if c.hasProxyMesh {
        cleanupProxyMeshRules(c)
    }

    // ========== 步骤 6: 更新本地子网映射 ==========
    updateLocalSubnetMap(c, false)

    log.WithField("id", id).Info("Container stopped and cleaned up")
}
```

### 4.2 端口清理逻辑

**源码位置**: `agent/pipe/port.go`

```go
func CleanupContainer(pid int, pairs []*InterceptPair) error {
    if len(pairs) == 0 {
        return nil
    }

    // 锁定线程
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // 保存当前命名空间
    curNs, _ := netns.Get()
    defer netns.Set(curNs)
    defer curNs.Close()

    // 切换到 Enforcer 命名空间
    dstNs, _ := netns.GetFromPath(workingNsPath)
    defer dstNs.Close()
    netns.Set(dstNs)

    // 清理每个端口对
    for _, pair := range pairs {
        // 删除 TC 规则
        piper.DetachPortPair(pair)

        // 删除 QDisc
        removeQDisc(pair.inPort)
        removeQDisc(pair.exPort)

        // 删除端口
        if link, err := netlink.LinkByName(pair.inPort); err == nil {
            netlink.LinkDel(link)
        }
        if link, err := netlink.LinkByName(pair.exPort); err == nil {
            netlink.LinkDel(link)
        }
    }

    return nil
}

func (d *tcPipeDriver) DetachPortPair(pair *InterceptPair) {
    // 删除 inPort 的 TC filter
    d.detachPort(pair.inPort)

    // 删除 exPort 的 TC filter
    d.detachPort(pair.exPort)

    // 删除 vbr-neuv 上的防循环规则
    d.deleteVbrRule(pair)
}

func (d *tcPipeDriver) detachPort(port string) {
    info, ok := d.portMap[port]
    if !ok {
        return
    }

    // 删除所有 filter 规则
    shell(fmt.Sprintf("tc filter del dev %v parent ffff:", port))

    // 释放优先级
    d.prefs.Remove(info.pref)

    // 删除映射
    delete(d.portMap, port)
}
```

## 五、容器删除处理

### 5.1 taskDelContainer - 删除容器

**源码位置**: `agent/engine.go:2199-2280`

```go
func taskDelContainer(id string) {
    // ========== 步骤 1: 获取并删除容器数据 ==========
    c, ok := gInfoReadActiveContainer(id)
    if !ok {
        log.WithField("id", id).Debug("Container not found for deletion")
        return
    }

    // 从活跃容器映射中删除
    gInfoDeleteActiveContainer(id)

    // ========== 步骤 2: 删除网络策略 ==========
    pe.DeleteNetworkPolicy(id)

    // ========== 步骤 3: 删除 Probe 数据 ==========
    prober.DeleteContainer(id)

    // ========== 步骤 4: 清理 PID 映射 ==========
    if c.pid > 0 {
        gInfoDeletePidContainer(c.pid)
    }

    // ========== 步骤 5: 清理 MAC 映射 ==========
    for _, pair := range c.intcpPairs {
        gInfoDeleteMacContainer(pair.MAC.String())
    }

    // ========== 步骤 6: 发送集群删除事件 ==========
    ev := ClusterEvent{
        event: EV_DEL_CONTAINER,
        id:    id,
    }
    ClusterEventChan <- &ev

    log.WithField("id", id).Info("Container deleted")
}
```

## 六、模式切换处理

### 6.1 changeContainerWire - 切换工作模式

当策略模式变更时，需要重新配置容器的网络拦截。

**源码位置**: `agent/engine.go:520-751`

```go
func changeContainerWire(c *containerData, inline, quar bool, quarReason string) error {
    // 检查是否需要变更
    if c.inline == inline && c.quar == quar {
        return nil
    }

    log.WithFields(log.Fields{
        "id":         c.id,
        "oldInline":  c.inline,
        "newInline":  inline,
        "oldQuar":    c.quar,
        "newQuar":    quar,
    }).Info("Changing container wire mode")

    // ========== 移除现有配置 ==========
    if len(c.intcpPairs) > 0 {
        for _, pair := range c.intcpPairs {
            // 移除现有 TC 规则
            if c.inline {
                pipe.ResetFwdPortPair(pair)
            } else {
                pipe.ResetTapPortPair(pair)
            }
        }
    }

    // ========== 应用新配置 ==========
    for _, pair := range c.intcpPairs {
        if inline {
            // 切换到 FWD 模式
            pipe.FwdPortPair(pair)
        } else {
            // 切换到 TAP 模式
            pipe.TapPortPair(pair)
        }

        // 如果需要隔离
        if quar {
            pipe.QuarantinePortPair(pair)
        }
    }

    // 更新状态
    c.inline = inline
    c.quar = quar

    // 重新推送策略到数据平面
    applyNetworkPolicy(c)

    return nil
}
```

## 七、完整生命周期流程图

```
┌─────────────────────────────────────────────────────────────────┐
│                    容器启动完整流程                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Docker/Containerd                                              │
│       │                                                         │
│       │ container start event                                   │
│       ▼                                                         │
│  Runtime Watcher                                                │
│       │                                                         │
│       │ EventContainerStart                                     │
│       ▼                                                         │
│  runtimeEventCallback()                                         │
│       │                                                         │
│       │ TASK_ADD_CONTAINER                                      │
│       ▼                                                         │
│  ContainerTaskChan ──────────────────────────────────────────►  │
│       │                                                         │
│       ▼                                                         │
│  containerTaskWorker()                                          │
│       │                                                         │
│       ▼                                                         │
│  taskAddContainer()                                             │
│       │                                                         │
│       ├──► GetContainer(id) ──► 获取 PID, MAC, IP               │
│       │                                                         │
│       ├──► 创建 containerData                                   │
│       │                                                         │
│       ├──► 发送 EV_ADD_CONTAINER 到集群                         │
│       │                                                         │
│       └──► TASK_INTERCEPT_CONTAINER ──────────────────────────► │
│                    │                                            │
│                    ▼                                            │
│            taskInterceptContainer()                             │
│                    │                                            │
│                    ├──► InterceptContainerPorts()               │
│                    │         │                                  │
│                    │         ├──► 进入容器命名空间               │
│                    │         ├──► 拉取 eth0 到 enforcer         │
│                    │         ├──► 创建新 veth pair              │
│                    │         └──► 返回 InterceptPair            │
│                    │                                            │
│                    ├──► TapPortPair() 或 FwdPortPair()          │
│                    │         │                                  │
│                    │         ├──► 创建 ingress qdisc            │
│                    │         └──► 添加 TC filter 规则           │
│                    │                                            │
│                    ├──► DPCtrlAddMAC()                          │
│                    │                                            │
│                    └──► applyNetworkPolicy()                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    容器停止完整流程                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Docker/Containerd                                              │
│       │                                                         │
│       │ container stop/die event                                │
│       ▼                                                         │
│  Runtime Watcher                                                │
│       │                                                         │
│       │ EventContainerStop                                      │
│       ▼                                                         │
│  runtimeEventCallback()                                         │
│       │                                                         │
│       │ TASK_STOP_CONTAINER                                     │
│       ▼                                                         │
│  ContainerTaskChan ──────────────────────────────────────────►  │
│       │                                                         │
│       ▼                                                         │
│  containerTaskWorker()                                          │
│       │                                                         │
│       ▼                                                         │
│  taskStopContainer()                                            │
│       │                                                         │
│       ├──► 验证容器存在 & PID 匹配                              │
│       │                                                         │
│       ├──► 发送 EV_STOP_CONTAINER 到集群                        │
│       │                                                         │
│       ├──► CleanupContainer()                                   │
│       │         │                                               │
│       │         ├──► DetachPortPair() - 删除 TC filter          │
│       │         ├──► removeQDisc() - 删除 qdisc                 │
│       │         └──► LinkDel() - 删除端口                       │
│       │                                                         │
│       ├──► DPCtrlDelMAC() - 删除 DP MAC 规则                    │
│       │                                                         │
│       └──► 清理 appMap, proxyMesh 等                            │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 八、简化实现示例

```go
package lifecycle

import (
    "fmt"
    "sync"
)

type LifecycleManager struct {
    pipe       *NetworkPipe
    policy     *PolicyEngine
    containers map[string]*Container
    mu         sync.RWMutex
}

type Container struct {
    ID         string
    PID        int
    IntcpPairs []*InterceptPair
    Inline     bool
}

func NewLifecycleManager(pipe *NetworkPipe, policy *PolicyEngine) *LifecycleManager {
    return &LifecycleManager{
        pipe:       pipe,
        policy:     policy,
        containers: make(map[string]*Container),
    }
}

// 处理容器启动
func (m *LifecycleManager) OnContainerStart(id string, pid int) error {
    // 检查是否已存在
    m.mu.RLock()
    _, exists := m.containers[id]
    m.mu.RUnlock()
    if exists {
        return nil
    }

    // 创建容器数据
    c := &Container{
        ID:  id,
        PID: pid,
    }

    // 执行端口拦截
    pairs, err := m.pipe.InterceptContainerPorts(pid)
    if err != nil {
        return fmt.Errorf("failed to intercept container: %v", err)
    }
    c.IntcpPairs = pairs

    // 配置 TC 规则 (默认 TAP 模式)
    for _, pair := range pairs {
        m.pipe.TapPortPair(pair)
    }

    // 应用策略
    m.policy.ApplyPolicy(c)

    // 保存容器
    m.mu.Lock()
    m.containers[id] = c
    m.mu.Unlock()

    return nil
}

// 处理容器停止
func (m *LifecycleManager) OnContainerStop(id string) error {
    m.mu.Lock()
    c, ok := m.containers[id]
    if !ok {
        m.mu.Unlock()
        return nil
    }
    m.mu.Unlock()

    // 清理网络拦截
    if err := m.pipe.CleanupContainer(c.PID, c.IntcpPairs); err != nil {
        return fmt.Errorf("failed to cleanup container: %v", err)
    }

    // 清理策略
    m.policy.RemovePolicy(c)

    return nil
}

// 处理容器删除
func (m *LifecycleManager) OnContainerDelete(id string) error {
    m.mu.Lock()
    delete(m.containers, id)
    m.mu.Unlock()
    return nil
}

// 切换工作模式
func (m *LifecycleManager) SetInlineMode(id string, inline bool) error {
    m.mu.RLock()
    c, ok := m.containers[id]
    m.mu.RUnlock()

    if !ok {
        return fmt.Errorf("container not found: %s", id)
    }

    if c.Inline == inline {
        return nil
    }

    // 移除现有规则
    for _, pair := range c.IntcpPairs {
        if c.Inline {
            m.pipe.ResetFwdPortPair(pair)
        } else {
            m.pipe.ResetTapPortPair(pair)
        }
    }

    // 应用新规则
    for _, pair := range c.IntcpPairs {
        if inline {
            m.pipe.FwdPortPair(pair)
        } else {
            m.pipe.TapPortPair(pair)
        }
    }

    c.Inline = inline
    return nil
}
```

## 九、关键要点

1. **原子操作**: 端口拦截和规则配置需要在同一事务中完成
2. **命名空间切换**: 必须锁定 OS 线程防止调度器切换
3. **PID 验证**: Containerd 环境需要验证停止事件的 PID
4. **顺序清理**: 停止时先删除 TC 规则，再删除端口
5. **状态同步**: 容器状态变更需要同步到集群和数据平面
6. **错误恢复**: 拦截失败需要回滚已创建的资源
