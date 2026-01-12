# 策略可视化实现

## 一、概述

本文档详细介绍网络拓扑中的策略可视化实现，包括三层链接系统、策略动作映射和策略 ID 关联机制。

## 二、关键源文件

| 文件 | 路径 | 功能 |
|------|------|------|
| 链接类型定义 | `controller/cache/connect.go:34` | policyLink/graphLink/attrLink |
| 策略学习 | `controller/cache/learn.go:166-250` | 策略链接管理 |
| 图结构 | `controller/graph/graph.go` | 图数据结构 |
| 类型定义 | `controller/types.go` | PolicyAction 等 |

## 三、三层链接系统

### 3.1 链接类型定义

**源码位置**: `controller/cache/connect.go:34`

```go
const (
    policyLink = "policy"  // 策略链接 - 规则定义的允许通信
    graphLink  = "graph"   // 流量链接 - 实际发生的通信
    attrLink   = "attr"    // 属性链接 - 节点属性存储
)
```

### 3.2 三层链接关系

```
┌─────────────────────────────────────────────────────────────────┐
│                      三层链接系统                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Node A                                                         │
│  │                                                              │
│  ├── attrLink → Node A (自环)                                   │
│  │   └── nodeAttr { alias, external, workload, host, ... }     │
│  │                                                              │
│  ├── policyLink → Node B                                        │
│  │   └── polAttr { ports, apps, portsSeen }                    │
│  │       表示: 策略规则允许 A 访问 B                            │
│  │                                                              │
│  └── graphLink → Node B                                         │
│      └── graphAttr { bytes, sessions, severity, entries }      │
│          表示: A 实际访问了 B                                   │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  同一对节点可能同时存在 policyLink 和 graphLink:          │   │
│  │  - policyLink 存在, graphLink 不存在 → 允许但未发生       │   │
│  │  - policyLink 存在, graphLink 存在   → 允许且已发生       │   │
│  │  - policyLink 不存在, graphLink 存在 → 未允许但已发生(违规)│   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 3.3 属性结构对比

| 链接类型 | 属性类型 | 字段 | 用途 |
|---------|---------|------|------|
| attrLink | nodeAttr | alias, external, workload, host, managed | 节点元数据 |
| policyLink | polAttr | ports, apps, portsSeen | 策略规则定义 |
| graphLink | graphAttr | bytes, sessions, severity, policyAction, entries | 流量统计 |

## 四、策略边属性 (polAttr)

### 4.1 数据结构

**源码位置**: `controller/cache/connect.go:87-92`

```go
type polAttr struct {
    ports        utils.Set // string - 规则允许的端口集合
    portsSeen    utils.Set // string - 观察到的实际端口
    apps         utils.Set // uint32 - 规则允许的应用 ID
    lastRecalcAt int64     // 最后重新计算时间
}
```

### 4.2 用途说明

- **ports**: 策略规则中定义的允许端口，如 `{"80", "443", "8080-8090"}`
- **portsSeen**: 实际观察到的端口，用于学习模式
- **apps**: 允许的应用 ID，如 HTTP=1001, MySQL=1002
- **lastRecalcAt**: 上次重新计算时间，用于缓存优化

### 4.3 创建策略链接

```go
// 在学习模式下创建策略链接
func addPolicyLink(from, to string, conn *share.CLUSConnection) {
    // 获取现有策略链接
    attr := wlGraph.Attr(from, policyLink, to)

    if attr == nil {
        // 创建新策略链接
        pAttr := &polAttr{
            ports:     utils.NewSet(),
            portsSeen: utils.NewSet(),
            apps:      utils.NewSet(),
        }
        pAttr.apps.Add(conn.Application)
        pAttr.portsSeen.Add(formatPort(conn.ServerPort, conn.IPProto))

        wlGraph.AddLink(from, policyLink, to, pAttr)
    } else {
        // 更新现有策略链接
        pAttr := attr.(*polAttr)
        pAttr.apps.Add(conn.Application)
        pAttr.portsSeen.Add(formatPort(conn.ServerPort, conn.IPProto))
    }
}
```

## 五、流量边属性 (graphAttr)

### 5.1 数据结构

**源码位置**: `controller/cache/connect.go:79-85`

```go
type graphAttr struct {
    bytes        uint64                   // 总字节数
    sessions     uint32                   // 总会话数
    severity     uint8                    // 最高威胁等级
    policyAction uint8                    // 聚合策略动作
    entries      map[graphKey]*graphEntry // 详细条目
}
```

### 5.2 关键字段

- **bytes/sessions**: 聚合的流量统计
- **severity**: 所有条目中的最高威胁等级 (0-4)
- **policyAction**: 所有条目中的最高优先级策略动作
- **entries**: 按 5 元组索引的详细条目

### 5.3 策略动作优先级

```go
// 策略动作优先级 (值越大优先级越高)
const (
    DP_POLICY_ACTION_OPEN    = 0  // 开放 - 未限制
    DP_POLICY_ACTION_ALLOW   = 1  // 允许 - 有规则且符合
    DP_POLICY_ACTION_DENY    = 2  // 拒绝 - 有规则但不符合
    DP_POLICY_ACTION_VIOLATE = 3  // 违规 - Monitor 模式下的不符合
    DP_POLICY_ACTION_LEARN   = 4  // 学习 - Discover 模式
)

