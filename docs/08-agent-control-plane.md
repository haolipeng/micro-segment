# Agent 控制平面架构

## 一、概述

Agent 控制平面是微隔离系统的核心协调组件，负责：
- 初始化和管理各个子系统
- 协调容器事件和网络拦截
- 处理策略更新和配置变更
- 维护系统状态

## 二、整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        Agent 控制平面                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │ 运行时监听器 │    │  策略引擎   │    │  数据平面   │         │
│  │  (Runtime)  │    │  (Policy)   │    │    (DP)     │         │
│  └──────┬──────┘    └──────┬──────┘    └──────┬──────┘         │
│         │                  │                  │                 │
│         ▼                  ▼                  ▼                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                    任务调度器                            │   │
│  │              (containerTaskWorker)                       │   │
│  └─────────────────────────┬───────────────────────────────┘   │
│                            │                                    │
│         ┌──────────────────┼──────────────────┐                │
│         ▼                  ▼                  ▼                │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐         │
│  │ 容器管理器  │    │ 网络拦截器  │    │ 集群同步器  │         │
│  │(Containers)│    │   (Pipe)    │    │  (Cluster)  │         │
│  └─────────────┘    └─────────────┘    └─────────────┘         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 三、Agent 启动流程

**源码位置**: `agent/agent.go:255-843`

```go
func main() {
    // ==================== 阶段 1: 基础初始化 ====================

    // 1.1 解析命令行参数
    flag.Parse()

    // 1.2 初始化日志
    log.SetOutput(os.Stdout)
    log.SetLevel(getLogLevel(*logLevel))

    // 1.3 初始化全局对象 (运行时、平台检测)
    platform, flavor, cloudPlatform, network, containers, err :=
        global.SetGlobalObjects(*rtSock, resource.Register)
    if err != nil {
        log.Fatal("Failed to initialize global objects")
    }

    // ==================== 阶段 2: 主机信息收集 ====================

    // 2.1 获取主机信息
    Host = share.CLUSHost{
        ID:       getHostID(),
        Name:     getHostName(),
        Platform: platform,
        Flavor:   flavor,
    }

    // 2.2 收集主机网络信息
    Host.Ifaces = getHostInterfaces()
    gInfo.hostIPs = getHostIPs()

    // ==================== 阶段 3: 子系统初始化 ====================

    // 3.1 初始化策略引擎
    policyInit()
    pe.Init(Host.ID, gInfo.hostIPs, Host.TunnelIP,
        ObtainGroupProcessPolicy, policyApplyDir)

    // 3.2 初始化 Pipe (网络拦截)
    pipe.Open(pipeDriver, Host.TunnelIP, dpTaskCallback)

    // 3.3 启动集群连接
    clusterStart(&clusterCfg)

    // 3.4 启动数据平面
    dp.Open(dpTaskCallback, dpStatusChan, errRestartChan)

    // 3.5 启动 Probe (进程/文件监控)
    prober, err = probe.New(&probeConfig)

    // ==================== 阶段 4: 事件循环启动 ====================

    // 4.1 启动容器事件监听
    eventMonitorLoop(probeTaskChan, fsmonTaskChan, dpStatusChan)

    // 4.2 启动集群事件循环
    clusterLoop(existing)

    // 4.3 启动统计循环
    go statsLoop(bPassiveContainerDetect)

    // 4.4 启动定时器循环
    go timerLoop()

    // ==================== 阶段 5: 信号处理 ====================

    // 等待退出信号
    signalChan := make(chan os.Signal, 1)
    signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
    <-signalChan

    // 清理资源
    cleanup()
}
```

## 四、任务调度器

任务调度器是控制平面的核心，负责处理各种事件和任务。

**源码位置**: `agent/engine.go:2283-2403`

### 4.1 任务类型定义

