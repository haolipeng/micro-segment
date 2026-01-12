# 前端渲染实现

## 一、概述

本文档详细介绍网络拓扑的前端渲染实现，包括 D3.js 力导向图配置、节点和边的样式映射、交互功能实现。

## 二、技术栈

| 技术 | 版本 | 用途 |
|------|------|------|
| D3.js | v7.8.5 | 力导向图渲染 |
| React | v18.2.0 | 组件框架 |
| TypeScript | v5.3.0 | 类型安全 |
| Ant Design | v5.12.0 | UI 组件库 |
| Vite | v5.0.0 | 构建工具 |

## 三、关键源文件

| 文件 | 路径 | 功能 |
|------|------|------|
| 拓扑主组件 | `web/src/pages/NetworkGraph.tsx` | D3 渲染核心 |
| API 定义 | `web/src/api/index.ts` | 数据类型和接口 |
| 样式文件 | `web/src/index.css` | 节点/边样式 |
| 构建配置 | `web/vite.config.ts` | Vite + 代理配置 |

## 四、D3.js 力导向图配置

### 4.1 力模型配置

```typescript
import * as d3 from 'd3';

interface D3Node extends d3.SimulationNodeDatum {
  id: string;
  name: string;
  kind: string;
  domain: string;
  service: string;
  policy_mode: string;
}

interface D3Link extends d3.SimulationLinkDatum<D3Node> {
  from: string;
  to: string;
  bytes: number;
  sessions: number;
  severity: number;
  policy_action: number;
}

// 创建力导向图模拟
const simulation = d3.forceSimulation<D3Node>(nodes)
  // 链接力 - 控制节点间距离
  .force('link', d3.forceLink<D3Node, D3Link>(links)
    .id(d => d.id)
    .distance(100)           // 链接距离 100px
  )
  // 电荷力 - 节点间斥力
  .force('charge', d3.forceManyBody()
    .strength(-300)          // 斥力强度 -300
  )
  // 中心力 - 将节点拉向中心
  .force('center', d3.forceCenter(width / 2, height / 2))
  // 碰撞力 - 防止节点重叠
  .force('collision', d3.forceCollide()
    .radius(40)              // 碰撞半径 40px
  );
```

### 4.2 力参数说明

| 力类型 | 参数 | 值 | 效果 |
|--------|------|-----|------|
| forceLink | distance | 100 | 节点间理想距离 |
| forceManyBody | strength | -300 | 节点斥力（负值表示斥力） |
| forceCenter | x, y | width/2, height/2 | 图的中心点 |
| forceCollide | radius | 40 | 节点碰撞半径 |

### 4.3 模拟参数调优

```typescript
// 模拟控制参数
simulation
  .alpha(1)                    // 初始能量 (0-1)
  .alphaDecay(0.0228)          // 能量衰减率 (默认 0.0228)
  .alphaMin(0.001)             // 最小能量阈值
  .alphaTarget(0)              // 目标能量 (0 = 静止)
  .velocityDecay(0.4);         // 速度衰减 (0-1)

// 重启模拟 (交互后)
simulation.alphaTarget(0.3).restart();

// 停止模拟
simulation.alphaTarget(0);
```

## 五、节点样式

### 5.1 节点颜色映射

```typescript
// 根据策略模式映射颜色
function getNodeColor(policyMode: string): string {
  switch (policyMode) {
    case 'Protect':
      return '#52c41a';   // 绿色 - 防护模式
    case 'Monitor':
      return '#1890ff';   // 蓝色 - 监控模式
    case 'Discover':
      return '#faad14';   // 黄色 - 发现模式
    default:
      return '#999999';   // 灰色 - 未知
  }
}

// 应用节点样式
node.append('circle')
  .attr('r', 20)                                    // 半径 20px
  .attr('fill', d => getNodeColor(d.policy_mode))   // 填充色
  .attr('stroke', '#fff')                           // 边框白色
  .attr('stroke-width', 2);                         // 边框宽度
```