// 聚合时取最高优先级
func aggregatePolicyAction(actions []uint8) uint8 {
    var max uint8
    for _, action := range actions {
        if action > max {
            max = action
        }
    }
    return max
}
```

## 六、策略动作可视化

### 6.1 策略动作定义

**源码位置**: `controller/types.go:19-31`

```go
type PolicyAction uint8

const (
    PolicyActionOpen    PolicyAction = 0  // 开放
    PolicyActionAllow   PolicyAction = 1  // 允许
    PolicyActionDeny    PolicyAction = 2  // 拒绝
    PolicyActionViolate PolicyAction = 3  // 违规
)

func (a PolicyAction) String() string {
    switch a {
    case PolicyActionOpen:
        return "open"
    case PolicyActionAllow:
        return "allow"
    case PolicyActionDeny:
        return "deny"
    case PolicyActionViolate:
        return "violate"
    default:
        return "unknown"
    }
}
```

### 6.2 颜色映射

```typescript
// 前端颜色映射
const PolicyActionColors = {
  0: '#999999',  // Open - 灰色
  1: '#52c41a',  // Allow - 绿色
  2: '#ff4d4f',  // Deny - 红色
  3: '#ff7a45',  // Violate - 橙色
};

function getLinkColor(policyAction: number): string {
  return PolicyActionColors[policyAction] || '#999999';
}
```

### 6.3 样式映射

```typescript
// 边的样式映射
function getLinkStyle(policyAction: number) {
  switch (policyAction) {
    case 0: // Open
      return {
        color: '#999999',
        width: 1,
        dashArray: 'none',
        opacity: 0.4
      };
    case 1: // Allow
      return {
        color: '#52c41a',
        width: 1,
        dashArray: 'none',
        opacity: 0.6
      };
    case 2: // Deny
      return {
        color: '#ff4d4f',
        width: 2,
        dashArray: 'none',
        opacity: 0.8
      };
    case 3: // Violate
      return {
        color: '#ff7a45',
        width: 2,
        dashArray: '5,5',  // 虚线
        opacity: 0.8
      };
    default:
      return {
        color: '#999999',
        width: 1,
        dashArray: 'none',
        opacity: 0.4
      };
  }
}
```

### 6.4 CSS 样式定义

```css
/* 开放连接 - 灰色半透明 */
.link.open {
  stroke: #999999;
  stroke-opacity: 0.4;
  stroke-width: 1px;
}

/* 允许连接 - 绿色 */
.link.allow {
  stroke: #52c41a;
  stroke-opacity: 0.6;
  stroke-width: 1px;
}

/* 拒绝连接 - 红色加粗 */
.link.deny {
  stroke: #ff4d4f;
  stroke-opacity: 0.8;
  stroke-width: 2px;
}

/* 违规连接 - 橙色虚线 */
.link.violate {
  stroke: #ff7a45;
  stroke-opacity: 0.8;
  stroke-width: 2px;
  stroke-dasharray: 5, 5;
  animation: dash 0.5s linear infinite;
}