```go
const (
    TASK_ADD_CONTAINER       = iota  // 添加容器
    TASK_STOP_CONTAINER              // 停止容器
    TASK_DEL_CONTAINER               // 删除容器
    TASK_CONFIG_CONTAINER            // 配置容器
    TASK_INTERCEPT_CONTAINER         // 拦截容器
    TASK_REINTERCEPT_CONTAINER       // 重新拦截
    TASK_CONFIG_AGENT                // 配置 Agent
    TASK_CONFIG_SYSTEM               // 系统配置
)

type ContainerTask struct {
    task     int                      // 任务类型
    id       string                   // 容器 ID
    pid      int                      // 容器 PID
    info     *container.ContainerMetaExtra
    macConf  *share.CLUSWorkloadConfig
    agentConf *share.CLUSAgentConfig
    taskData  *systemConfigTask
}
```

### 4.2 任务工作线程

```go
func containerTaskWorker(probeChan chan *probe.ProbeMessage,
    fsmonChan chan *fsmon.MonitorMessage,
    dpStatusChan chan bool) {

    // 策略拉取定时器 (每 10 秒)
    pullPolicyTicker := time.NewTicker(time.Second * 10)
    defer pullPolicyTicker.Stop()

    for {
        select {
        // ========== 定时任务 ==========
        case <-pullPolicyTicker.C:
            // 检查是否需要拉取新策略
            if policyNeedUpdate() {
                pullNetworkPolicy()
            }

        // ========== 容器任务 ==========
        case task := <-ContainerTaskChan:
            switch task.task {
            case TASK_ADD_CONTAINER:
                taskAddContainer(task.id, task.info)

            case TASK_STOP_CONTAINER:
                taskStopContainer(task.id, task.pid)

            case TASK_DEL_CONTAINER:
                taskDelContainer(task.id)

            case TASK_CONFIG_CONTAINER:
                taskConfigContainer(task.id, task.macConf)

            case TASK_INTERCEPT_CONTAINER:
                taskInterceptContainer(task.id, task.info)

            case TASK_REINTERCEPT_CONTAINER:
                taskReinterceptContainer(task.id)

            case TASK_CONFIG_AGENT:
                taskConfigAgent(task.agentConf)

            case TASK_CONFIG_SYSTEM:
                task.taskData.handler()
            }

        // ========== Probe 消息 ==========
        case msg := <-probeChan:
            handleProbeMessage(msg)

        // ========== 文件监控消息 ==========
        case msg := <-fsmonChan:
            handleFsmonMessage(msg)

        // ========== 数据平面状态 ==========
        case connected := <-dpStatusChan:
            if connected {
                // DP 连接恢复，重新推送所有配置
                pushAllConfigToDP()
            }
        }
    }
}
```

## 五、事件通道架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        事件通道架构                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ContainerTaskChan ──────────────┐                              │
│  (容器任务)                       │                              │
│                                  │                              │
│  ClusterEventChan ──────────────┼──▶ containerTaskWorker       │
│  (集群事件)                       │                              │
│                                  │                              │
│  ProbeTaskChan ─────────────────┤                              │
│  (进程监控)                       │                              │
│                                  │                              │
│  FsmonTaskChan ─────────────────┤                              │
│  (文件监控)                       │                              │
│                                  │                              │
│  DPStatusChan ──────────────────┘                              │
│  (数据平面状态)                                                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 5.1 通道定义

```go
var (
    // 容器任务通道
    ContainerTaskChan = make(chan *ContainerTask, 256)

    // 集群事件通道
    ClusterEventChan = make(chan *ClusterEvent, 256)

    // DP 状态通道
    dpStatusChan = make(chan bool, 8)

    // 错误重启通道
    errRestartChan = make(chan interface{}, 8)
)
```

## 六、集群事件循环

**源码位置**: `agent/cluster.go:782-840`

```go
func clusterLoop(existing utils.Set) {
    // 清理不存在的容器记录
    for id := range existing.Iter() {
        if _, ok := gInfoReadActiveContainer(id.(string)); !ok {
            cluster.Delete(share.CLUSWorkloadKey(Host.ID, id.(string)))
        }
    }

    // 启动集群事件处理线程
    go func() {
        for {
            select {
            case ev := <-ClusterEventChan:
                clusterEventHandler(ev)
            }
        }
    }()

    // 上传现有容器到集群
    go func() {
        for id := range existing.Iter() {
            task := &ContainerTask{
                task: TASK_ADD_CONTAINER,
                id:   id.(string),
            }
            ContainerTaskChan <- task
        }

        // 注册配置监听器
        registerConfigWatchers()
    }()
}
```

