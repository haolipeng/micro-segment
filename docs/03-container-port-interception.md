# 容器端口拦截机制

## 一、网络命名空间操作流程

```go
// port.go:744-872 - InterceptContainerPorts 函数

func InterceptContainerPorts(pid int, existPairs []*InterceptPair) ([]*InterceptPair, error) {
    // 1. 锁定 OS 线程 (防止命名空间切换时线程调度)
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()

    // 2. 保存当前命名空间
    curNs, _ := netns.Get()
    defer curNs.Close()

    // 3. 获取容器命名空间: /proc/{pid}/ns/net
    containerNs, _ := netns.GetFromPath(netns_path)

    // 4. 获取 Enforcer 命名空间
    dstNs, _ := netns.GetFromPath(workingNsPath)

    // 5. 切换到 Enforcer 命名空间，读取现有端口
    netns.Set(dstNs)
    exLinks, _ := netlink.LinkList()

    // 6. 切换到容器命名空间，执行端口拉取
    netns.Set(containerNs)
    intcpPairs, pulled, _ := pullAllContainerPorts(pid, int(dstNs), existPairMap, exLinks)

    // 7. 切换回 Enforcer 命名空间，附加 TC 规则
    netns.Set(dstNs)
    for _, pair := range intcpPairs {
        pair.UCMAC, pair.BCMAC = piper.AttachPortPair(pair)
        // 激活端口
        netlink.LinkSetUp(link)
    }

    // 8. 恢复原始命名空间
    netns.Set(curNs)
    return intcpPairs, nil
}
```

## 二、端口拉取详细流程

```go
// port.go:188-351 - pullContainerPort 函数

func pullContainerPort(link netlink.Link, addrs []netlink.Addr, pid, dstNs int, ...) {
    // 原始状态: 容器中有 eth0 (MAC=aa:bb:cc:dd:ee:ff, IP=172.17.0.2)

    // 步骤 1: 禁用原端口
    netlink.LinkSetDown(link)

    // 步骤 2: 重命名 eth0 → vex-{pid}-eth0 (将成为 exPort)
    netlink.LinkSetName(link, exPortName)

    // 步骤 3: 移除原端口的 IP 地址
    for _, addr := range addrs {
        netlink.AddrDel(link, &addr)
    }

    // 步骤 4: 临时修改 MAC 地址
    netlink.LinkSetHardwareAddr(link, "00:01:02:03:04:05")

    // 步骤 5: 创建新的 veth 对
    //   一端: eth0 (保留原名，作为 local port 留在容器中)
    //   另一端: vin-{pid}-eth0 (作为 inPort)
    veth := &linkVeth{
        LinkAttrs: netlink.LinkAttrs{
            Name:  attrs.Name,        // "eth0"
            Index: localPortIndex,    // 指定索引
        },
        PeerName:  inPortName,        // "vin-{pid}-eth0"
        PeerIndex: inPortIndex,
    }
    vethAdd(veth)

    // 步骤 6: 配置 local port (新 eth0)
    //   - 设置原容器 MAC
    //   - 添加原 IP 地址
    //   - 启用端口
    netlink.LinkSetHardwareAddr(local, attrs.HardwareAddr)
    netlink.AddrAdd(local, &addr)
    netlink.LinkSetUp(local)

    // 步骤 7: 移动 inPort 到 Enforcer 命名空间
    netlink.LinkSetNsFd(peer, dstNs)

    // 步骤 8: 恢复 exPort 的 MAC 并移动到 Enforcer 命名空间
    netlink.LinkSetHardwareAddr(link, localMAC)
    netlink.LinkSetNsFd(link, dstNs)
}
```

## 三、端口拉取前后对比

```
拉取前 (容器命名空间):
┌────────────────────────┐
│  eth0                  │
│  MAC: aa:bb:cc:dd:ee:ff│
│  IP: 172.17.0.2        │
│  ↕ veth pair           │
│  宿主机: vethXXX       │
└────────────────────────┘

拉取后:
┌─────────────────────────────────────────────────────────┐
│ 容器命名空间                                             │
│  eth0 (新建 local port)                                 │
│  MAC: aa:bb:cc:dd:ee:ff (保持不变)                      │
│  IP: 172.17.0.2 (保持不变)                              │
│  ↕ veth pair (新建)                                     │
└───────────────────────┬─────────────────────────────────┘
                        │
┌───────────────────────▼─────────────────────────────────┐
│ Enforcer 命名空间                                        │
│  vin-{pid}-eth0 (inPort) ← veth 对端                    │
│  vex-{pid}-eth0 (exPort) ← 原 eth0，重命名后移入         │
│  ↕ 原 veth pair                                         │
│  宿主机: vethXXX (原对端)                                │
└─────────────────────────────────────────────────────────┘
```

## 四、虚拟网桥 (vbr-neuv) 设计

### 4.1 创建逻辑

```go
// port.go:116-150 - createNVPorts 函数

func createNVPorts(jumboframe bool) {
    // 创建 veth 对作为虚拟网桥
    veth := &linkVeth{
        LinkAttrs: netlink.LinkAttrs{
            Name:  nvVbrPortName,              // "vbr-neuv"
            Index: inPortIndexBase,            // 10000000 (大索引避免冲突)
            MTU:   share.NV_VBR_PORT_MTU,      // 1500 或 9000 (jumbo)
        },
        PeerName:  nvVthPortName,              // "vth-neuv"
        PeerIndex: inPortIndexBase + 1,
    }
    vethAdd(veth)

    // 启用两端
    netlink.LinkSetUp(vbr)
    netlink.LinkSetUp(vth)

    // 关闭 offload 以确保软件处理
    DisableOffload(nvVbrPortName)
}
```

### 4.2 虚拟网桥的作用

```
所有容器流量汇聚点:

Container A ──→ vin-A ──→ vbr-neuv ──→ vth-neuv ──→ DP 数据平面
Container B ──→ vin-B ──↗                           (DPI 检测)
Container C ──→ vin-C ──↗                              ↓
                                                   策略判决
                                                      ↓
                                              ┌───────┴───────┐
                                              ↓               ↓
                                            允许            拒绝
                                              ↓
                                        vbr-neuv → 返回对应容器
```

## 五、完整数据流示意

### 容器 A 访问容器 B (FWD 模式)

```
1. 容器 A 发起请求
   App → eth0(A) → vin-A

2. TC 规则处理 (vin-A ingress)
   匹配源 MAC = A 的 MAC
   → pedit: 修改源 MAC 为 UCMAC-A
   → mirred: mirror 到 vbr-neuv

3. 虚拟网桥接收
   vbr-neuv → vth-neuv → DP 数据平面

4. DPI 检测 + 策略判决
   - 应用层协议识别 (HTTP/MySQL/...)
   - 访问控制策略匹配
   - 威胁检测

5. 允许通过
   vth-neuv → vbr-neuv → TC 规则
   匹配目标 UCMAC-A
   → pedit: 还原 MAC 为 A 的原 MAC
   → mirred: mirror 到 vex-A → 网络 → vex-B → ...

6. 容器 B 接收
   ... → vin-B → eth0(B) → App
```
