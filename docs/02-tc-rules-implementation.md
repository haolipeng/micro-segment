# TC 规则实现机制

## 一、QDisc (排队规则) 创建

NeuVector 为每个拦截端口创建 **ingress qdisc**：

```go
// tc.go:61-65
func (d *tcPipeDriver) addQDisc(port string) {
    shell(fmt.Sprintf("tc qdisc add dev %v ingress", port))
}
```

**等效命令**：
```bash
tc qdisc add dev vex-1234-eth0 ingress
tc qdisc add dev vin-1234-eth0 ingress
tc qdisc add dev vbr-neuv ingress
```

## 二、MAC 地址标识方案

NeuVector 使用特殊 MAC 地址标识已处理的数据包，防止循环处理：

```go
// tc.go:100-111
func (d *tcPipeDriver) AttachPortPair(pair *InterceptPair) (net.HardwareAddr, net.HardwareAddr) {
    d.attachPort(pair.inPort)
    idx := d.attachPort(pair.exPort)

    // 4e:65:75:56 = "NeuV" 的十六进制
    mac_str = fmt.Sprintf("4e:65:75:56:%02x:%02x", (idx>>8)&0xff, idx&0xff)
    ucmac, _ := net.ParseMAC(mac_str)

    // 广播标识 MAC
    mac_str = fmt.Sprintf("ff:ff:ff:00:%02x:%02x", (idx>>8)&0xff, idx&0xff)
    bcmac, _ := net.ParseMAC(mac_str)

    return ucmac, bcmac
}
```

**示例**：端口索引为 100 (0x64)
- UCMAC: `4e:65:75:56:00:64`
- BCMAC: `ff:ff:ff:00:00:64`

## 三、TAP 模式 (监听/镜像模式)

TAP 模式用于**流量监控和学习**，不阻断流量：

```go
// tc.go:167-266 - TapPortPair 函数

// 入站流量规则 (外部 → 容器)
// 匹配目标 MAC 为容器 MAC 的 IP 数据包
cmd = fmt.Sprintf("tc filter add dev %v pref %v parent ffff: protocol ip "+
    "u32 match u8 0 1 at -14 "+                           // 单播标志检测
    "match u16 0x%02x%02x 0xffff at -14 "+               // 目标 MAC 前2字节
    "match u32 0x%02x%02x%02x%02x 0xffffffff at -12 "+   // 目标 MAC 后4字节
    "action mirred egress mirror dev %v "+               // 1. 镜像到 inPort (送达容器)
    "action pedit munge offset -14 u16 set 0x%02x%02x "+ // 2. 修改 MAC 为 UCMAC
    "munge offset -12 u32 set 0x%02x%02x%02x%02x pipe "+
    "action mirred egress mirror dev %v",                // 3. 镜像到 vbr-neuv (送给 enforcer)
    pair.exPort, tcPrefBase+1,
    pair.MAC[0], pair.MAC[1], pair.MAC[2], pair.MAC[3], pair.MAC[4], pair.MAC[5],
    pair.inPort,                                          // 转发给容器
    pair.UCMAC[0], pair.UCMAC[1], ...                    // 标记为已处理
    nvVbrPortName)                                        // 送给 enforcer 分析

// 出站流量规则 (容器 → 外部)
// 匹配源 MAC 为容器 MAC 的 IP 数据包
cmd = fmt.Sprintf("tc filter add dev %v pref %v parent ffff: protocol ip "+
    "u32 match u8 0 1 at -14 "+
    "match u32 0x%02x%02x%02x%02x 0xffffffff at -8 "+    // 源 MAC 后4字节
    "match u16 0x%02x%02x 0xffff at -4 "+               // 源 MAC 前2字节
    "action mirred egress mirror dev %v "+               // 1. 镜像到 exPort (送出网络)
    "action pedit munge offset -8 u32 set ... pipe "+    // 2. 修改源 MAC 为 UCMAC
    "action mirred egress mirror dev %v",                // 3. 镜像到 vbr-neuv
    pair.inPort, tcPrefBase+1, ...)

// 防循环规则：丢弃来自 enforcer 的已标记数据包
cmd = fmt.Sprintf("tc filter add dev %v pref %v parent ffff: protocol all "+
    "u32 match u16 0x%02x%02x 0xffff at -14 "+
    "match u32 0x%02x%02x%02x%02x 0xffffffff at -12 "+
    "action drop",
    nvVbrPortName, exInfo.pref, pair.UCMAC[0], ...)
```