@keyframes dash {
  to {
    stroke-dashoffset: -10;
  }
}
```

## 七、策略 ID 关联

### 7.1 关联机制

每个 graphEntry 都存储了匹配的策略规则 ID：

```go
type graphEntry struct {
    // ... 其他字段
    policyAction uint8    // 策略动作
    policyID     uint32   // 策略规则 ID
    // ...
}
```

### 7.2 策略匹配流程

```
┌─────────────────────────────────────────────────────────────────┐
│                    策略匹配和 ID 关联流程                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. 数据平面 (DP) 检测到新连接                                   │
│     ├── 源 IP: 10.0.0.1 (Container A)                          │
│     ├── 目标 IP: 10.0.0.2 (Container B)                        │
│     └── 目标端口: 80/TCP                                        │
│     ↓                                                           │
│  2. DP 查询策略规则                                              │
│     ├── 遍历规则表                                               │
│     ├── 匹配: Rule #1001 (Allow A → B on port 80)              │
│     └── 记录: PolicyAction=ALLOW, PolicyID=1001                │
│     ↓                                                           │
│  3. Agent 上报连接                                               │
│     CLUSConnection {                                           │
│         ClientWL: "container-a",                               │
│         ServerWL: "container-b",                               │
│         ServerPort: 80,                                        │
│         PolicyAction: 1,  // ALLOW                             │
│         PolicyId: 1001,   // 匹配的规则 ID                      │
│     }                                                          │
│     ↓                                                           │
│  4. Controller 存储到图                                         │
│     graphEntry {                                               │
│         policyAction: 1,                                       │
│         policyID: 1001,                                        │
│     }                                                          │
│     ↓                                                           │
│  5. 前端查询并显示                                               │
│     ├── 获取边的 policyAction 和 policyID                       │
│     ├── 根据 policyAction 设置边颜色                            │
│     └── 支持点击边查看关联的策略规则详情                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 7.3 REST API 返回

```go
// 会话详情响应
type RESTConversationEntry struct {
    Bytes        uint64 `json:"bytes"`
    Sessions     uint32 `json:"sessions"`
    Port         string `json:"port"`
    Application  string `json:"application"`
    PolicyAction string `json:"policy_action"`
    PolicyID     uint32 `json:"policy_id"`      // 关联的策略 ID
    LastSeenAt   string `json:"last_seen_at"`
    // ...
}
```

### 7.4 前端策略关联

```typescript
// 点击边时获取关联策略
async function handleEdgeClick(edge: GraphLink) {
  // 获取会话详情
  const detail = await getConversationDetail(edge.from, edge.to);

  // 收集所有关联的策略 ID
  const policyIds = new Set<number>();
  detail.entries.forEach(entry => {
    if (entry.policy_id > 0) {
      policyIds.add(entry.policy_id);
    }
  });

  // 获取策略详情
  const policies = await Promise.all(
    Array.from(policyIds).map(id => getPolicyRule(id))
  );

  // 显示策略信息
  showPolicyPanel(policies);
}
```

## 八、策略视图模式

### 8.1 双视图切换

```typescript
type ViewMode = 'traffic' | 'policy';

interface TopologyView {
  mode: ViewMode;
  showTrafficLinks: boolean;   // 显示流量链接
  showPolicyLinks: boolean;    // 显示策略链接
  showUnusedPolicies: boolean; // 显示未使用的策略
}

// 流量视图 - 只显示实际发生的连接
const trafficView: TopologyView = {
  mode: 'traffic',
  showTrafficLinks: true,
  showPolicyLinks: false,
  showUnusedPolicies: false,
};

// 策略视图 - 显示策略规则定义
const policyView: TopologyView = {
  mode: 'policy',
  showTrafficLinks: false,
  showPolicyLinks: true,
  showUnusedPolicies: true,
};

// 混合视图 - 同时显示两者
const mixedView: TopologyView = {
  mode: 'traffic',
  showTrafficLinks: true,
  showPolicyLinks: true,
  showUnusedPolicies: false,
};
```

### 8.2 视图渲染逻辑

```typescript
function renderTopology(data: NetworkGraph, view: TopologyView) {
  const links: GraphLink[] = [];

  if (view.showTrafficLinks) {
    // 添加流量链接 (graphLink)
    links.push(...data.trafficLinks.map(l => ({
      ...l,
      type: 'traffic',
      style: getLinkStyle(l.policy_action),
    })));
  }

  if (view.showPolicyLinks) {
    // 添加策略链接 (policyLink)
    data.policyLinks.forEach(p => {
      // 检查是否已有对应的流量链接
      const hasTraffic = data.trafficLinks.some(
        t => t.from === p.from && t.to === p.to
      );

      if (view.showUnusedPolicies || hasTraffic) {
        links.push({
          ...p,
          type: 'policy',
          style: {
            color: '#1890ff',
            width: 1,
            dashArray: '3,3',
            opacity: hasTraffic ? 0.3 : 0.6,
          },
        });
      }
    });
  }

  // 渲染节点和边
  renderGraph(data.nodes, links);
}
```

## 九、策略模式可视化

### 9.1 策略模式定义

```go
const (
    PolicyModeDiscover = "Discover"  // 发现模式 - 学习流量
    PolicyModeMonitor  = "Monitor"   // 监控模式 - 记录违规
    PolicyModeProtect  = "Protect"   // 保护模式 - 阻断违规
)
```

### 9.2 节点颜色映射

