import React, { useEffect, useRef, useState } from 'react'
import { Card, Spin, Alert, Button, Space, Select, Tag } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import * as d3 from 'd3'
import { getNetworkGraph, NetworkGraph as NetworkGraphData, GraphNode, GraphLink } from '../api'

interface D3Node extends GraphNode {
  x?: number
  y?: number
  fx?: number | null
  fy?: number | null
}

interface D3Link extends GraphLink {
  source: D3Node | string
  target: D3Node | string
}

const NetworkGraph: React.FC = () => {
  const svgRef = useRef<SVGSVGElement>(null)
  const [data, setData] = useState<NetworkGraphData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [filter, setFilter] = useState<string>('all')

  useEffect(() => {
    loadData()
  }, [])

  useEffect(() => {
    if (data && svgRef.current) {
      renderGraph()
    }
  }, [data, filter])

  const loadData = async () => {
    setLoading(true)
    try {
      const graphData = await getNetworkGraph()
      setData(graphData)
      setError(null)
    } catch (err) {
      setError('加载网络拓扑失败')
    } finally {
      setLoading(false)
    }
  }

  const renderGraph = () => {
    if (!svgRef.current || !data) return

    const svg = d3.select(svgRef.current)
    svg.selectAll('*').remove()

    const width = svgRef.current.clientWidth || 800
    const height = svgRef.current.clientHeight || 600

    // 过滤数据
    let filteredLinks = data.links
    if (filter === 'violate') {
      filteredLinks = data.links.filter(l => l.policy_action === 3)
    } else if (filter === 'deny') {
      filteredLinks = data.links.filter(l => l.policy_action === 2)
    }

    const nodeIds = new Set<string>()
    filteredLinks.forEach(l => {
      nodeIds.add(l.from)
      nodeIds.add(l.to)
    })
    const filteredNodes = filter === 'all' 
      ? data.nodes 
      : data.nodes.filter(n => nodeIds.has(n.id))

    // 准备D3数据
    const nodes: D3Node[] = filteredNodes.map(n => ({ ...n }))
    const links: D3Link[] = filteredLinks.map(l => ({
      ...l,
      source: l.from,
      target: l.to,
    }))

    // 创建缩放行为
    const zoom = d3.zoom<SVGSVGElement, unknown>()
      .scaleExtent([0.1, 4])
      .on('zoom', (event) => {
        g.attr('transform', event.transform)
      })

    svg.call(zoom)

    const g = svg.append('g')

    // 创建力导向图
    const simulation = d3.forceSimulation(nodes)
      .force('link', d3.forceLink<D3Node, D3Link>(links).id(d => d.id).distance(100))
      .force('charge', d3.forceManyBody().strength(-300))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide().radius(40))

    // 绘制连接线
    const link = g.append('g')
      .selectAll('line')
      .data(links)
      .join('line')
      .attr('class', d => {
        if (d.policy_action === 3) return 'link violate'
        if (d.policy_action === 2) return 'link deny'
        return 'link'
      })
      .attr('stroke-width', d => Math.max(1, Math.log(d.sessions + 1)))

    // 绘制节点
    const node = g.append('g')
      .selectAll('g')
      .data(nodes)
      .join('g')
      .attr('class', 'node')
      .call(d3.drag<SVGGElement, D3Node>()
        .on('start', (event, d) => {
          if (!event.active) simulation.alphaTarget(0.3).restart()
          d.fx = d.x
          d.fy = d.y
        })
        .on('drag', (event, d) => {
          d.fx = event.x
          d.fy = event.y
        })
        .on('end', (event, d) => {
          if (!event.active) simulation.alphaTarget(0)
          d.fx = null
          d.fy = null
        }) as any)

    // 节点圆形
    node.append('circle')
      .attr('r', 20)
      .attr('fill', d => {
        if (d.policy_mode === 'Protect') return '#52c41a'
        if (d.policy_mode === 'Monitor') return '#1890ff'
        return '#999'
      })

    // 节点标签
    node.append('text')
      .attr('dy', 35)
      .attr('text-anchor', 'middle')
      .text(d => d.name || d.id.substring(0, 8))

    // 创建提示框
    const tooltip = d3.select('body').append('div')
      .attr('class', 'tooltip')
      .style('opacity', 0)

    node.on('mouseover', (event, d) => {
      tooltip.transition().duration(200).style('opacity', 1)
      tooltip.html(`
        <strong>${d.name || d.id}</strong><br/>
        类型: ${d.kind}<br/>
        服务: ${d.service || '-'}<br/>
        域: ${d.domain || '-'}<br/>
        模式: ${d.policy_mode || '-'}
      `)
        .style('left', (event.pageX + 10) + 'px')
        .style('top', (event.pageY - 10) + 'px')
    })
      .on('mouseout', () => {
        tooltip.transition().duration(500).style('opacity', 0)
      })

    // 更新位置
    simulation.on('tick', () => {
      link
        .attr('x1', d => (d.source as D3Node).x!)
        .attr('y1', d => (d.source as D3Node).y!)
        .attr('x2', d => (d.target as D3Node).x!)
        .attr('y2', d => (d.target as D3Node).y!)

      node.attr('transform', d => `translate(${d.x},${d.y})`)
    })

    return () => {
      tooltip.remove()
      simulation.stop()
    }
  }

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (error) {
    return <Alert message="错误" description={error} type="error" showIcon />
  }

  return (
    <Card
      title="网络拓扑图"
      extra={
        <Space>
          <Select
            value={filter}
            onChange={setFilter}
            style={{ width: 120 }}
            options={[
              { value: 'all', label: '全部' },
              { value: 'violate', label: '违规连接' },
              { value: 'deny', label: '拒绝连接' },
            ]}
          />
          <Button icon={<ReloadOutlined />} onClick={loadData}>刷新</Button>
        </Space>
      }
    >
      <div style={{ marginBottom: 16 }}>
        <Space>
          <Tag color="blue">Monitor模式</Tag>
          <Tag color="green">Protect模式</Tag>
          <Tag color="red">违规连接</Tag>
          <Tag color="orange">拒绝连接</Tag>
        </Space>
      </div>
      <div className="network-graph">
        <svg ref={svgRef} />
      </div>
    </Card>
  )
}

export default NetworkGraph