### 6.1 集群事件处理

```go
type ClusterEvent struct {
    event int
    id    string
    info  *container.ContainerMetaExtra
    agent *Agent
}

const (
    EV_ADD_CONTAINER    = iota
    EV_STOP_CONTAINER
    EV_DEL_CONTAINER
    EV_UPDATE_CONTAINER
    EV_ADD_AGENT
    EV_DEL_AGENT
)

func clusterEventHandler(ev *ClusterEvent) {
    switch ev.event {
    case EV_ADD_CONTAINER:
        // 同步容器信息到集群存储
        wl := containerToWorkload(ev.id, ev.info)
        cluster.Put(share.CLUSWorkloadKey(Host.ID, ev.id), wl)

    case EV_STOP_CONTAINER:
        // 更新容器状态
        wl := cluster.Get(share.CLUSWorkloadKey(Host.ID, ev.id))
        wl.Running = false
        cluster.Put(share.CLUSWorkloadKey(Host.ID, ev.id), wl)

    case EV_DEL_CONTAINER:
        // 删除集群记录
        cluster.Delete(share.CLUSWorkloadKey(Host.ID, ev.id))
    }
}
```

## 七、配置监听器

**源码位置**: `agent/system.go`

```go
func registerConfigWatchers() {
    // 监听网络策略变更
    cluster.Watch(share.CLUSNetworkStore, networkConfigHandler)

    // 监听节点规则变更
    cluster.Watch(share.CLUSNodeRulesKey(Host.ID), nodeRulesHandler)

    // 监听 Agent 配置变更
    cluster.Watch(share.CLUSAgentKey(Host.ID), agentConfigHandler)

    // 监听进程配置变更
    cluster.Watch(share.CLUSNodeProfileStore(Host.ID), profileHandler)
}

// 网络配置变更处理
func networkConfigHandler(nType cluster.ClusterNotifyType, key string, value []byte) {
    task := &ContainerTask{
        task: TASK_CONFIG_SYSTEM,
        taskData: &systemConfigTask{
            handler: func() {
                systemUpdatePolicy(value)
            },
        },
    }
    ContainerTaskChan <- task
}
```

## 八、容器数据管理

### 8.1 容器数据结构

```go
type containerData struct {
    id         string
    name       string
    pid        int
    info       *container.ContainerMetaExtra

    // 网络拦截信息
    intcpPairs []*pipe.InterceptPair
    appMap     map[share.CLUSProtoPort]*share.CLUSApp
    portMap    map[share.CLUSProtoPort]*share.CLUSMappedPort

    // 状态标志
    inline     bool  // 是否内联模式
    quar       bool  // 是否隔离
    capIntcp   bool  // 是否可拦截
    hostMode   bool  // 是否主机网络模式

    // 策略信息
    policyMode string
}

// 全局容器映射
var (
    activeContainers = make(map[string]*containerData)
    containerMutex   sync.RWMutex
)
```

### 8.2 容器访问函数

```go
// 读取容器（带读锁）
func gInfoReadActiveContainer(id string) (*containerData, bool) {
    containerMutex.RLock()
    defer containerMutex.RUnlock()
    c, ok := activeContainers[id]
    return c, ok
}

// 写入容器（带写锁）
func gInfoWriteActiveContainer(id string, c *containerData) {
    containerMutex.Lock()
    defer containerMutex.Unlock()
    activeContainers[id] = c
}

// 删除容器（带写锁）
func gInfoDeleteActiveContainer(id string) {
    containerMutex.Lock()
    defer containerMutex.Unlock()
    delete(activeContainers, id)
}
```

## 九、定时器循环

