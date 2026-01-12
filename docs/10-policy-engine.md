# 策略引擎

## 一、概述

策略引擎负责：
- 定义和管理网络访问控制策略
- 将策略转换为数据平面可执行的规则
- 匹配流量和策略，决定允许或拒绝
- 支持策略的动态更新

## 二、策略架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        策略引擎架构                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                     策略存储                             │   │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │   │
│  │  │ 组策略      │  │ 工作负载策略 │  │ 地址映射    │      │   │
│  │  │ GroupPolicy │  │ WorkloadPol │  │ AddrMap    │      │   │
│  │  └─────────────┘  └─────────────┘  └─────────────┘      │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                     策略解析器                           │   │
│  │  - 解析组策略为工作负载规则                               │   │
│  │  - 构建 IP/端口匹配规则                                  │   │
│  │  - 处理通配符和地址组                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                     策略分发器                           │   │
│  │  - 计算策略差异                                          │   │
│  │  - 增量更新数据平面                                       │   │
│  │  - 管理策略版本                                          │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │                     数据平面                             │   │
│  │  - 执行策略匹配                                          │   │
│  │  - 返回 ACCEPT/DROP/RESET 判决                          │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## 三、核心数据结构

### 3.1 策略引擎结构

**源码位置**: `agent/policy/type.go:34-48`

```go
type Engine struct {
    // 网络策略映射: 工作负载 ID → 策略信息
    NetworkPolicy     map[string]*WorkloadIPPolicyInfo

    // 进程策略映射
    ProcessPolicy     map[string]*share.CLUSProcessProfile

    // 主机信息
    HostID            string
    HostIPs           utils.Set
    TunnelIP          []net.IPNet

    // 策略地址映射
    PolicyAddrMap     map[string]share.CLUSSubnet
    HostPolicyAddrMap map[string]share.CLUSSubnet

    // 同步锁
    Mutex             sync.Mutex
}
```

### 3.2 工作负载策略信息

```go
type WorkloadIPPolicyInfo struct {
    // 规则映射: 规则 ID → 规则详情
    RuleMap    map[string]*dp.DPPolicyIPRule

    // 数据平面策略结构
    Policy     dp.DPWorkloadIPPolicy

    // 状态标志
    Configured bool   // 是否已配置到 DP
    SkipPush   bool   // 是否跳过推送
    HostMode   bool   // 是否主机网络模式
    CapIntcp   bool   // 是否可拦截
    PolVer     uint16 // 策略版本
    Nbe        bool   // 非后端模式
}
```

### 3.3 数据平面策略结构

```go
// dp/types.go
type DPWorkloadIPPolicy struct {
    WorkloadID  string              // 工作负载 ID
    Mode        string              // 策略模式: Discover/Monitor/Protect
    DefAction   uint8               // 默认动作
    ApplyDir    uint8               // 应用方向
    Rules       []DPPolicyIPRule    // 规则列表
}

type DPPolicyIPRule struct {
    ID          uint32              // 规则 ID
    SrcIP       net.IP              // 源 IP
    SrcIPR      net.IP              // 源 IP 范围结束
    DstIP       net.IP              // 目标 IP
    DstIPR      net.IP              // 目标 IP 范围结束
    SrcPort     uint16              // 源端口
    SrcPortR    uint16              // 源端口范围结束
    DstPort     uint16              // 目标端口
    DstPortR    uint16              // 目标端口范围结束
    IPProto     uint8               // IP 协议
    Action      uint8               // 动作: ALLOW/DENY
    Ingress     bool                // 是否入站规则
    FQDN        string              // 域名 (可选)
    App         uint32              // 应用 ID (可选)
}
```

### 3.4 组策略结构

```go
// share/types.go
type CLUSGroupIPPolicy struct {
    ID        string                // 组 ID
    Name      string                // 组名称
    Mode      string                // 策略模式
    DefAction uint8                 // 默认动作
    Rules     []CLUSGroupIPRule     // 组规则列表
}

type CLUSGroupIPRule struct {
    ID          uint32
    From        string              // 源组/地址
    To          string              // 目标组/地址
    Ports       string              // 端口 (如 "tcp/80,443")
    Application string              // 应用名称
    Action      uint8               // 动作
    Priority    uint16              // 优先级
}
```

## 四、策略更新流程

### 4.1 UpdateNetworkPolicy - 更新网络策略

**源码位置**: `agent/policy/network.go:1382-1447`

