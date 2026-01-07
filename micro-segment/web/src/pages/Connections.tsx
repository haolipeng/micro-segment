import React, { useEffect, useState } from 'react'
import { Table, Card, Button, Tag, Select, Space, message } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { getNetworkGraph, GraphLink } from '../api'

const Connections: React.FC = () => {
  const [connections, setConnections] = useState<GraphLink[]>([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<string>('all')

  useEffect(() => {
    loadConnections()
    const interval = setInterval(loadConnections, 10000)
    return () => clearInterval(interval)
  }, [])

  const loadConnections = async () => {
    setLoading(true)
    try {
      const data = await getNetworkGraph()
      setConnections(data?.links || [])
    } catch (err) {
      message.error('加载连接失败')
    } finally {
      setLoading(false)
    }
  }

  const filteredConnections = connections.filter(conn => {
    if (filter === 'all') return true
    if (filter === 'violate') return conn.policy_action === 3
    if (filter === 'deny') return conn.policy_action === 2
    if (filter === 'allow') return conn.policy_action === 1
    return true
  })

  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  const getActionTag = (action: number) => {
    switch (action) {
      case 0:
        return <Tag color="default">Open</Tag>
      case 1:
        return <Tag color="green">Allow</Tag>
      case 2:
        return <Tag color="red">Deny</Tag>
      case 3:
        return <Tag color="orange">Violate</Tag>
      default:
        return <Tag>{action}</Tag>
    }
  }

  const getSeverityTag = (severity: number) => {
    if (severity === 0) return null
    const colors = ['', 'blue', 'gold', 'orange', 'red']
    const labels = ['', 'Low', 'Medium', 'High', 'Critical']
    return <Tag color={colors[severity] || 'default'}>{labels[severity] || severity}</Tag>
  }

  const columns = [
    {
      title: '源工作负载',
      dataIndex: 'from',
      key: 'from',
      ellipsis: true,
      render: (id: string) => id.substring(0, 12) + '...',
    },
    {
      title: '目标工作负载',
      dataIndex: 'to',
      key: 'to',
      ellipsis: true,
      render: (id: string) => id.substring(0, 12) + '...',
    },
    {
      title: '流量',
      dataIndex: 'bytes',
      key: 'bytes',
      render: (bytes: number) => formatBytes(bytes),
      sorter: (a: GraphLink, b: GraphLink) => a.bytes - b.bytes,
    },
    {
      title: '会话数',
      dataIndex: 'sessions',
      key: 'sessions',
      sorter: (a: GraphLink, b: GraphLink) => a.sessions - b.sessions,
    },
    {
      title: '策略动作',
      dataIndex: 'policy_action',
      key: 'policy_action',
      render: (action: number) => getActionTag(action),
      filters: [
        { text: 'Open', value: 0 },
        { text: 'Allow', value: 1 },
        { text: 'Deny', value: 2 },
        { text: 'Violate', value: 3 },
      ],
      onFilter: (value: unknown, record: GraphLink) => record.policy_action === value,
    },
    {
      title: '威胁等级',
      dataIndex: 'severity',
      key: 'severity',
      render: (severity: number) => getSeverityTag(severity),
    },
  ]

  return (
    <Card
      title="连接监控"
      extra={
        <Space>
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
          <Button icon={<ReloadOutlined />} onClick={loadConnections}>
            刷新
          </Button>
        </Space>
      }
    >
      <div style={{ marginBottom: 16 }}>
        <Space>
          <span>总连接数: {connections.length}</span>
          <span>|</span>
          <span>违规: {connections.filter(c => c.policy_action === 3).length}</span>
          <span>|</span>
          <span>拒绝: {connections.filter(c => c.policy_action === 2).length}</span>
        </Space>
      </div>
      <Table
        columns={columns}
        dataSource={filteredConnections}
        rowKey={(record) => `${record.from}-${record.to}`}
        loading={loading}
        pagination={{ pageSize: 20 }}
      />
    </Card>
  )
}

export default Connections