```go
func timerLoop() {
    // 统计上报定时器
    statsTicker := time.NewTicker(time.Second * 30)
    defer statsTicker.Stop()

    // 健康检查定时器
    healthTicker := time.NewTicker(time.Minute)
    defer healthTicker.Stop()

    // 清理定时器
    cleanupTicker := time.NewTicker(time.Hour)
    defer cleanupTicker.Stop()

    for {
        select {
        case <-statsTicker.C:
            reportStats()

        case <-healthTicker.C:
            healthCheck()

        case <-cleanupTicker.C:
            cleanupStaleData()
        }
    }
}
```

## 十、简化实现示例

对于 PoC 实现，可以使用更简单的架构：

```go
package agent

import (
    "sync"
    "time"
)

type Agent struct {
    runtime   *RuntimeWatcher
    pipe      *NetworkPipe
    policy    *PolicyEngine
    containers map[string]*Container
    taskChan  chan Task
    stopChan  chan struct{}
    mu        sync.RWMutex
}

type Task struct {
    Type        string
    ContainerID string
    Data        interface{}
}

func New() (*Agent, error) {
    a := &Agent{
        containers: make(map[string]*Container),
        taskChan:   make(chan Task, 100),
        stopChan:   make(chan struct{}),
    }

    // 初始化子系统
    var err error
    a.runtime, err = NewRuntimeWatcher()
    if err != nil {
        return nil, err
    }

    a.pipe, err = NewNetworkPipe()
    if err != nil {
        return nil, err
    }

    a.policy = NewPolicyEngine()

    return a, nil
}

func (a *Agent) Start() error {
    // 启动运行时监听
    if err := a.runtime.Start(); err != nil {
        return err
    }

    // 启动任务处理循环
    go a.taskLoop()

    // 启动事件转发
    go a.eventForwarder()

    return nil
}

func (a *Agent) taskLoop() {
    ticker := time.NewTicker(time.Second * 10)
    defer ticker.Stop()

    for {
        select {
        case task := <-a.taskChan:
            a.handleTask(task)

        case <-ticker.C:
            a.periodicCheck()

        case <-a.stopChan:
            return
        }
    }
}

func (a *Agent) handleTask(task Task) {
    switch task.Type {
    case "add":
        a.addContainer(task.ContainerID)
    case "stop":
        a.stopContainer(task.ContainerID)
    case "delete":
        a.deleteContainer(task.ContainerID)
    }
}

func (a *Agent) eventForwarder() {
    for ev := range a.runtime.Events() {
        task := Task{
            Type:        ev.Type,
            ContainerID: ev.ContainerID,
        }
        a.taskChan <- task
    }
}

func (a *Agent) addContainer(id string) {
    info, err := a.runtime.GetContainer(id)
    if err != nil {
        return
    }

    // 创建容器数据
    c := &Container{
        ID:  id,
        PID: info.Pid,
    }

    // 拦截网络
    pairs, err := a.pipe.InterceptContainer(info.Pid)
    if err != nil {
        return
    }
    c.IntcpPairs = pairs

    // 应用策略
    a.policy.ApplyPolicy(c)

    // 保存容器
    a.mu.Lock()
    a.containers[id] = c
    a.mu.Unlock()
}

func (a *Agent) stopContainer(id string) {
    a.mu.Lock()
    c, ok := a.containers[id]
    a.mu.Unlock()

    if !ok {
        return
    }

    // 清理网络拦截
    a.pipe.CleanupContainer(c.PID, c.IntcpPairs)
}

func (a *Agent) deleteContainer(id string) {
    a.mu.Lock()
    delete(a.containers, id)
    a.mu.Unlock()
}

func (a *Agent) Stop() {
    close(a.stopChan)
    a.runtime.Stop()
}
```

## 十一、关键要点

1. **事件驱动**: 使用通道实现异步事件处理
2. **单一调度器**: 所有任务通过 containerTaskWorker 统一处理
3. **锁保护**: 容器数据使用读写锁保护并发访问
4. **配置监听**: 支持动态配置更新
5. **定时任务**: 周期性执行统计、健康检查等
6. **优雅退出**: 支持信号处理和资源清理
