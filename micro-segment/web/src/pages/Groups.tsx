import React, { useEffect, useState } from 'react'
import { Table, Card, Button, Space, Modal, Form, Input, Select, message, Popconfirm, Tag } from 'antd'
import { PlusOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons'
import { getGroups, createGroup, deleteGroup, Group } from '../api'

const Groups: React.FC = () => {
  const [groups, setGroups] = useState<Group[]>([])
  const [loading, setLoading] = useState(true)
  const [modalVisible, setModalVisible] = useState(false)
  const [detailVisible, setDetailVisible] = useState(false)
  const [selectedGroup, setSelectedGroup] = useState<Group | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    loadGroups()
  }, [])

  const loadGroups = async () => {
    setLoading(true)
    try {
      const data = await getGroups()
      setGroups(data || [])
    } catch (err) {
      message.error('加载组失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    form.resetFields()
    form.setFieldsValue({
      policy_mode: 'Monitor',
    })
    setModalVisible(true)
  }

  const handleDelete = async (name: string) => {
    try {
      await deleteGroup(name)
      message.success('删除成功')
      loadGroups()
    } catch (err) {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      await createGroup(values)
      message.success('创建成功')
      setModalVisible(false)
      loadGroups()
    } catch (err) {
      message.error('操作失败')
    }
  }

  const handleViewDetail = (record: Group) => {
    setSelectedGroup(record)
    setDetailVisible(true)
  }

  const columns = [
    {
      title: '组名',
      dataIndex: 'name',
      key: 'name',
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
      title: '成员数',
      dataIndex: 'members',
      key: 'members',
      render: (members: string[]) => members?.length || 0,
    },
    {
      title: '备注',
      dataIndex: 'comment',
      key: 'comment',
      ellipsis: true,
    },
    {
      title: '操作',
      key: 'actions',
      width: 120,
      render: (_: unknown, record: Group) => (
        <Space>
          <Button type="link" icon={<EyeOutlined />} onClick={() => handleViewDetail(record)} />
          <Popconfirm title="确定删除?" onConfirm={() => handleDelete(record.name)}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title="组管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
          新建组
        </Button>
      }
    >
      <Table
        columns={columns}
        dataSource={groups}
        rowKey="name"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />

      <Modal
        title="新建组"
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        width={500}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="组名" rules={[{ required: true }]}>
            <Input placeholder="例如: web-servers" />
          </Form.Item>
          <Form.Item name="domain" label="域">
            <Input placeholder="例如: default" />
          </Form.Item>
          <Form.Item name="policy_mode" label="策略模式" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'Monitor', label: 'Monitor (监控)' },
                { value: 'Protect', label: 'Protect (防护)' },
              ]}
            />
          </Form.Item>
          <Form.Item name="comment" label="备注">
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="组详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={600}
      >
        {selectedGroup && (
          <div>
            <p><strong>组名:</strong> {selectedGroup.name}</p>
            <p><strong>域:</strong> {selectedGroup.domain || '-'}</p>
            <p><strong>策略模式:</strong> {selectedGroup.policy_mode}</p>
            <p><strong>备注:</strong> {selectedGroup.comment || '-'}</p>
            <p><strong>成员:</strong></p>
            {selectedGroup.members?.length > 0 ? (
              <ul>
                {selectedGroup.members.map((m, i) => (
                  <li key={i}>{m}</li>
                ))}
              </ul>
            ) : (
              <p>无成员</p>
            )}
            <p><strong>匹配条件:</strong></p>
            {selectedGroup.criteria?.length > 0 ? (
              <ul>
                {selectedGroup.criteria.map((c, i) => (
                  <li key={i}>{c.key} {c.op} {c.value}</li>
                ))}
              </ul>
            ) : (
              <p>无匹配条件</p>
            )}
          </div>
        )}
      </Modal>
    </Card>
  )
}

export default Groups