```go
func (e *Engine) UpdateNetworkPolicy(ps []share.CLUSGroupIPPolicy,
    newPolicy map[string]*WorkloadIPPolicyInfo) utils.Set {

    e.Mutex.Lock()
    defer e.Mutex.Unlock()

    // ========== 步骤 1: 解析组策略 ==========
    newPolicyAddrMap := make(map[string]share.CLUSSubnet)
    newHostPolicyAddrMap := make(map[string]share.CLUSSubnet)

    parseGroupIPPolicy(ps, newPolicy, newPolicyAddrMap, newHostPolicyAddrMap)

    // ========== 步骤 2: 比较新旧策略 ==========
    hostModeChanged := utils.NewSet()

    for id, pInfo := range newPolicy {
        oldInfo, exists := e.NetworkPolicy[id]

        if !exists {
            // 新策略: 添加到数据平面
            if pInfo.Configured && !pInfo.SkipPush {
                dp.DPCtrlConfigPolicy(&pInfo.Policy, C.CFG_ADD)
            }
        } else if policyChanged(oldInfo, pInfo) {
            // 策略变更: 修改数据平面配置
            if pInfo.Configured && !pInfo.SkipPush {
                dp.DPCtrlConfigPolicy(&pInfo.Policy, C.CFG_MODIFY)
            }
        }

        // 检查主机模式变更
        if exists && oldInfo.HostMode != pInfo.HostMode {
            hostModeChanged.Add(id)
        }
    }

    // ========== 步骤 3: 删除过期策略 ==========
    for id, oldInfo := range e.NetworkPolicy {
        if _, exists := newPolicy[id]; !exists {
            if oldInfo.Configured && !oldInfo.SkipPush {
                dp.DPCtrlConfigPolicy(&oldInfo.Policy, C.CFG_DEL)
            }
        }
    }

    // ========== 步骤 4: 更新策略地址映射 ==========
    dp.DPCtrlConfigPolicyAddr(newPolicyAddrMap)

    // ========== 步骤 5: 保存新策略 ==========
    e.NetworkPolicy = newPolicy
    e.PolicyAddrMap = newPolicyAddrMap
    e.HostPolicyAddrMap = newHostPolicyAddrMap

    return hostModeChanged
}
```

### 4.2 parseGroupIPPolicy - 解析组策略

```go
func parseGroupIPPolicy(ps []share.CLUSGroupIPPolicy,
    newPolicy map[string]*WorkloadIPPolicyInfo,
    newPolicyAddrMap, newHostPolicyAddrMap map[string]share.CLUSSubnet) {

    for _, gp := range ps {
        // 获取组成员的工作负载
        workloads := getGroupWorkloads(gp.ID)

        for _, wlID := range workloads {
            pInfo, ok := newPolicy[wlID]
            if !ok {
                pInfo = &WorkloadIPPolicyInfo{
                    RuleMap: make(map[string]*dp.DPPolicyIPRule),
                    Policy: dp.DPWorkloadIPPolicy{
                        WorkloadID: wlID,
                        Mode:       gp.Mode,
                        DefAction:  gp.DefAction,
                    },
                }
                newPolicy[wlID] = pInfo
            }

            // 解析每条组规则
            for _, rule := range gp.Rules {
                dpRule := parseGroupRule(rule, wlID, newPolicyAddrMap)
                if dpRule != nil {
                    ruleKey := fmt.Sprintf("%d", dpRule.ID)
                    pInfo.RuleMap[ruleKey] = dpRule
                    pInfo.Policy.Rules = append(pInfo.Policy.Rules, *dpRule)
                }
            }
        }
    }
}
```

### 4.3 parseGroupRule - 解析单条规则