**TAP 模式数据流**：
```
入站: 网络 → exPort → [mirror→inPort] + [pedit MAC→UCMAC, mirror→vbr-neuv]
                         ↓                        ↓
                      容器收到               Enforcer 分析

出站: 容器 → inPort → [mirror→exPort] + [pedit MAC→UCMAC, mirror→vbr-neuv]
                         ↓                        ↓
                      网络发出               Enforcer 分析
```

## 四、FWD 模式 (转发/强制模式)

FWD 模式用于**策略强制执行**，Enforcer 可以阻断流量：

```go
// tc.go:268-367 - FwdPortPair 函数

// 入站: 只转发给 enforcer，由 enforcer 决定是否送达容器
cmd = fmt.Sprintf("tc filter add dev %v pref %v parent ffff: protocol ip "+
    "u32 match u8 0 1 at -14 "+
    "match u16 0x%02x%02x 0xffff at -14 match u32 0x%02x%02x%02x%02x 0xffffffff at -12 "+
    "action pedit munge offset -14 u16 set 0x%02x%02x ... pipe "+  // 先标记
    "action mirred egress mirror dev %v",                          // 只送 enforcer
    pair.exPort, tcPrefBase+1, ..., nvVbrPortName)

// Enforcer 处理后转发规则：将标记的包修改回原 MAC 后送达容器
cmd = fmt.Sprintf("tc filter add dev %v pref %v parent ffff: protocol all "+
    "u32 match u16 0x%02x%02x 0xffff at -14 match u32 0x%02x%02x%02x%02x 0xffffffff at -12 "+
    "action pedit munge offset -14 u16 set 0x%02x%02x ... pipe "+  // 还原 MAC
    "action mirred egress mirror dev %v",                          // 转发给容器
    nvVbrPortName, exInfo.pref, pair.UCMAC..., pair.MAC..., pair.inPort)
```

**FWD 模式数据流**：
```
入站: 网络 → exPort → [pedit→UCMAC, mirror→vbr-neuv]
                              ↓
                      Enforcer 判决
                              ↓
              ┌───────────────┴───────────────┐
              ↓ (允许)                        ↓ (拒绝)
      vbr-neuv → [pedit→原MAC, mirror→inPort]  丢弃
                              ↓
                          容器收到
```

## 五、TC Filter u32 匹配语法解析

```
u32 match u{8|16|32} <value> <mask> at <offset>

偏移量参考 (相对于 IP 头):
at -14: 以太网目标 MAC 地址 (前2字节)
at -12: 以太网目标 MAC 地址 (后4字节)
at -8:  以太网源 MAC 地址 (后4字节)
at -4:  以太网源 MAC 地址 (前2字节)

示例: match u8 0 1 at -14
解释: 在偏移 -14 处匹配 1 字节, 值为 0, 掩码为 1
      即检测以太网目标地址第一字节的最低位 = 0 (单播地址)
```

## 六、多驱动支持

NeuVector 支持多种流量控制驱动：

```go
// port.go:1769-1781
func Open(driver string, ...) {
    switch driver {
    case PIPE_TC:
        piper = &tcPipe      // Linux TC (默认)
    case PIPE_OVS:
        piper = &ovsPipe     // Open vSwitch
    case PIPE_NOTC:
        piper = &notcPipe    // 无 TC (仅学习模式)
    case PIPE_CLM:
        piper = &clmPipe     // Calico 集成
    }
}
```

| 驱动 | 使用场景 | 特点 |
|------|---------|------|
| `tc` | 标准 Linux 环境 | 无需额外组件，内核原生支持 |
| `ovs` | 使用 OVS 的环境 | 更高性能，支持 OpenFlow |
| `no_tc` | 纯学习/监控模式 | 不创建任何规则 |
| `clm` | Calico 网络 | 与 Calico 策略集成 |