### 5.2 节点图标

```typescript
// 根据节点类型显示不同图标
function getNodeIcon(kind: string): string {
  switch (kind) {
    case 'container':
      return '\uf13d';    // Docker 图标 (FontAwesome)
    case 'host':
      return '\uf233';    // 服务器图标
    case 'external':
      return '\uf0ac';    // 地球图标
    case 'address':
      return '\uf0e8';    // 网络图标
    default:
      return '\uf111';    // 圆形默认图标
  }
}

// 添加图标 (使用 FontAwesome)
node.append('text')
  .attr('class', 'node-icon')
  .attr('font-family', 'FontAwesome')
  .attr('font-size', '16px')
  .attr('fill', '#fff')
  .attr('text-anchor', 'middle')
  .attr('dominant-baseline', 'central')
  .text(d => getNodeIcon(d.kind));
```

### 5.3 节点标签

```typescript
// 添加节点标签
node.append('text')
  .attr('class', 'node-label')
  .attr('dy', 35)                    // 标签在节点下方
  .attr('text-anchor', 'middle')
  .attr('font-size', '12px')
  .attr('fill', '#333')
  .text(d => d.name || d.id)
  .each(function(d) {
    // 文本截断 (最多 15 字符)
    const text = d3.select(this);
    const name = d.name || d.id;
    if (name.length > 15) {
      text.text(name.substring(0, 12) + '...');
    }
  });
```

### 5.4 CSS 样式

```css
/* 节点容器 */
.node {
  cursor: pointer;
}

/* 节点圆形 */
.node circle {
  stroke: #fff;
  stroke-width: 2px;
  transition: all 0.2s ease;
}

/* 节点悬停效果 */
.node:hover circle {
  stroke-width: 4px;
  filter: brightness(1.1);
}

/* 节点选中效果 */
.node.selected circle {
  stroke: #1890ff;
  stroke-width: 4px;
}

/* 节点标签 */
.node-label {
  font-size: 12px;
  fill: #333;
  pointer-events: none;
  user-select: none;
}

/* 节点图标 */
.node-icon {
  pointer-events: none;
}
```

## 六、边样式

### 6.1 边颜色映射

```typescript
// 根据策略动作映射颜色
function getLinkColor(policyAction: number): string {
  switch (policyAction) {
    case 0:  // Open
      return '#999999';   // 灰色
    case 1:  // Allow
      return '#52c41a';   // 绿色
    case 2:  // Deny
      return '#ff4d4f';   // 红色
    case 3:  // Violate
      return '#ff7a45';   // 橙色
    default:
      return '#999999';
  }
}

// 应用边样式
link.attr('stroke', d => getLinkColor(d.policy_action))
    .attr('stroke-opacity', 0.6);
```

### 6.2 边宽度映射

```typescript
// 根据会话数映射边宽度 (对数缩放)
function getLinkWidth(sessions: number): number {
  return Math.max(1, Math.log(sessions + 1));
}

// 应用边宽度
link.attr('stroke-width', d => getLinkWidth(d.sessions));
```

### 6.3 边样式变体

```typescript
// 虚线样式 (用于违规连接)
function getLinkDashArray(policyAction: number): string {
  if (policyAction === 3) {  // Violate
    return '5,5';            // 虚线
  }
  return 'none';             // 实线
}

link.attr('stroke-dasharray', d => getLinkDashArray(d.policy_action));
```

### 6.4 边箭头

```typescript
// 定义箭头标记
svg.append('defs').append('marker')
  .attr('id', 'arrowhead')
  .attr('viewBox', '-0 -5 10 10')
  .attr('refX', 25)                    // 箭头位置 (考虑节点半径)
  .attr('refY', 0)
  .attr('orient', 'auto')
  .attr('markerWidth', 6)
  .attr('markerHeight', 6)
  .append('path')
  .attr('d', 'M 0,-5 L 10,0 L 0,5')
  .attr('fill', '#999');

// 应用箭头
link.attr('marker-end', 'url(#arrowhead)');
```