```go
func parseGroupRule(rule share.CLUSGroupIPRule, wlID string,
    addrMap map[string]share.CLUSSubnet) *dp.DPPolicyIPRule {

    dpRule := &dp.DPPolicyIPRule{
        ID:     rule.ID,
        Action: rule.Action,
    }

    // 解析源地址
    if rule.From == "any" {
        dpRule.SrcIP = net.IPv4zero
        dpRule.SrcIPR = net.IPv4bcast
    } else if ip := net.ParseIP(rule.From); ip != nil {
        dpRule.SrcIP = ip
        dpRule.SrcIPR = ip
    } else if _, subnet, err := net.ParseCIDR(rule.From); err == nil {
        dpRule.SrcIP = subnet.IP
        dpRule.SrcIPR = lastIP(subnet)
    } else {
        // 地址组引用
        if addr, ok := addrMap[rule.From]; ok {
            dpRule.SrcIP = addr.IP
            dpRule.SrcIPR = lastIP(&addr.IPNet)
        }
    }

    // 解析目标地址
    // ... 类似源地址解析

    // 解析端口
    parsePorts(rule.Ports, dpRule)

    // 确定方向
    if rule.From == wlID || isWorkloadInGroup(wlID, rule.From) {
        dpRule.Ingress = false  // 出站规则
    } else {
        dpRule.Ingress = true   // 入站规则
    }

    return dpRule
}

func parsePorts(portsStr string, dpRule *dp.DPPolicyIPRule) {
    // 格式: "tcp/80,443" 或 "any"
    parts := strings.Split(portsStr, "/")
    if len(parts) != 2 {
        return
    }

    // 解析协议
    switch strings.ToLower(parts[0]) {
    case "tcp":
        dpRule.IPProto = syscall.IPPROTO_TCP
    case "udp":
        dpRule.IPProto = syscall.IPPROTO_UDP
    case "icmp":
        dpRule.IPProto = syscall.IPPROTO_ICMP
    case "any":
        dpRule.IPProto = 0
    }

    // 解析端口
    portParts := strings.Split(parts[1], ",")
    for _, p := range portParts {
        if strings.Contains(p, "-") {
            // 端口范围
            rangeParts := strings.Split(p, "-")
            dpRule.DstPort, _ = strconv.ParseUint(rangeParts[0], 10, 16)
            dpRule.DstPortR, _ = strconv.ParseUint(rangeParts[1], 10, 16)
        } else {
            // 单个端口
            port, _ := strconv.ParseUint(p, 10, 16)
            dpRule.DstPort = uint16(port)
            dpRule.DstPortR = uint16(port)
        }
    }
}
```

## 五、策略推送到数据平面

### 5.1 PushNetworkPolicyToDP

**源码位置**: `agent/policy/network.go:1477+`

```go
func (e *Engine) PushNetworkPolicyToDP() {
    e.Mutex.Lock()
    defer e.Mutex.Unlock()

    // 推送所有已配置的策略
    for _, pInfo := range e.NetworkPolicy {
        if !pInfo.Configured || pInfo.SkipPush {
            continue
        }
        dp.DPCtrlConfigPolicy(&pInfo.Policy, C.CFG_ADD)
    }

    // 推送策略地址映射
    dp.DPCtrlConfigPolicyAddr(e.PolicyAddrMap)
}
```

### 5.2 数据平面策略配置接口

```go
// dp/dp.go
func DPCtrlConfigPolicy(policy *DPWorkloadIPPolicy, action int) error {
    msg := DPMsgPolicy{
        Action: action,
        Policy: *policy,
    }

    // 序列化消息
    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }

    // 通过 Unix socket 发送给 DP
    return sendToDP(DP_MSG_POLICY, data)
}

func DPCtrlConfigPolicyAddr(addrMap map[string]share.CLUSSubnet) error {
    msg := DPMsgPolicyAddr{
        AddrMap: addrMap,
    }

    data, err := json.Marshal(msg)
    if err != nil {
        return err
    }

    return sendToDP(DP_MSG_POLICY_ADDR, data)
}
```

## 六、策略模式

NeuVector 支持三种策略模式：

### 6.1 Discover (发现模式)

- 学习容器之间的通信模式
- 不阻断任何流量
- 生成建议的策略规则

```go
const PolicyModeDiscover = "Discover"

// Discover 模式下，默认允许所有流量
func getDiscoverPolicy(wlID string) *DPWorkloadIPPolicy {
    return &DPWorkloadIPPolicy{
        WorkloadID: wlID,
        Mode:       PolicyModeDiscover,
        DefAction:  C.DP_POLICY_ACTION_ALLOW,
    }
}
```

### 6.2 Monitor (监控模式)

- 按策略匹配流量
- 违规流量记录日志但不阻断
- 用于策略验证

```go
const PolicyModeMonitor = "Monitor"

// Monitor 模式下，违规流量只记录不阻断
func getMonitorPolicy(wlID string, rules []DPPolicyIPRule) *DPWorkloadIPPolicy {
    return &DPWorkloadIPPolicy{
        WorkloadID: wlID,
        Mode:       PolicyModeMonitor,
        DefAction:  C.DP_POLICY_ACTION_VIOLATE,  // 违规但不阻断
        Rules:      rules,
    }
}
```