```typescript
// 策略模式 → 节点颜色
const PolicyModeColors = {
  'Protect':  '#52c41a',  // 绿色 - 最严格
  'Monitor':  '#1890ff',  // 蓝色 - 中等
  'Discover': '#faad14',  // 黄色 - 最宽松
};

function getNodeColor(policyMode: string): string {
  return PolicyModeColors[policyMode] || '#999999';
}
```

### 9.3 图例组件

```tsx
const TopologyLegend: React.FC = () => (
  <div className="topology-legend">
    <h4>节点 (策略模式)</h4>
    <div className="legend-item">
      <span className="dot" style={{ backgroundColor: '#52c41a' }} />
      <span>Protect - 保护模式</span>
    </div>
    <div className="legend-item">
      <span className="dot" style={{ backgroundColor: '#1890ff' }} />
      <span>Monitor - 监控模式</span>
    </div>
    <div className="legend-item">
      <span className="dot" style={{ backgroundColor: '#faad14' }} />
      <span>Discover - 发现模式</span>
    </div>

    <h4>边 (策略动作)</h4>
    <div className="legend-item">
      <span className="line allow" />
      <span>Allow - 允许</span>
    </div>
    <div className="legend-item">
      <span className="line deny" />
      <span>Deny - 拒绝</span>
    </div>
    <div className="legend-item">
      <span className="line violate" />
      <span>Violate - 违规</span>
    </div>
    <div className="legend-item">
      <span className="line open" />
      <span>Open - 开放</span>
    </div>
  </div>
);
```

## 十、前端过滤实现

### 10.1 按策略动作过滤

```typescript
type PolicyActionFilter = 'all' | 'allow' | 'deny' | 'violate';

function filterByPolicyAction(
  links: GraphLink[],
  filter: PolicyActionFilter
): GraphLink[] {
  if (filter === 'all') return links;

  const actionMap = {
    'allow': 1,
    'deny': 2,
    'violate': 3,
  };

  return links.filter(l => l.policy_action === actionMap[filter]);
}
```

### 10.2 按策略模式过滤

```typescript
type PolicyModeFilter = 'all' | 'Protect' | 'Monitor' | 'Discover';

function filterByPolicyMode(
  nodes: GraphNode[],
  filter: PolicyModeFilter
): GraphNode[] {
  if (filter === 'all') return nodes;
  return nodes.filter(n => n.policy_mode === filter);
}
```

### 10.3 组合过滤组件

```tsx
interface FilterState {
  policyAction: PolicyActionFilter;
  policyMode: PolicyModeFilter;
  searchTerm: string;
}

const TopologyFilter: React.FC<{
  filter: FilterState;
  onChange: (filter: FilterState) => void;
}> = ({ filter, onChange }) => (
  <Space className="topology-filter">
    <Select
      value={filter.policyAction}
      onChange={v => onChange({ ...filter, policyAction: v })}
      options={[
        { value: 'all', label: '全部动作' },
        { value: 'allow', label: '允许' },
        { value: 'deny', label: '拒绝' },
        { value: 'violate', label: '违规' },
      ]}
    />
    <Select
      value={filter.policyMode}
      onChange={v => onChange({ ...filter, policyMode: v })}
      options={[
        { value: 'all', label: '全部模式' },
        { value: 'Protect', label: '保护模式' },
        { value: 'Monitor', label: '监控模式' },
        { value: 'Discover', label: '发现模式' },
      ]}
    />
    <Input.Search
      placeholder="搜索节点..."
      value={filter.searchTerm}
      onChange={e => onChange({ ...filter, searchTerm: e.target.value })}
    />
  </Space>
);
```

## 十一、完整示例

### 11.1 策略可视化组件

