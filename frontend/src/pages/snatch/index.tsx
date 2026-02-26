import React, { useEffect, useState } from 'react';
import {
  Table, Button, Space, Modal, Form, Input, InputNumber, Tag,
  Select, message, Popconfirm, Switch, Tooltip, Badge,
} from 'antd';
import { PlusOutlined, DeleteOutlined, RobotOutlined, ReloadOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import {
  getSnatchTaskList, createSnatchTask, updateSnatchTask, deleteSnatchTask,
} from '../../api';

interface SnatchTaskItem {
  id: number;
  domainId: number;
  domain: string;
  status: string;
  priority: number;
  targetRegistrar: string;
  autoRegister: number;   // 是否自动注册
  retryCount: number;     // 重试次数
  lastError: string;      // 最后错误信息
  result: string;
  createdAt: string;
  updatedAt: string;
}

const statusMap: Record<string, { color: string; text: string }> = {
  pending: { color: 'orange', text: '待处理' },
  processing: { color: 'blue', text: '处理中' },
  success: { color: 'green', text: '成功' },
  failed: { color: 'red', text: '失败' },
};

const SnatchPage: React.FC = () => {
  const [data, setData] = useState<SnatchTaskItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [status, setStatus] = useState('');
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [form] = Form.useForm();

  const fetchData = async () => {
    setLoading(true);
    try {
      const res: any = await getSnatchTaskList({ page, pageSize, status });
      setData(res.list || []);
      setTotal(res.total || 0);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [page, pageSize, status]);

  const handleCreate = async () => {
    try {
      const values = await form.validateFields();
      await createSnatchTask({
        domainId: 0,
        domain: values.domain,
        priority: values.priority || 0,
        targetRegistrar: values.targetRegistrar || '',
        autoRegister: values.autoRegister ? 1 : 0,
      });
      message.success('创建成功');
      setCreateModalVisible(false);
      form.resetFields();
      fetchData();
    } catch (e: any) {
      if (e?.response?.data) {
        message.error(e.response.data.message || '创建失败');
      }
    }
  };

  const handleUpdateStatus = async (id: number, newStatus: string) => {
    await updateSnatchTask(id, { status: newStatus });
    message.success('更新成功');
    fetchData();
  };

  const handleDelete = async (id: number) => {
    await deleteSnatchTask(id);
    message.success('删除成功');
    fetchData();
  };

  const columns: ColumnsType<SnatchTaskItem> = [
    {
      title: '域名',
      dataIndex: 'domain',
      key: 'domain',
      width: 200,
      render: (text, record) => (
        <Space>
          <span style={{ fontWeight: 500 }}>{text}</span>
          {record.autoRegister === 1 && (
            <Tooltip title="自动抢注已启用">
              <RobotOutlined style={{ color: '#1890ff' }} />
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (s, record) => {
        const info = statusMap[s] || { color: 'default', text: s };
        // 如果有重试次数，显示重试信息
        if (record.retryCount > 0 && s !== 'success') {
          return (
            <Tooltip title={record.lastError || '查看重试详情'}>
              <Badge count={record.retryCount} size="small" offset={[8, 0]}>
                <Tag color={info.color}>{info.text}</Tag>
              </Badge>
            </Tooltip>
          );
        }
        return <Tag color={info.color}>{info.text}</Tag>;
      },
    },
    {
      title: '模式',
      dataIndex: 'autoRegister',
      key: 'autoRegister',
      width: 80,
      render: (val) => (
        <Tag color={val === 1 ? 'blue' : 'default'}>
          {val === 1 ? '自动' : '手动'}
        </Tag>
      ),
    },
    {
      title: '优先级',
      dataIndex: 'priority',
      key: 'priority',
      width: 80,
      sorter: (a, b) => a.priority - b.priority,
    },
    {
      title: '目标注册商',
      dataIndex: 'targetRegistrar',
      key: 'targetRegistrar',
      width: 120,
      render: (text) => text || '-',
    },
    {
      title: '结果/错误',
      dataIndex: 'result',
      key: 'result',
      width: 220,
      ellipsis: true,
      render: (text, record) => {
        if (text) return <span style={{ color: record.status === 'success' ? '#52c41a' : undefined }}>{text}</span>;
        if (record.lastError) return <span style={{ color: '#ff4d4f' }}>{record.lastError}</span>;
        return '-';
      },
    },
    {
      title: '创建时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      width: 160,
    },
    {
      title: '操作',
      key: 'action',
      width: 200,
      render: (_, record) => (
        <Space size="small">
          {record.status === 'pending' && (
            <Button
              type="link"
              size="small"
              onClick={() => handleUpdateStatus(record.id, 'processing')}
            >
              开始处理
            </Button>
          )}
          {record.status === 'processing' && (
            <>
              <Button
                type="link"
                size="small"
                style={{ color: '#52c41a' }}
                onClick={() => handleUpdateStatus(record.id, 'success')}
              >
                标记成功
              </Button>
              <Button
                type="link"
                size="small"
                danger
                onClick={() => handleUpdateStatus(record.id, 'failed')}
              >
                标记失败
              </Button>
            </>
          )}
          {record.status === 'failed' && (
            <Tooltip title="重试抢注">
              <Button
                type="link"
                size="small"
                icon={<ReloadOutlined />}
                onClick={() => handleUpdateStatus(record.id, 'pending')}
              />
            </Tooltip>
          )}
          <Popconfirm title="确定删除?" onConfirm={() => handleDelete(record.id)}>
            <Button type="link" danger size="small" icon={<DeleteOutlined />} />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <h2 style={{ marginBottom: 16, fontWeight: 600 }}>抢注管理</h2>

      <Space style={{ marginBottom: 16 }}>
        <Select
          placeholder="状态筛选"
          value={status || undefined}
          onChange={(val) => { setStatus(val || ''); setPage(1); }}
          allowClear
          style={{ width: 140 }}
          options={[
            { label: '全部', value: '' },
            { label: '待处理', value: 'pending' },
            { label: '处理中', value: 'processing' },
            { label: '成功', value: 'success' },
            { label: '失败', value: 'failed' },
          ]}
        />
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalVisible(true)}>
          创建抢注任务
        </Button>
      </Space>

      <Table
        rowKey="id"
        columns={columns}
        dataSource={data}
        loading={loading}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); },
        }}
        size="middle"
        scroll={{ x: 1100 }}
      />

      <Modal
        title="创建抢注任务"
        open={createModalVisible}
        onOk={handleCreate}
        onCancel={() => { setCreateModalVisible(false); form.resetFields(); }}
        width={480}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="domain"
            label="域名"
            rules={[{ required: true, message: '请输入域名' }]}
          >
            <Input placeholder="例如: example.com" />
          </Form.Item>
          <Form.Item name="priority" label="优先级" initialValue={50}>
            <InputNumber min={0} max={100} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="targetRegistrar" label="目标注册商" initialValue="GoDaddy">
            <Select
              placeholder="选择注册商"
              options={[
                { label: 'GoDaddy', value: 'GoDaddy' },
                { label: '阿里云', value: '阿里云' },
                { label: '腾讯云', value: '腾讯云' },
                { label: 'Namecheap', value: 'Namecheap' },
                { label: '其他', value: '' },
              ]}
            />
          </Form.Item>
          <Form.Item 
            name="autoRegister" 
            label="自动抢注" 
            valuePropName="checked"
            tooltip="启用后，当域名可注册时系统将自动通过GoDaddy API注册（需配置GoDaddy凭证）"
          >
            <Switch checkedChildren="自动" unCheckedChildren="手动" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default SnatchPage;