### 6.3 Protect (保护模式)

- 严格按策略执行
- 违规流量被阻断
- 生产环境使用

```go
const PolicyModeProtect = "Protect"

// Protect 模式下，违规流量被阻断
func getProtectPolicy(wlID string, rules []DPPolicyIPRule) *DPWorkloadIPPolicy {
    return &DPWorkloadIPPolicy{
        WorkloadID: wlID,
        Mode:       PolicyModeProtect,
        DefAction:  C.DP_POLICY_ACTION_DENY,
        Rules:      rules,
    }
}
```

## 七、策略匹配逻辑

数据平面的策略匹配在 C 代码中实现：

**源码位置**: `dp/dpi/dpi_policy.c`

```c
// 策略匹配入口
int dpi_policy_check(dpi_packet_t *p) {
    dpi_session_t *s = p->session;
    dpi_policy_desc_t *pd = &s->policy_desc;

    // 获取工作负载策略
    dpi_policy_t *pol = get_workload_policy(p->ep_mac);
    if (pol == NULL) {
        return DPI_ACTION_NONE;
    }

    // 遍历规则列表
    for (int i = 0; i < pol->num_rules; i++) {
        dpi_policy_rule_t *rule = &pol->rules[i];

        // 检查方向
        if (rule->ingress != p->ingress) {
            continue;
        }

        // 检查源 IP
        if (!ip_in_range(p->src_ip, rule->src_ip, rule->src_ip_r)) {
            continue;
        }

        // 检查目标 IP
        if (!ip_in_range(p->dst_ip, rule->dst_ip, rule->dst_ip_r)) {
            continue;
        }

        // 检查协议
        if (rule->ip_proto != 0 && rule->ip_proto != p->ip_proto) {
            continue;
        }

        // 检查端口
        if (!port_in_range(p->dst_port, rule->dst_port, rule->dst_port_r)) {
            continue;
        }

        // 检查应用 (如果指定)
        if (rule->app != 0 && rule->app != s->app) {
            continue;
        }

        // 规则匹配，返回动作
        pd->rule_id = rule->id;
        pd->action = rule->action;
        return rule->action;
    }

    // 无规则匹配，返回默认动作
    pd->rule_id = 0;
    pd->action = pol->def_action;
    return pol->def_action;
}

// IP 范围检查
static inline bool ip_in_range(uint32_t ip, uint32_t start, uint32_t end) {
    return ip >= start && ip <= end;
}

// 端口范围检查
static inline bool port_in_range(uint16_t port, uint16_t start, uint16_t end) {
    if (start == 0 && end == 0) {
        return true;  // 任意端口
    }
    return port >= start && port <= end;
}
```

## 八、策略变更处理

### 8.1 策略更新触发

```go
// agent/system.go
func systemUpdatePolicy(data []byte) bool {
    // 解析策略数据
    var ps share.CLUSGroupIPPolicyVer
    if err := json.Unmarshal(data, &ps); err != nil {
        return false
    }

    // 读取组策略
    groupIPPolicy := cluster.GetGroupIPPolicies()

    // 初始化新策略映射
    newPolicy := make(map[string]*WorkloadIPPolicyInfo)

    // 更新网络策略
    hostModeChanged := pe.UpdateNetworkPolicy(groupIPPolicy, newPolicy)

    // 处理主机模式变更的容器
    for id := range hostModeChanged.Iter() {
        updateContainerPolicyMode(id.(string))
    }

    return true
}
```

### 8.2 容器策略模式更新

```go
func updateContainerPolicyMode(id string, mode string) {
    c, ok := gInfoReadActiveContainer(id)
    if !ok {
        return
    }

    // 根据新模式决定是否切换 inline
    inline := (mode == PolicyModeProtect)

    if c.inline != inline {
        changeContainerWire(c, inline, c.quar, "")
    }
}
```

## 九、简化实现示例