### 6.5 CSS 样式

```css
/* 连接线基础样式 */
.link {
  stroke: #999;
  stroke-opacity: 0.6;
  fill: none;
}

/* 违规连接 */
.link.violate {
  stroke: #ff7a45;
  stroke-width: 2px;
  stroke-dasharray: 5, 5;
}

/* 拒绝连接 */
.link.deny {
  stroke: #ff4d4f;
  stroke-width: 2px;
}

/* 允许连接 */
.link.allow {
  stroke: #52c41a;
}

/* 悬停高亮 */
.link:hover {
  stroke-opacity: 1;
  stroke-width: 3px;
}
```

## 七、交互功能

### 7.1 缩放和平移

```typescript
// 创建缩放行为
const zoom = d3.zoom<SVGSVGElement, unknown>()
  .scaleExtent([0.1, 4])              // 缩放范围 0.1x - 4x
  .on('zoom', (event) => {
    g.attr('transform', event.transform);
  });

// 应用缩放
svg.call(zoom);

// 重置视图
function resetView() {
  svg.transition()
    .duration(750)
    .call(zoom.transform, d3.zoomIdentity);
}

// 缩放到适应视口
function fitView() {
  const bounds = g.node()?.getBBox();
  if (!bounds) return;

  const fullWidth = width;
  const fullHeight = height;
  const midX = bounds.x + bounds.width / 2;
  const midY = bounds.y + bounds.height / 2;

  const scale = 0.8 / Math.max(
    bounds.width / fullWidth,
    bounds.height / fullHeight
  );

  const translate = [
    fullWidth / 2 - scale * midX,
    fullHeight / 2 - scale * midY
  ];

  svg.transition()
    .duration(750)
    .call(
      zoom.transform,
      d3.zoomIdentity.translate(translate[0], translate[1]).scale(scale)
    );
}
```

### 7.2 节点拖拽

```typescript
// 创建拖拽行为
const drag = d3.drag<SVGGElement, D3Node>()
  .on('start', (event, d) => {
    // 拖拽开始：重启模拟
    if (!event.active) {
      simulation.alphaTarget(0.3).restart();
    }
    // 固定节点位置
    d.fx = d.x;
    d.fy = d.y;
  })
  .on('drag', (event, d) => {
    // 拖拽中：更新位置
    d.fx = event.x;
    d.fy = event.y;
  })
  .on('end', (event, d) => {
    // 拖拽结束：释放节点
    if (!event.active) {
      simulation.alphaTarget(0);
    }
    // 可选：释放固定位置
    d.fx = null;
    d.fy = null;
  });

// 应用拖拽
node.call(drag);
```

### 7.3 悬停提示

```typescript
// 创建提示框
const tooltip = d3.select('body')
  .append('div')
  .attr('class', 'tooltip')
  .style('opacity', 0);

// 节点悬停
node
  .on('mouseover', (event, d) => {
    tooltip.transition()
      .duration(200)
      .style('opacity', 1);

    tooltip.html(`
      <strong>${d.name || d.id}</strong><br/>
      类型: ${d.kind}<br/>
      服务: ${d.service || '-'}<br/>
      域: ${d.domain || '-'}<br/>
      模式: ${d.policy_mode || '-'}
    `)
    .style('left', (event.pageX + 10) + 'px')
    .style('top', (event.pageY - 10) + 'px');
  })
  .on('mouseout', () => {
    tooltip.transition()
      .duration(500)
      .style('opacity', 0);
  });

// 边悬停
link
  .on('mouseover', (event, d) => {
    tooltip.transition()
      .duration(200)
      .style('opacity', 1);

    tooltip.html(`
      <strong>${d.from} → ${d.to}</strong><br/>
      流量: ${formatBytes(d.bytes)}<br/>
      会话: ${d.sessions}<br/>
      动作: ${getPolicyActionName(d.policy_action)}
    `)
    .style('left', (event.pageX + 10) + 'px')
    .style('top', (event.pageY - 10) + 'px');
  })
  .on('mouseout', () => {
    tooltip.transition()
      .duration(500)
      .style('opacity', 0);
  });
```

