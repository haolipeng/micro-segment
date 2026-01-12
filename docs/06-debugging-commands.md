# 调试命令参考

## 一、TC 规则查看

```bash
# 查看特定端口的 TC filter 规则
tc filter show dev vex-{pid}-eth0 parent ffff:
tc filter show dev vin-{pid}-eth0 parent ffff:
tc filter show dev vbr-neuv parent ffff:

# 查看 QDisc
tc qdisc show dev vex-{pid}-eth0
tc qdisc show dev vin-{pid}-eth0
tc qdisc show dev vbr-neuv

# 查看所有 qdisc
tc qdisc show

# 查看统计信息
tc -s filter show dev vbr-neuv parent ffff:
```

## 二、iptables 规则查看

```bash
# 查看 NeuVector 自定义链
iptables -L NV_INPUT_PROXYMESH -n -v
iptables -L NV_OUTPUT_PROXYMESH -n -v

# 查看 NFQUEUE 规则
iptables -L -n -v | grep NFQUEUE

# 查看 nat 表规则
iptables -t nat -L -n -v

# 查看 mangle 表规则
iptables -t mangle -L -n -v
```

## 三、网络命名空间操作

```bash
# 列出所有网络命名空间
ip netns list

# 进入容器网络命名空间执行命令
nsenter -t {pid} -n ip link show
nsenter -t {pid} -n ip addr show
nsenter -t {pid} -n ip route show

# 查看容器进程的网络命名空间路径
ls -la /proc/{pid}/ns/net

# 在指定命名空间中执行命令
ip netns exec {ns-name} ip link show
```

## 四、网络接口查看

```bash
# 查看所有接口
ip link show

# 查看 NeuVector 创建的接口
ip link show | grep -E "(vbr-neuv|vth-neuv|vin-|vex-)"

# 查看接口详细信息
ip -d link show vbr-neuv

# 查看接口统计
ip -s link show vbr-neuv

# 查看 veth 对端
ethtool -S veth{xxx} | grep peer_ifindex
```

## 五、抓包分析

```bash
# 在虚拟网桥上抓包
tcpdump -i vbr-neuv -e -nn

# 抓取特定协议
tcpdump -i vbr-neuv -e -nn tcp port 80

# 抓取特定 MAC 地址
tcpdump -i vbr-neuv -e -nn ether host 4e:65:75:56:00:64

# 保存到文件
tcpdump -i vbr-neuv -e -nn -w /tmp/capture.pcap

# 在容器内抓包
nsenter -t {pid} -n tcpdump -i eth0 -nn
```

## 六、NFQUEUE 调试

```bash
# 查看 NFQUEUE 队列状态
cat /proc/net/netfilter/nfnetlink_queue

# 查看 conntrack 连接跟踪
conntrack -L

# 查看 netfilter 统计
cat /proc/net/netfilter/nf_conntrack_count
```

## 七、NeuVector 特定调试

```bash
# 查看 enforcer 容器日志
docker logs neuvector-enforcer-pod

# 进入 enforcer 容器
docker exec -it neuvector-enforcer-pod sh

# 查看 DP 数据平面进程
ps aux | grep dp

# 查看 Unix socket
ls -la /tmp/dp_listen.sock
ls -la /tmp/ctrl_listen.sock
```

## 八、常用排查流程

### 8.1 检查容器是否被拦截

```bash
# 1. 获取容器 PID
docker inspect {container-id} --format '{{.State.Pid}}'

# 2. 检查是否有对应的 vin/vex 端口
ip link show | grep "vin-{pid}"
ip link show | grep "vex-{pid}"

# 3. 检查 TC 规则是否存在
tc filter show dev vin-{pid}-eth0 parent ffff:
```

### 8.2 检查流量是否经过 enforcer

```bash
# 1. 在 vbr-neuv 上抓包
tcpdump -i vbr-neuv -nn -c 10

# 2. 检查 TC filter 统计
tc -s filter show dev vbr-neuv parent ffff:

# 3. 检查容器内发出的流量
nsenter -t {pid} -n tcpdump -i eth0 -nn -c 10
```

### 8.3 检查策略是否生效

```bash
# 1. 查看 iptables 规则命中计数
iptables -L NV_INPUT_PROXYMESH -n -v

# 2. 查看 enforcer 日志中的策略匹配
docker logs neuvector-enforcer-pod 2>&1 | grep -i policy
```

## 九、参考源码文件

### TC 层 (Go)
- `neuvector/agent/pipe/tc.go:1-425` - TC 驱动完整实现
- `neuvector/agent/pipe/port.go:1-1795` - 端口管理和命名空间操作
- `neuvector/agent/pipe/link_linux.go` - veth 对创建
- `neuvector/agent/pipe/ovs.go` - OVS 备选驱动

### 数据平面 (C)
- `neuvector/dp/nfq.c` - NFQUEUE 数据包接收
- `neuvector/dp/dpi/dpi_entry.c` - DPI 入口
- `neuvector/dp/dpi/dpi_session.c` - 会话管理
- `neuvector/dp/dpi/dpi_parser.c` - 协议解析器调度
- `neuvector/dp/dpi/parsers/*.c` - 各协议解析器
- `neuvector/dp/dpi/sig/*.c` - 威胁检测引擎
