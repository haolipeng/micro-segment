# 容器运行时集成

## 一、概述

容器运行时集成是微隔离系统的核心组件之一，负责：
- 监听容器启动/停止事件
- 获取容器元数据（PID、网络命名空间等）
- 触发网络拦截和清理流程

## 二、运行时抽象接口

NeuVector 定义了统一的运行时接口，支持 Docker 和 Containerd：

**源码位置**: `share/container/types.go:26-29`

```go
type Runtime interface {
    // 事件监听
    MonitorEvent(cb EventCallback, cpath bool) error
    StopMonitorEvent()

    // 容器操作
    GetContainer(id string) (*ContainerMetaExtra, error)
    ListContainers(runningOnly bool) ([]*ContainerMeta, error)
    GetImageHistory(name string) ([]*ImageHistory, error)

    // 网络信息
    GetNetworkEndpoint(netName, container, epName string) (*NetworkEndpoint, error)

    // 文件系统
    GetParent(info *ContainerMetaExtra, pidMap map[int]string) (bool, string)
    IsDaemonProcess(proc string, cmds []string) bool
}

// 事件回调函数类型
type EventCallback func(event Event, id string, pid int)

// 事件类型
type Event string

const (
    EventContainerStart   Event = "start"
    EventContainerStop    Event = "stop"
    EventContainerDelete  Event = "delete"
    EventContainerCopyIn  Event = "copy-in"
    EventContainerCopyOut Event = "copy-out"
    EventSocketError      Event = "socket-err"
)
```

## 三、容器元数据结构

**源码位置**: `share/container/types.go`

```go
// 基础容器元数据
type ContainerMeta struct {
    ID       string
    Name     string
    Image    string
    Labels   map[string]string
    Pid      int
    Envs     []string
    Running  bool
    Finished bool
}

// 扩展容器元数据
type ContainerMetaExtra struct {
    ContainerMeta

    // 网络信息
    IPAddress   string
    IPPrefixLen int
    MacAddress  string
    Gateway     string
    NetworkMode string
    Networks    map[string]*ContainerNetwork

    // 端口映射
    MappedPorts map[share.CLUSProtoPort]*share.CLUSMappedPort

    // 文件系统
    LogPath   string
    Privileged bool
    RunAsRoot  bool

    // 时间戳
    StartedAt time.Time
    FinishedAt time.Time
    ExitCode   int
}

// 容器网络信息
type ContainerNetwork struct {
    IPAddress   string
    IPPrefixLen int
    MacAddress  string
    Gateway     string
    NetworkID   string
}
```

## 四、Docker 运行时实现

### 4.1 事件监听

**源码位置**: `share/container/docker.go:518-619`

```go
func (d *dockerDriver) MonitorEvent(cb EventCallback, cpath bool) error {
    d.eventCallback = cb

    // 设置事件过滤器
    opts := types.EventsOptions{
        Filters: filters.NewArgs(
            filters.Arg("type", "container"),
        ),
    }

    for {
        // 打开事件流
        ctx := context.Background()
        eventChan, errChan := d.client.Events(ctx, opts)

        for {
            select {
            case e := <-eventChan:
                d.eventHandler(e)

            case err := <-errChan:
                if err == io.EOF || err == context.Canceled {
                    return nil
                }
                // 重连逻辑
                time.Sleep(time.Second * 2)
                break
            }
        }
    }
}

// 事件处理器
func (d *dockerDriver) eventHandler(e events.Message) {
    switch e.Action {
    case "start":
        // 获取容器 PID
        info, err := d.GetContainer(e.ID)
        if err == nil && info.Pid > 0 {
            d.eventCallback(EventContainerStart, e.ID, info.Pid)
        }

    case "die", "kill":
        // 过滤 SIGHUP 信号（不是真正的停止）
        if sig, ok := e.Actor.Attributes["signal"]; ok && sig == "1" {
            return
        }
        d.eventCallback(EventContainerStop, e.ID, 0)

    case "destroy":
        d.eventCallback(EventContainerDelete, e.ID, 0)
    }
}
```

### 4.2 获取容器信息