### 7.4 节点点击

```typescript
// 节点点击事件
node.on('click', (event, d) => {
  event.stopPropagation();

  // 移除其他选中状态
  node.classed('selected', false);

  // 选中当前节点
  d3.select(event.currentTarget).classed('selected', true);

  // 高亮相关边
  link.classed('highlighted', l =>
    l.source.id === d.id || l.target.id === d.id
  );

  // 触发详情面板
  onNodeSelect?.(d);
});

// 背景点击取消选中
svg.on('click', () => {
  node.classed('selected', false);
  link.classed('highlighted', false);
  onNodeSelect?.(null);
});
```

## 八、过滤功能

### 8.1 策略动作过滤

```typescript
type FilterType = 'all' | 'violate' | 'deny';

function filterGraph(data: NetworkGraph, filter: FilterType) {
  let filteredLinks = data.links;

  if (filter === 'violate') {
    filteredLinks = data.links.filter(l => l.policy_action === 3);
  } else if (filter === 'deny') {
    filteredLinks = data.links.filter(l => l.policy_action === 2);
  }

  // 过滤只保留有连接的节点
  const connectedNodeIds = new Set<string>();
  filteredLinks.forEach(l => {
    connectedNodeIds.add(l.from);
    connectedNodeIds.add(l.to);
  });

  const filteredNodes = data.nodes.filter(n =>
    connectedNodeIds.has(n.id)
  );

  return { nodes: filteredNodes, links: filteredLinks };
}
```

### 8.2 搜索过滤

```typescript
function searchNodes(nodes: D3Node[], searchTerm: string): D3Node[] {
  if (!searchTerm) return nodes;

  const term = searchTerm.toLowerCase();
  return nodes.filter(n =>
    n.id.toLowerCase().includes(term) ||
    n.name?.toLowerCase().includes(term) ||
    n.service?.toLowerCase().includes(term) ||
    n.domain?.toLowerCase().includes(term)
  );
}
```

### 8.3 React 组件示例

```tsx
import React, { useState, useEffect, useRef } from 'react';
import { Select, Input, Button, Space } from 'antd';
import * as d3 from 'd3';
import { getNetworkGraph, NetworkGraph } from '../api';

export const NetworkTopology: React.FC = () => {
  const svgRef = useRef<SVGSVGElement>(null);
  const [data, setData] = useState<NetworkGraph | null>(null);
  const [filter, setFilter] = useState<'all' | 'violate' | 'deny'>('all');
  const [search, setSearch] = useState('');

  // 加载数据
  useEffect(() => {
    getNetworkGraph().then(setData);
  }, []);

  // 渲染图
  useEffect(() => {
    if (!data || !svgRef.current) return;

    // 应用过滤
    const filtered = filterGraph(data, filter);
    const searched = {
      ...filtered,
      nodes: searchNodes(filtered.nodes, search)
    };

    // 渲染 D3 图 (如上所述)
    renderGraph(svgRef.current, searched);
  }, [data, filter, search]);

  return (
    <div className="network-topology">
      <div className="toolbar">
        <Space>
          <Select
            value={filter}
            onChange={setFilter}
            options={[
              { value: 'all', label: '全部' },
              { value: 'violate', label: '违规' },
              { value: 'deny', label: '拒绝' }
            ]}
          />
          <Input.Search
            placeholder="搜索节点..."
            value={search}
            onChange={e => setSearch(e.target.value)}
            style={{ width: 200 }}
          />
          <Button onClick={() => fitView()}>适应视口</Button>
          <Button onClick={() => resetView()}>重置视图</Button>
        </Space>
      </div>
      <svg ref={svgRef} className="graph-container" />
    </div>
  );
};
```