```tsx
import React, { useState, useEffect, useRef } from 'react';
import * as d3 from 'd3';
import { Card, Space, Select, Tag, Tooltip, Modal, Table } from 'antd';

interface PolicyVisualizationProps {
  data: NetworkGraph;
}

export const PolicyVisualization: React.FC<PolicyVisualizationProps> = ({ data }) => {
  const svgRef = useRef<SVGSVGElement>(null);
  const [selectedEdge, setSelectedEdge] = useState<GraphLink | null>(null);
  const [policyDetails, setPolicyDetails] = useState<PolicyRule[] | null>(null);
  const [filter, setFilter] = useState<PolicyActionFilter>('all');

  // 渲染拓扑图
  useEffect(() => {
    if (!svgRef.current || !data) return;

    const svg = d3.select(svgRef.current);
    const width = svgRef.current.clientWidth;
    const height = svgRef.current.clientHeight;

    // 清除现有内容
    svg.selectAll('*').remove();

    // 过滤边
    const filteredLinks = filterByPolicyAction(data.links, filter);

    // 过滤只保留有连接的节点
    const connectedNodeIds = new Set<string>();
    filteredLinks.forEach(l => {
      connectedNodeIds.add(l.from);
      connectedNodeIds.add(l.to);
    });
    const filteredNodes = data.nodes.filter(n => connectedNodeIds.has(n.id));

    // 创建力导向图
    const simulation = d3.forceSimulation(filteredNodes)
      .force('link', d3.forceLink(filteredLinks).id((d: any) => d.id).distance(100))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2));

    // 绘制边
    const link = svg.append('g')
      .selectAll('line')
      .data(filteredLinks)
      .join('line')
      .attr('stroke', d => getLinkColor(d.policy_action))
      .attr('stroke-width', d => d.policy_action >= 2 ? 2 : 1)
      .attr('stroke-dasharray', d => d.policy_action === 3 ? '5,5' : 'none')
      .attr('stroke-opacity', 0.6)
      .style('cursor', 'pointer')
      .on('click', (event, d) => handleEdgeClick(d));

    // 绘制节点
    const node = svg.append('g')
      .selectAll('g')
      .data(filteredNodes)
      .join('g')
      .attr('class', 'node');

    node.append('circle')
      .attr('r', 20)
      .attr('fill', d => getNodeColor(d.policy_mode));

    node.append('text')
      .attr('dy', 35)
      .attr('text-anchor', 'middle')
      .attr('font-size', '12px')
      .text(d => d.name || d.id);

    // 更新位置
    simulation.on('tick', () => {
      link
        .attr('x1', d => (d.source as any).x)
        .attr('y1', d => (d.source as any).y)
        .attr('x2', d => (d.target as any).x)
        .attr('y2', d => (d.target as any).y);

      node.attr('transform', d => `translate(${(d as any).x},${(d as any).y})`);
    });

  }, [data, filter]);

  // 处理边点击
  const handleEdgeClick = async (edge: GraphLink) => {
    setSelectedEdge(edge);

    // 获取关联的策略规则
    const detail = await getConversationDetail(edge.from, edge.to);
    const policyIds = new Set<number>();
    detail.entries.forEach(entry => {
      if (entry.policy_id > 0) {
        policyIds.add(entry.policy_id);
      }
    });

    const policies = await Promise.all(
      Array.from(policyIds).map(id => getPolicyRule(id))
    );
    setPolicyDetails(policies);
  };

  // 策略详情弹窗
  const renderPolicyModal = () => (
    <Modal
      title="关联策略规则"
      open={!!policyDetails}
      onCancel={() => setPolicyDetails(null)}
      footer={null}
      width={800}
    >
      <Table
        dataSource={policyDetails || []}
        columns={[
          { title: 'ID', dataIndex: 'id', width: 80 },
          { title: '源', dataIndex: 'from' },
          { title: '目标', dataIndex: 'to' },
          { title: '端口', dataIndex: 'ports' },
          {
            title: '动作',
            dataIndex: 'action',
            render: (action: string) => (
              <Tag color={action === 'allow' ? 'green' : 'red'}>
                {action.toUpperCase()}
              </Tag>
            ),
          },
        ]}
        pagination={false}
        size="small"
      />
    </Modal>
  );

  return (
    <Card title="策略可视化">
      <Space direction="vertical" style={{ width: '100%' }}>
        <Space>
          <span>过滤:</span>
          <Select
            value={filter}
            onChange={setFilter}
            style={{ width: 120 }}
            options={[
              { value: 'all', label: '全部' },
              { value: 'allow', label: '允许' },
              { value: 'deny', label: '拒绝' },
              { value: 'violate', label: '违规' },
            ]}
          />
        </Space>

        <svg ref={svgRef} width="100%" height={500} />

        <TopologyLegend />
      </Space>

      {renderPolicyModal()}
    </Card>
  );
};
```

## 十二、关键要点

1. **三层链接**: policyLink(规则)、graphLink(流量)、attrLink(属性) 分离存储
2. **策略动作映射**: Open=灰色、Allow=绿色、Deny=红色、Violate=橙色虚线
3. **策略模式映射**: Protect=绿色、Monitor=蓝色、Discover=黄色
4. **策略 ID 关联**: graphEntry.policyID 关联匹配的策略规则
5. **双视图切换**: 支持流量视图和策略视图切换
6. **过滤功能**: 支持按策略动作、策略模式过滤
7. **交互关联**: 点击边可查看关联的策略规则详情