```go
package policy

import (
    "net"
    "sync"
)

type Engine struct {
    policies map[string]*WorkloadPolicy
    mu       sync.RWMutex
}

type WorkloadPolicy struct {
    WorkloadID string
    Mode       string  // "discover", "monitor", "protect"
    DefAction  string  // "allow", "deny"
    Rules      []Rule
}

type Rule struct {
    ID        uint32
    SrcIP     net.IP
    SrcIPEnd  net.IP
    DstIP     net.IP
    DstIPEnd  net.IP
    DstPort   uint16
    DstPortEnd uint16
    Protocol  string  // "tcp", "udp", "icmp", "any"
    Action    string  // "allow", "deny"
    Ingress   bool
}

func NewEngine() *Engine {
    return &Engine{
        policies: make(map[string]*WorkloadPolicy),
    }
}

// 添加或更新策略
func (e *Engine) SetPolicy(policy *WorkloadPolicy) {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.policies[policy.WorkloadID] = policy
}

// 删除策略
func (e *Engine) DeletePolicy(workloadID string) {
    e.mu.Lock()
    defer e.mu.Unlock()
    delete(e.policies, workloadID)
}

// 获取策略
func (e *Engine) GetPolicy(workloadID string) *WorkloadPolicy {
    e.mu.RLock()
    defer e.mu.RUnlock()
    return e.policies[workloadID]
}

// 匹配策略
func (e *Engine) Match(workloadID string, pkt *Packet) string {
    e.mu.RLock()
    policy := e.policies[workloadID]
    e.mu.RUnlock()

    if policy == nil {
        return "allow"  // 无策略默认允许
    }

    // 遍历规则
    for _, rule := range policy.Rules {
        if e.matchRule(&rule, pkt) {
            return rule.Action
        }
    }

    // 返回默认动作
    return policy.DefAction
}

func (e *Engine) matchRule(rule *Rule, pkt *Packet) bool {
    // 检查方向
    if rule.Ingress != pkt.Ingress {
        return false
    }

    // 检查源 IP
    if !ipInRange(pkt.SrcIP, rule.SrcIP, rule.SrcIPEnd) {
        return false
    }

    // 检查目标 IP
    if !ipInRange(pkt.DstIP, rule.DstIP, rule.DstIPEnd) {
        return false
    }

    // 检查协议
    if rule.Protocol != "any" && rule.Protocol != pkt.Protocol {
        return false
    }

    // 检查端口
    if !portInRange(pkt.DstPort, rule.DstPort, rule.DstPortEnd) {
        return false
    }

    return true
}

type Packet struct {
    SrcIP    net.IP
    DstIP    net.IP
    SrcPort  uint16
    DstPort  uint16
    Protocol string
    Ingress  bool
}

func ipInRange(ip, start, end net.IP) bool {
    if start == nil {
        return true
    }
    ipVal := ipToUint32(ip)
    startVal := ipToUint32(start)
    endVal := ipToUint32(end)
    return ipVal >= startVal && ipVal <= endVal
}

func portInRange(port, start, end uint16) bool {
    if start == 0 && end == 0 {
        return true
    }
    return port >= start && port <= end
}

func ipToUint32(ip net.IP) uint32 {
    ip = ip.To4()
    if ip == nil {
        return 0
    }
    return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// 从 YAML/JSON 加载策略
func (e *Engine) LoadPolicies(data []byte) error {
    // 解析策略配置
    // ...
    return nil
}
```

## 十、策略配置示例

### YAML 格式策略定义

```yaml
policies:
  - workload: "web-frontend"
    mode: "protect"
    default_action: "deny"
    rules:
      - id: 1
        from: "any"
        to: "self"
        ports: "tcp/80,443"
        action: "allow"
        comment: "允许入站 HTTP/HTTPS"

      - id: 2
        from: "self"
        to: "api-backend"
        ports: "tcp/8080"
        action: "allow"
        comment: "允许访问后端 API"

      - id: 3
        from: "self"
        to: "database"
        ports: "tcp/3306"
        action: "deny"
        comment: "禁止直接访问数据库"

  - workload: "api-backend"
    mode: "protect"
    default_action: "deny"
    rules:
      - id: 1
        from: "web-frontend"
        to: "self"
        ports: "tcp/8080"
        action: "allow"

      - id: 2
        from: "self"
        to: "database"
        ports: "tcp/3306"
        action: "allow"
```

## 十一、关键要点

1. **分层策略**: 组策略 → 工作负载策略 → 数据平面规则
2. **增量更新**: 只推送变更的策略，减少 DP 负载
3. **优先级匹配**: 规则按优先级顺序匹配
4. **三种模式**: Discover/Monitor/Protect 满足不同场景
5. **地址组**: 支持 IP 范围和组引用简化策略定义
6. **版本控制**: 策略带版本号避免重复推送
