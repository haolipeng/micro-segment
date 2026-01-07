import React, { useEffect, useState } from 'react'
import { Table, Card, Button, Space, Modal, Form, Input, Select, InputNumber, Switch, message, Popconfirm, Tag } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { getPolicies, createPolicy, updatePolicy, deletePolicy, PolicyRule } from '../api'

const Policies: React.FC = () => {
  const [policies, setPolicies] = useState<PolicyRule[]>([])
  const [loading, setLoading] = useState(true)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingPolicy, setEditingPolicy] = useState<PolicyRule | null>(null)
  const [form] = Form.useForm()

  useEffect(() => {
    loadPolicies()
  }, [])

  const loadPolicies = async () => {
    setLoading(true)
    try {
      const data = await getPolicies()
      setPolicies(data || [])
    } catch (err) {
      message.error('加载策略失败')
    } finally {
      setLoading(false)
    }
  }

  const handleCreate = () => {
    setEditingPolicy(null)
    form.resetFields()
    form.setFieldsValue({
      action: 'allow',
      priority: 100,
      disable: false,
    })
    setModalVisible(true)
  }

  const handleEdit = (record: PolicyRule) => {
    setEditingPolicy(record)
    form.setFieldsValue(record)
    setModalVisible(true)
  }

  const handleDelete = async (id: number) => {
    try {
      await deletePolicy(id)
      message.success('删除成功')
      loadPolicies()
    } catch (err) {
      message.error('删除失败')
    }
  }

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields()
      if (editingPolicy) {
        await updatePolicy({ ...editingPolicy, ...values })
        message.success('更新成功')
      } else {
        await createPolicy(values)
        message.success('创建成功')
      }
      setModalVisible(false)
      loadPolicies()
    } catch (err) {
      message.error('操作失败')
    }
  }

  const columns = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
    },
    {
      title: '源组',
      dataIndex: 'from',
      key: 'from',
    },
    {
      title: '目标组',
      dataIndex: 'to',
      key: 'to',
    },
    {
      title: '端口',
      dataIndex: 'ports',
      key: 'ports',
      render: (ports: string) => ports || 'any',
    },
    {
      title: '动作',
      dataIndex: 'action',
      key: 'action',
      render: (action: string) => {
        const colors: Record<string, string> = {
          allow: 'green',
          deny: 'red',
          violate: 'orange',
        }
        return <Tag color={colors[action] || 'default'}>{action}</Tag>
      },
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
    },
    {
      title: '状态',
      dataIndex: 'disable',
      key: 'disable',
      render: (disable: boolean) => (
        <Tag color={disable ? 'default' : 'blue'}>{disable ? '禁用' : '启用'}</Tag>
      ),
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
      render: (_: unknown, record: PolicyRule) => (
        <Space>
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEdit(record)} />
          <Popconfirm title="确定删除?" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" danger icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title="策略管理"
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
          新建策略
        </Button>
      }
    >
      <Table
        columns={columns}
        dataSource={policies}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />

      <Modal
        title={editingPolicy ? '编辑策略' : '新建策略'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => setModalVisible(false)}
        width={600}
      >
        <Form form={form} layout="vertical">
          {!editingPolicy && (
            <Form.Item name="id" label="策略ID" rules={[{ required: true }]}>
              <InputNumber min={1} style={{ width: '100%' }} />
            </Form.Item>
          )}
          <Form.Item name="from" label="源组" rules={[{ required: true }]}>
            <Input placeholder="例如: web-servers 或 any" />
          </Form.Item>
          <Form.Item name="to" label="目标组" rules={[{ required: true }]}>
            <Input placeholder="例如: db-servers 或 any" />
          </Form.Item>
          <Form.Item name="ports" label="端口">
            <Input placeholder="例如: tcp/80,tcp/443 或留空表示any" />
          </Form.Item>
          <Form.Item name="action" label="动作" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'allow', label: '允许 (Allow)' },
                { value: 'deny', label: '拒绝 (Deny)' },
              ]}
            />
          </Form.Item>
          <Form.Item name="priority" label="优先级">
            <InputNumber min={1} max={1000} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="disable" label="禁用" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item name="comment" label="备注">
            <Input.TextArea rows={2} />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}

export default Policies
