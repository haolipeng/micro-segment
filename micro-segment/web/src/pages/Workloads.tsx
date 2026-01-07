import React, { useEffect, useState } from 'react'
import { Table, Card, Button, Tag, Modal, Descriptions, message } from 'antd'
import { ReloadOutlined, EyeOutlined } from '@ant-design/icons'
import { getWorkloads, Workload } from '../api'

const Workloads: React.FC = () => {
  const [workloads, setWorkloads] = useState<Workload[]>([])
  const [loading, setLoading] = useState(true)
  const [detailVisible, setDetailVisible] = useState(false)
  const [selectedWorkload, setSelectedWorkload] = useState<Workload | null>(null)

  useEffect(() => {
    loadWorkloads()
  }, [])

  const loadWorkloads = async () => {
    setLoading(true)
    try {
      const data = await getWorkloads()
      setWorkloads(data || [])
    } catch (err) {
      message.error('加载工作负载失败')
    } finally {
      setLoading(false)
    }
  }

  const handleViewDetail = (record: Workload) => {
    setSelectedWorkload(record)
    setDetailVisible(true)
  }

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      ellipsis: true,
    },
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 120,
      render: (id: string) => id.substring(0, 12),
    },
    {
      title: '主机',
      dataIndex: 'host_name',
      key: 'host_name',
    },
    {
      title: '服务',
      dataIndex: 'service',
      key: 'service',
      render: (service: string) => service || '-',
    },
    {
      title: '域',
      dataIndex: 'domain',
      key: 'domain',
      render: (domain: string) => domain || '-',
    },
    {
      title: '策略模式',
      dataIndex: 'policy_mode',
      key: 'policy_mode',
      render: (mode: string) => {
        const colors: Record<string, string> = {
          Monitor: 'blue',
          Protect: 'green',
        }
        return <Tag color={colors[mode] || 'default'}>{mode}</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'running',
      key: 'running',
      render: (running: boolean) => (
        <Tag color={running ? 'green' : 'default'}>{running ? '运行中' : '已停止'}</Tag>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 80,
      render: (_: unknown, record: Workload) => (
        <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)} />
      ),
    },
  ]

  return (
    <Card
      title="工作负载"
      extra={
        <Button icon={<ReloadOutlined />} onClick={loadWorkloads}>
          刷新
        </Button>
      }
    >
      <Table
        columns={columns}
        dataSource={workloads}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />

      <Modal
        title="工作负载详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={700}
      >
        {selectedWorkload && (
          <Descriptions bordered column={2}>
            <Descriptions.Item label="ID" span={2}>{selectedWorkload.id}</Descriptions.Item>
            <Descriptions.Item label="名称">{selectedWorkload.name}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={selectedWorkload.running ? 'green' : 'default'}>
                {selectedWorkload.running ? '运行中' : '已停止'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="主机ID">{selectedWorkload.host_id}</Descriptions.Item>
            <Descriptions.Item label="主机名">{selectedWorkload.host_name}</Descriptions.Item>
            <Descriptions.Item label="服务">{selectedWorkload.service || '-'}</Descriptions.Item>
            <Descriptions.Item label="域">{selectedWorkload.domain || '-'}</Descriptions.Item>
            <Descriptions.Item label="镜像" span={2}>{selectedWorkload.image || '-'}</Descriptions.Item>
            <Descriptions.Item label="策略模式">
              <Tag color={selectedWorkload.policy_mode === 'Protect' ? 'green' : 'blue'}>
                {selectedWorkload.policy_mode}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="网络接口" span={2}>
              {selectedWorkload.ifaces && Object.keys(selectedWorkload.ifaces).length > 0 ? (
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  {Object.entries(selectedWorkload.ifaces).map(([name, addrs]) => (
                    <li key={name}>
                      <strong>{name}:</strong>{' '}
                      {addrs.map((addr, i) => (
                        <Tag key={i}>{addr.ip}</Tag>
                      ))}
                    </li>
                  ))}
                </ul>
              ) : (
                '-'
              )}
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </Card>
  )
}

export default Workloads
