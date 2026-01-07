import React, { useEffect, useState } from 'react'
import { Row, Col, Card, Statistic, Spin, Alert } from 'antd'
import {
  DesktopOutlined,
  TeamOutlined,
  SafetyOutlined,
  CloudServerOutlined,
  ApiOutlined,
  NodeIndexOutlined,
  LinkOutlined,
} from '@ant-design/icons'
import { getStats, Stats } from '../api'

const Dashboard: React.FC = () => {
  const [stats, setStats] = useState<Stats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    loadStats()
    const interval = setInterval(loadStats, 10000)
    return () => clearInterval(interval)
  }, [])

  const loadStats = async () => {
    try {
      const data = await getStats()
      setStats(data)
      setError(null)
    } catch (err) {
      setError('无法连接到服务器')
    } finally {
      setLoading(false)
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
    <div>
      <h3 style={{ marginBottom: 24 }}>系统概览</h3>
      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="工作负载"
              value={stats?.workloads || 0}
              prefix={<DesktopOutlined />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="组"
              value={stats?.groups || 0}
              prefix={<TeamOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="策略规则"
              value={stats?.policies || 0}
              prefix={<SafetyOutlined />}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="主机"
              value={stats?.hosts || 0}
              prefix={<CloudServerOutlined />}
              valueStyle={{ color: '#fa8c16' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="Agent"
              value={stats?.agents || 0}
              prefix={<ApiOutlined />}
              valueStyle={{ color: '#13c2c2' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="拓扑节点"
              value={stats?.graph_nodes || 0}
              prefix={<NodeIndexOutlined />}
              valueStyle={{ color: '#eb2f96' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} md={8} lg={6}>
          <Card className="stat-card">
            <Statistic
              title="拓扑连接"
              value={stats?.graph_links || 0}
              prefix={<LinkOutlined />}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 24 }}>
        <Col span={24}>
          <Card title="策略模式说明">
            <Row gutter={16}>
              <Col span={12}>
                <Card type="inner" title="Monitor 模式" style={{ borderColor: '#1890ff' }}>
                  <p>监控模式：记录所有违规连接，但不阻断流量。</p>
                  <p>适用于：策略调试阶段，了解网络流量模式。</p>
                </Card>
              </Col>
              <Col span={12}>
                <Card type="inner" title="Protect 模式" style={{ borderColor: '#52c41a' }}>
                  <p>防护模式：实时阻断违规连接，保护网络安全。</p>
                  <p>适用于：生产环境，强制执行安全策略。</p>
                </Card>
              </Col>
            </Row>
          </Card>
        </Col>
      </Row>
    </div>
  )
}

export default Dashboard