```go
func (d *dockerDriver) GetContainer(id string) (*ContainerMetaExtra, error) {
    ctx := context.Background()
    info, err := d.client.ContainerInspect(ctx, id)
    if err != nil {
        return nil, err
    }

    meta := &ContainerMetaExtra{
        ContainerMeta: ContainerMeta{
            ID:      info.ID,
            Name:    strings.TrimPrefix(info.Name, "/"),
            Image:   info.Config.Image,
            Labels:  info.Config.Labels,
            Pid:     info.State.Pid,
            Running: info.State.Running,
        },
        NetworkMode: string(info.HostConfig.NetworkMode),
        Privileged:  info.HostConfig.Privileged,
    }

    // 解析网络信息
    if info.NetworkSettings != nil {
        meta.Networks = make(map[string]*ContainerNetwork)
        for name, net := range info.NetworkSettings.Networks {
            meta.Networks[name] = &ContainerNetwork{
                IPAddress:   net.IPAddress,
                IPPrefixLen: net.IPPrefixLen,
                MacAddress:  net.MacAddress,
                Gateway:     net.Gateway,
                NetworkID:   net.NetworkID,
            }
        }
    }

    return meta, nil
}
```

## 五、Containerd 运行时实现

### 5.1 事件监听

**源码位置**: `share/container/containerd.go:529-595`

```go
func (d *containerdDriver) MonitorEvent(cb EventCallback, cpath bool) error {
    d.eventCallback = cb
    failCount := 0

    for {
        // 订阅事件
        ctx := context.Background()
        eventChan, errChan := d.client.EventService().Subscribe(ctx,
            `topic=="/tasks/start"`,
            `topic=="/tasks/exit"`,
            `topic=="/tasks/delete"`,
        )

        for {
            select {
            case ev := <-eventChan:
                d.handleEvent(ev)
                failCount = 0

            case err := <-errChan:
                if err == nil {
                    continue
                }
                failCount++
                if failCount >= 12 {
                    // 通知 agent 需要重启
                    cb(EventSocketError, "", 0)
                }
                time.Sleep(time.Second * 10)
                break
            }
        }
    }
}

// 事件处理
func (d *containerdDriver) handleEvent(ev *events.Envelope) {
    var id string
    var pid int

    switch ev.Topic {
    case "/tasks/start":
        var e TaskStart
        if err := proto.Unmarshal(ev.Event.Value, &e); err == nil {
            id = e.ContainerID
            pid = int(e.Pid)
            d.eventCallback(EventContainerStart, id, pid)
        }

    case "/tasks/exit":
        var e TaskExit
        if err := proto.Unmarshal(ev.Event.Value, &e); err == nil {
            id = e.ContainerID
            pid = int(e.Pid)
            d.eventCallback(EventContainerStop, id, pid)
        }

    case "/tasks/delete":
        var e TaskDelete
        if err := proto.Unmarshal(ev.Event.Value, &e); err == nil {
            id = e.ContainerID
            d.eventCallback(EventContainerDelete, id, 0)
        }
    }
}
```

### 5.2 获取容器信息

```go
func (d *containerdDriver) GetContainer(id string) (*ContainerMetaExtra, error) {
    ctx := context.Background()

    // 获取容器
    container, err := d.client.LoadContainer(ctx, id)
    if err != nil {
        return nil, err
    }

    // 获取任务（运行中的容器实例）
    task, err := container.Task(ctx, nil)
    if err != nil {
        return nil, err
    }

    // 获取容器配置
    spec, err := container.Spec(ctx)
    if err != nil {
        return nil, err
    }

    meta := &ContainerMetaExtra{
        ContainerMeta: ContainerMeta{
            ID:      container.ID(),
            Pid:     int(task.Pid()),
            Running: true,
        },
    }

    // 从 spec 解析标签和环境变量
    if spec.Annotations != nil {
        meta.Labels = spec.Annotations
    }

    return meta, nil
}
```

## 六、Agent 事件回调

**源码位置**: `agent/engine.go:346-387`