## 九、完整渲染函数

```typescript
function renderGraph(
  svgElement: SVGSVGElement,
  data: { nodes: D3Node[]; links: D3Link[] }
) {
  const width = svgElement.clientWidth;
  const height = svgElement.clientHeight;

  // 清除现有内容
  d3.select(svgElement).selectAll('*').remove();

  const svg = d3.select(svgElement)
    .attr('viewBox', [0, 0, width, height]);

  // 创建容器组 (用于缩放)
  const g = svg.append('g');

  // 设置缩放
  const zoom = d3.zoom<SVGSVGElement, unknown>()
    .scaleExtent([0.1, 4])
    .on('zoom', (event) => g.attr('transform', event.transform));

  svg.call(zoom);

  // 创建模拟
  const simulation = d3.forceSimulation(data.nodes)
    .force('link', d3.forceLink(data.links).id((d: any) => d.id).distance(100))
    .force('charge', d3.forceManyBody().strength(-300))
    .force('center', d3.forceCenter(width / 2, height / 2))
    .force('collision', d3.forceCollide().radius(40));

  // 绘制边
  const link = g.append('g')
    .attr('class', 'links')
    .selectAll('line')
    .data(data.links)
    .join('line')
    .attr('class', d => `link ${getPolicyActionClass(d.policy_action)}`)
    .attr('stroke', d => getLinkColor(d.policy_action))
    .attr('stroke-width', d => getLinkWidth(d.sessions))
    .attr('stroke-dasharray', d => getLinkDashArray(d.policy_action));

  // 绘制节点
  const node = g.append('g')
    .attr('class', 'nodes')
    .selectAll('g')
    .data(data.nodes)
    .join('g')
    .attr('class', 'node')
    .call(d3.drag<SVGGElement, D3Node>()
      .on('start', dragstarted)
      .on('drag', dragged)
      .on('end', dragended)
    );

  // 节点圆形
  node.append('circle')
    .attr('r', 20)
    .attr('fill', d => getNodeColor(d.policy_mode));

  // 节点标签
  node.append('text')
    .attr('dy', 35)
    .attr('text-anchor', 'middle')
    .attr('font-size', '12px')
    .text(d => d.name || d.id);

  // 更新位置
  simulation.on('tick', () => {
    link
      .attr('x1', d => (d.source as D3Node).x!)
      .attr('y1', d => (d.source as D3Node).y!)
      .attr('x2', d => (d.target as D3Node).x!)
      .attr('y2', d => (d.target as D3Node).y!);

    node.attr('transform', d => `translate(${d.x},${d.y})`);
  });

  // 拖拽函数
  function dragstarted(event: d3.D3DragEvent<SVGGElement, D3Node, D3Node>, d: D3Node) {
    if (!event.active) simulation.alphaTarget(0.3).restart();
    d.fx = d.x;
    d.fy = d.y;
  }

  function dragged(event: d3.D3DragEvent<SVGGElement, D3Node, D3Node>, d: D3Node) {
    d.fx = event.x;
    d.fy = event.y;
  }

  function dragended(event: d3.D3DragEvent<SVGGElement, D3Node, D3Node>, d: D3Node) {
    if (!event.active) simulation.alphaTarget(0);
    d.fx = null;
    d.fy = null;
  }
}
```

## 十、关键要点

1. **力导向布局**: 使用 D3.js 的多力模型实现自动布局
2. **样式映射**: 节点颜色映射策略模式，边颜色映射策略动作
3. **流量可视化**: 边宽度使用对数缩放映射会话数
4. **交互丰富**: 支持缩放、平移、拖拽、悬停提示
5. **过滤功能**: 支持按策略动作和搜索词过滤
6. **性能优化**: 使用 SVG 组进行统一变换，减少 DOM 操作
