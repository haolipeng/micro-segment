import React from 'react'
import { Routes, Route, Link, useLocation } from 'react-router-dom'
import { Layout, Menu } from 'antd'
import {
  DashboardOutlined,
  ClusterOutlined,
  SafetyOutlined,
  TeamOutlined,
  DesktopOutlined,
  NodeIndexOutlined,
} from '@ant-design/icons'

import Dashboard from './pages/Dashboard'
import NetworkGraph from './pages/NetworkGraph'
import Policies from './pages/Policies'
import Groups from './pages/Groups'
import Workloads from './pages/Workloads'
import Connections from './pages/Connections'

const { Header, Sider, Content } = Layout

const App: React.FC = () => {
  const location = useLocation()

  const menuItems = [
    { key: '/', icon: <DashboardOutlined />, label: <Link to="/">仪表盘</Link> },
    { key: '/graph', icon: <ClusterOutlined />, label: <Link to="/graph">网络拓扑</Link> },
    { key: '/policies', icon: <SafetyOutlined />, label: <Link to="/policies">策略管理</Link> },
    { key: '/groups', icon: <TeamOutlined />, label: <Link to="/groups">组管理</Link> },
    { key: '/workloads', icon: <DesktopOutlined />, label: <Link to="/workloads">工作负载</Link> },
    { key: '/connections', icon: <NodeIndexOutlined />, label: <Link to="/connections">连接监控</Link> },
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider theme="dark" width={200}>
        <div className="logo">微隔离</div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[location.pathname]}
          items={menuItems}
        />
      </Sider>
      <Layout>
        <Header style={{ background: '#fff', padding: '0 24px', boxShadow: '0 1px 4px rgba(0,0,0,0.1)' }}>
          <h2 style={{ margin: 0, lineHeight: '64px' }}>微隔离管理平台</h2>
        </Header>
        <Content style={{ margin: '24px', background: '#f0f2f5' }}>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/graph" element={<NetworkGraph />} />
            <Route path="/policies" element={<Policies />} />
            <Route path="/groups" element={<Groups />} />
            <Route path="/workloads" element={<Workloads />} />
            <Route path="/connections" element={<Connections />} />
          </Routes>
        </Content>
      </Layout>
    </Layout>
  )
}

export default App