```go
func runtimeEventCallback(ev container.Event, id string, pid int) {
    switch ev {
    case container.EventContainerStart:
        // 发送添加容器任务
        task := ContainerTask{
            task: TASK_ADD_CONTAINER,
            id:   id,
        }
        ContainerTaskChan <- &task

    case container.EventContainerStop:
        // 发送停止容器任务（需要验证 PID）
        task := ContainerTask{
            task: TASK_STOP_CONTAINER,
            id:   id,
            pid:  pid,
        }
        ContainerTaskChan <- &task

    case container.EventContainerDelete:
        // 发送删除容器任务
        task := ContainerTask{
            task: TASK_DEL_CONTAINER,
            id:   id,
        }
        ContainerTaskChan <- &task

    case container.EventSocketError:
        // 运行时连接错误，需要重启监听
        log.Error("Runtime socket error, restarting monitor")
        go restartRuntimeMonitor()
    }
}
```

## 七、事件监听启动

**源码位置**: `agent/engine.go:2456-2467`

```go
func eventMonitorLoop(probeChan, fsmonChan, dpStatusChan chan) {
    // 启动容器任务工作线程
    go containerTaskWorker(probeChan, fsmonChan, dpStatusChan)

    // 启动运行时事件监听
    go func() {
        for {
            err := global.RT.MonitorEvent(runtimeEventCallback, false)
            if err != nil {
                log.WithError(err).Error("Runtime monitor failed")
            }
            time.Sleep(time.Second * 5)
        }
    }()
}
```

## 八、运行时初始化

**源码位置**: `share/global/global.go`

```go
func SetGlobalObjects(rtSocket string, ...) (..., error) {
    // 检测运行时类型
    if _, err := os.Stat("/run/containerd/containerd.sock"); err == nil {
        RT = container.NewContainerdDriver(...)
    } else if _, err := os.Stat("/var/run/docker.sock"); err == nil {
        RT = container.NewDockerDriver(...)
    } else {
        return nil, fmt.Errorf("no container runtime found")
    }

    // 验证连接
    _, err := RT.ListContainers(false)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to runtime: %v", err)
    }

    return ...
}
```

## 九、简化实现示例

对于 PoC 实现，可以使用更简单的方式：

```go
package runtime

import (
    "context"
    "github.com/docker/docker/api/types"
    "github.com/docker/docker/api/types/events"
    "github.com/docker/docker/client"
)

type ContainerEvent struct {
    Type        string // "start", "stop", "delete"
    ContainerID string
    Pid         int
}

type Watcher struct {
    client   *client.Client
    eventCh  chan ContainerEvent
    stopCh   chan struct{}
}

func NewWatcher() (*Watcher, error) {
    cli, err := client.NewClientWithOpts(client.FromEnv)
    if err != nil {
        return nil, err
    }
    return &Watcher{
        client:  cli,
        eventCh: make(chan ContainerEvent, 100),
        stopCh:  make(chan struct{}),
    }, nil
}

func (w *Watcher) Start() error {
    ctx := context.Background()
    eventChan, errChan := w.client.Events(ctx, types.EventsOptions{})

    go func() {
        for {
            select {
            case e := <-eventChan:
                w.handleEvent(e)
            case <-errChan:
                // 重连逻辑
            case <-w.stopCh:
                return
            }
        }
    }()
    return nil
}

func (w *Watcher) handleEvent(e events.Message) {
    if e.Type != "container" {
        return
    }

    var ev ContainerEvent
    ev.ContainerID = e.ID

    switch e.Action {
    case "start":
        ev.Type = "start"
        // 获取 PID
        info, _ := w.client.ContainerInspect(context.Background(), e.ID)
        ev.Pid = info.State.Pid
    case "die":
        ev.Type = "stop"
    case "destroy":
        ev.Type = "delete"
    default:
        return
    }

    w.eventCh <- ev
}

func (w *Watcher) Events() <-chan ContainerEvent {
    return w.eventCh
}

func (w *Watcher) Stop() {
    close(w.stopCh)
}
```

## 十、关键要点

1. **统一接口**: 使用 Runtime 接口抽象不同的容器运行时
2. **事件驱动**: 通过事件回调触发容器生命周期管理
3. **重连机制**: 运行时连接断开后自动重连
4. **PID 验证**: Containerd 需要通过 PID 验证停止事件
5. **元数据缓存**: 可以缓存容器信息减少 API 调用
