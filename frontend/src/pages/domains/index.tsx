import React, { useEffect, useState, useRef } from 'react';
import {
  Table, Button, Input, Select, Space, Modal, Form, Tag, Switch,
  Upload, message, Popconfirm, Tooltip, DatePicker, Radio,
} from 'antd';
import {
  PlusOutlined, UploadOutlined, ReloadOutlined,
  DeleteOutlined, SearchOutlined, RocketOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import {
  getDomainList, addDomain, deleteDomain, toggleMonitor,
  checkDomains, importDomains, createSnatchTask,
} from '../../api';

interface LifecycleStagesItem {
  expiryDate: string;        // 到期日期
  gracePeriodEnd: string;    // 宽限期结束
  redemptionStart: string;   // 赎回期开始
  redemptionEnd: string;     // 赎回期结束
  pendingDeleteStart: string; // 待删除期开始
  pendingDeleteEnd: string;   // 待删除期结束
  availableDate: string;      // 可注册时间
}

interface DomainItem {
  id: number;
  domain: string;
  status: string;
  expiryDate: string;
  creationDate: string;
  deleteDate: string;      // 预计删除日期
  registrar: string;
  whoisStatus: string;     // WHOIS Domain Status 原始值
  monitor: number;
  lastChecked: string;
  createdAt: string;
  lifecycleStages?: LifecycleStagesItem; // 生命周期各阶段时间
}

const statusMap: Record<string, { color: string; text: string; priority?: number }> = {
  available: { color: 'green', text: '可注册', priority: 5 },
  pending_delete: { color: 'magenta', text: '待删除', priority: 4 },
  redemption: { color: 'volcano', text: '赎回期', priority: 3 },
  grace_period: { color: 'gold', text: '宽限期', priority: 2 },
  registered: { color: 'blue', text: '已注册', priority: 1 },
  restricted: { color: 'default', text: '限制注册', priority: 0 },
  expired: { color: 'orange', text: '已过期', priority: 0 },
  unknown: { color: 'default', text: '未知', priority: 0 },
};

const DomainsPage: React.FC = () => {
  const [data, setData] = useState<DomainItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [status, setStatus] = useState('');
  const [keyword, setKeyword] = useState('');
  const [expiryStart, setExpiryStart] = useState('');
  const [expiryEnd, setExpiryEnd] = useState('');
  const [sortField, setSortField] = useState('expiry_date');
  const [sortOrder, setSortOrder] = useState('asc');
  const [addModalVisible, setAddModalVisible] = useState(false);
  const [selectedRowKeys, setSelectedRowKeys] = useState<number[]>([]);
  const [checkLoading, setCheckLoading] = useState(false);
  const [checkProgress, setCheckProgress] = useState<{ current: number; total: number }>({ current: 0, total: 0 });
  const [form] = Form.useForm();

  const fetchData = async () => {
    setLoading(true);
    try {
      const res: any = await getDomainList({ 
        page, 
        pageSize, 
        status, 
        keyword, 
        monitor: -1,
        expiryStart,
        expiryEnd,
        sortField,
        sortOrder,
      });
      
      // 后端已经按状态优先级排序，直接使用返回的数据
      setData(res.list || []);
      setTotal(res.total || 0);
    } catch (e) {
      // error handled by interceptor
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [page, pageSize, status, sortField, sortOrder]);

  const handleSearch = () => {
    setPage(1);
    fetchData();
  };

  const handleAdd = async () => {
    try {
      const values = await form.validateFields();
      await addDomain(values);
      message.success('添加成功');
      setAddModalVisible(false);
      form.resetFields();
      fetchData();
    } catch (e: any) {
      if (e?.response?.data) {
        message.error(e.response.data.message || '添加失败');
      }
    }
  };

  const handleDelete = async (id: number) => {
    await deleteDomain(id);
    message.success('删除成功');

    // 先在当前页面删除这一行
    const newData = data.filter((item) => item.id !== id);
    const newTotal = total - 1;
    setData(newData);
    setTotal(newTotal);

    // 如果后面还有数据（非最后一页），从下一页拉一条补到当前页尾部
    const hasMore = page * pageSize < newTotal;
    if (hasMore) {
      try {
        const res: any = await getDomainList({
          page: page + 1,
          pageSize: 1,
          status,
          keyword,
          monitor: -1,
          expiryStart,
          expiryEnd,
          sortField,
          sortOrder,
        });
        if (res?.list && Array.isArray(res.list) && res.list.length > 0) {
          const extra = res.list[0] as DomainItem;
          setData((prev) => [...prev, extra]);
        }
      } catch {
        // 补数据失败时不影响当前页展示
      }
    }
  };

  const handleToggleMonitor = async (id: number, checked: boolean) => {
    await toggleMonitor(id, checked ? 1 : 0);
    message.success(checked ? '已开启监控' : '已关闭监控');
    fetchData();
  };

  const handleCheck = async () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请选择要检查的域名');
      return;
    }
    setCheckLoading(true);
    try {
      const ids = [...selectedRowKeys];
      const total = ids.length;
      setCheckProgress({ current: 0, total });

      let successCount = 0;

      for (let i = 0; i < ids.length; i += 1) {
        const id = ids[i];
        setCheckProgress({ current: i + 1, total });

        try {
          // 后端一次支持多条，这里每次只传一个，方便做到“处理一条，刷新一条”
          const res: any = await checkDomains({ ids: [id] });
          if (res?.count) {
            successCount += res.count;
          }

          // 使用后端返回的最新域名信息，在当前页面局部更新对应行
          if (res?.list && Array.isArray(res.list) && res.list.length > 0) {
            const updated = res.list[0] as DomainItem;
            setData((prev) =>
              prev.map((item) => (item.id === updated.id ? { ...item, ...updated } : item)),
            );
          }
        } catch {
          // 单条失败直接跳过，继续后面的
        }

        // 不整体刷新列表，避免页面跳动 / 分页变化
      }

      message.success(`成功检查 ${successCount} 个域名`);
      setCheckProgress({ current: 0, total: 0 });
    } finally {
      setCheckLoading(false);
    }
  };

  const handleImport = async (file: File) => {
    try {
      const res: any = await importDomains(file);
      message.success(`导入完成: 总计 ${res.total}, 成功 ${res.success}, 失败 ${res.failed}`);
      fetchData();
    } catch (e) {
      message.error('导入失败');
    }
    return false;
  };

  // 快速创建抢注任务（针对可注册或即将到期的域名）
  const handleCreateSnatchTask = async (record: DomainItem) => {
    try {
      // 根据状态设置优先级：pending_delete > available > redemption > expired
      let priority = 50;
      if (record.status === 'pending_delete') priority = 100;
      else if (record.status === 'available') priority = 90;
      else if (record.status === 'redemption') priority = 70;
      else if (record.status === 'expired') priority = 60;

      await createSnatchTask({
        domainId: record.id,
        domain: record.domain,
        priority,
        targetRegistrar: 'GoDaddy',
        autoRegister: record.status === 'pending_delete' ? 1 : 0, // pending_delete默认开启自动抢注
      });
      message.success(`已创建抢注任务: ${record.domain}${record.status === 'pending_delete' ? '（自动抢注已启用）' : ''}`);
    } catch (e: any) {
      if (e?.response?.data) {
        message.error(e.response.data.message || '创建失败');
      }
    }
  };

  const columns: ColumnsType<DomainItem> = [
    {
      title: '域名',
      dataIndex: 'domain',
      key: 'domain',
      width: 200,
      render: (text) => <span style={{ fontWeight: 500 }}>{text}</span>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (s, record) => {
        const info = statusMap[s] || { color: 'default', text: s };
        return (
          <Tooltip title={record.whoisStatus || '无详细状态'}>
            <Tag color={info.color}>{info.text}</Tag>
          </Tooltip>
        );
      },
    },
    {
      title: '到期日期',
      dataIndex: 'expiryDate',
      key: 'expiryDate',
      width: 140,
      render: (text, record) => {
        if (!text) {
          return '-';
        }
        
        // 根据状态显示当前阶段的截止时间（包含日期和时间）
        const stages = record.lifecycleStages;
        const now = dayjs();
        
        if (!stages) {
          // 已注册状态，显示到期日期和时间
          return dayjs(text).format('YYYY-MM-DD HH:mm:ss');
        }
        
        // 根据状态显示对应的截止时间
        let displayDateTime = '';
        let tooltipText = `到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
        let dateColor = '#666';
        
        if (record.status === 'available' && stages.availableDate) {
          displayDateTime = stages.availableDate;
          dateColor = '#52c41a';
          tooltipText = `可注册时间: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
        } else if (record.status === 'pending_delete' && stages.pendingDeleteEnd) {
          displayDateTime = stages.pendingDeleteEnd;
          dateColor = '#eb2f96';
          tooltipText = `待删除期截止: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
        } else if (record.status === 'redemption' && stages.redemptionEnd) {
          displayDateTime = stages.redemptionEnd;
          dateColor = '#fa541c';
          tooltipText = `赎回期截止: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
        } else if (record.status === 'grace_period' && stages.gracePeriodEnd) {
          displayDateTime = stages.gracePeriodEnd;
          dateColor = '#faad14';
          tooltipText = `宽限期截止: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
        } else if (record.status === 'expired') {
          // 已过期，根据当前时间自动判断阶段
          if (stages.gracePeriodEnd && now.isBefore(dayjs(stages.gracePeriodEnd))) {
            displayDateTime = stages.gracePeriodEnd;
            dateColor = '#faad14';
            tooltipText = `宽限期截止: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
          } else if (stages.redemptionEnd && now.isBefore(dayjs(stages.redemptionEnd))) {
            displayDateTime = stages.redemptionEnd;
            dateColor = '#fa541c';
            tooltipText = `赎回期截止: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
          } else if (stages.pendingDeleteEnd && now.isBefore(dayjs(stages.pendingDeleteEnd))) {
            displayDateTime = stages.pendingDeleteEnd;
            dateColor = '#eb2f96';
            tooltipText = `待删除期截止: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
          } else if (stages.availableDate) {
            displayDateTime = stages.availableDate;
            dateColor = '#52c41a';
            tooltipText = `可注册时间: ${displayDateTime}\n到期日期: ${dayjs(text).format('YYYY-MM-DD HH:mm:ss')}`;
          } else {
            displayDateTime = dayjs(text).format('YYYY-MM-DD HH:mm:ss');
          }
        } else {
          // 默认显示到期日期和时间
          displayDateTime = dayjs(text).format('YYYY-MM-DD HH:mm:ss');
        }
        
        // 构建完整的生命周期信息用于 Tooltip
        const buildLifecycleTooltip = () => {
          if (stages) {
            const now = dayjs();
            
            // 判断各阶段的状态
            const isExpiryPast = stages.expiryDate ? now.isAfter(dayjs(stages.expiryDate)) : false;
            const isGracePeriodPast = stages.gracePeriodEnd ? now.isAfter(dayjs(stages.gracePeriodEnd)) : false;
            const isGracePeriodActive = stages.expiryDate && stages.gracePeriodEnd 
              ? now.isAfter(dayjs(stages.expiryDate)) && now.isBefore(dayjs(stages.gracePeriodEnd))
              : false;
            
            const isRedemptionPast = stages.redemptionEnd ? now.isAfter(dayjs(stages.redemptionEnd)) : false;
            const isRedemptionActive = stages.redemptionStart && stages.redemptionEnd
              ? now.isAfter(dayjs(stages.redemptionStart)) && now.isBefore(dayjs(stages.redemptionEnd))
              : false;
            
            const isPendingDeletePast = stages.pendingDeleteEnd ? now.isAfter(dayjs(stages.pendingDeleteEnd)) : false;
            const isPendingDeleteActive = stages.pendingDeleteStart && stages.pendingDeleteEnd
              ? now.isAfter(dayjs(stages.pendingDeleteStart)) && now.isBefore(dayjs(stages.pendingDeleteEnd))
              : false;
            
            const isAvailableActive = stages.availableDate ? now.isAfter(dayjs(stages.availableDate)) : false;
            
            // 判断当前处于哪个阶段
            let currentStage = '';
            if (isAvailableActive) {
              currentStage = 'available';
            } else if (isPendingDeleteActive) {
              currentStage = 'pending_delete';
            } else if (isRedemptionActive) {
              currentStage = 'redemption';
            } else if (isGracePeriodActive) {
              currentStage = 'grace_period';
            } else if (!isExpiryPast) {
              currentStage = 'registered';
            }
            
            return (
              <div style={{ fontSize: 12, lineHeight: '22px' }}>
                <div style={{ marginBottom: 8, fontWeight: 500, borderBottom: '1px solid #eee', paddingBottom: 4 }}>
                  生命周期时间线
                </div>
                <div style={{ 
                  color: isExpiryPast ? '#999' : currentStage === 'registered' ? '#1890ff' : '#666',
                  fontWeight: currentStage === 'registered' ? 500 : 400
                }}>
                  {currentStage === 'registered' && <span style={{ marginRight: 4, color: '#1890ff' }}>●</span>}
                  <strong>到期日期:</strong> {stages.expiryDate || '-'}
                </div>
                <div style={{ 
                  color: isGracePeriodPast ? '#999' : currentStage === 'grace_period' ? '#faad14' : '#666',
                  fontWeight: currentStage === 'grace_period' ? 500 : 400
                }}>
                  {currentStage === 'grace_period' && <span style={{ marginRight: 4, color: '#faad14' }}>●</span>}
                  <strong>宽限期结束:</strong> {stages.gracePeriodEnd || '-'}
                </div>
                <div style={{ 
                  color: isRedemptionPast ? '#999' : currentStage === 'redemption' ? '#fa541c' : '#666',
                  fontWeight: currentStage === 'redemption' ? 500 : 400
                }}>
                  {currentStage === 'redemption' && <span style={{ marginRight: 4, color: '#fa541c' }}>●</span>}
                  <strong>赎回期:</strong> {stages.redemptionStart || '-'} ~ {stages.redemptionEnd || '-'}
                </div>
                <div style={{ 
                  color: isPendingDeletePast ? '#999' : currentStage === 'pending_delete' ? '#eb2f96' : '#666',
                  fontWeight: currentStage === 'pending_delete' ? 500 : 400
                }}>
                  {currentStage === 'pending_delete' && <span style={{ marginRight: 4, color: '#eb2f96' }}>●</span>}
                  <strong>待删除期:</strong> {stages.pendingDeleteStart || '-'} ~ {stages.pendingDeleteEnd || '-'}
                </div>
                <div style={{ 
                  color: isAvailableActive ? '#52c41a' : '#666',
                  marginTop: 4,
                  fontWeight: isAvailableActive ? 500 : 400
                }}>
                  {isAvailableActive && <span style={{ marginRight: 4, color: '#52c41a' }}>●</span>}
                  <strong>可注册时间:</strong> {stages.availableDate || '-'}
                </div>
              </div>
            );
          }
          return tooltipText;
        };
        
        if (displayDateTime) {
          return (
            <Tooltip title={buildLifecycleTooltip()} placement="top">
              <span style={{ color: dateColor, fontWeight: dateColor !== '#666' ? 500 : 400 }}>
                {displayDateTime}
              </span>
            </Tooltip>
          );
        }
        
        return (
          <Tooltip title={buildLifecycleTooltip()} placement="top">
            <span>{dayjs(text).format('YYYY-MM-DD HH:mm:ss')}</span>
          </Tooltip>
        );
      },
    },
    {
      title: '注册商',
      dataIndex: 'registrar',
      key: 'registrar',
      width: 200,
      ellipsis: true,
      render: (text) => text || '-',
    },
    {
      title: '监控',
      dataIndex: 'monitor',
      key: 'monitor',
      width: 80,
      render: (val, record) => (
        <Switch
          size="small"
          checked={val === 1}
          onChange={(checked) => handleToggleMonitor(record.id, checked)}
        />
      ),
    },
    {
      title: '最后检查',
      dataIndex: 'lastChecked',
      key: 'lastChecked',
      width: 160,
      render: (text) => text || '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      render: (_, record) => {
        // 可抢注的状态：可注册、已过期、赎回期、待删除
        const canSnatch = ['available', 'expired', 'redemption', 'pending_delete'].includes(record.status);
        const snatchTooltip = record.status === 'pending_delete' 
          ? '待删除状态，创建高优先级抢注任务' 
          : record.status === 'redemption'
          ? '赎回期，创建抢注任务监控'
          : '快速创建抢注任务';

        return (
          <Space>
            {canSnatch && (
              <Tooltip title={snatchTooltip}>
                <Button
                  type="link"
                  size="small"
                  icon={<RocketOutlined />}
                  style={{ 
                    color: record.status === 'pending_delete' ? '#eb2f96' : 
                           record.status === 'redemption' ? '#fa541c' : '#52c41a' 
                  }}
                  onClick={() => handleCreateSnatchTask(record)}
                />
              </Tooltip>
            )}
            <Popconfirm title="确定删除?" onConfirm={() => handleDelete(record.id)}>
              <Button type="link" danger size="small" icon={<DeleteOutlined />} />
            </Popconfirm>
          </Space>
        );
      },
    },
  ];

  return (
    <div>
      <h2 style={{ marginBottom: 16, fontWeight: 600 }}>域名管理</h2>

      <Space style={{ marginBottom: 16 }} wrap>
        <Input
          placeholder="搜索域名"
          prefix={<SearchOutlined />}
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          onPressEnter={handleSearch}
          style={{ width: 200 }}
          allowClear
        />
        <Select
          placeholder="状态筛选"
          value={status || undefined}
          onChange={(val) => { setStatus(val || ''); setPage(1); }}
          allowClear
          style={{ width: 140 }}
          options={[
            { label: '全部', value: '' },
            { label: '已注册', value: 'registered' },
            { label: '限制注册', value: 'restricted' },
            { label: '已过期', value: 'expired' },
            { label: '宽限期', value: 'grace_period' },
            { label: '赎回期', value: 'redemption' },
            { label: '待删除', value: 'pending_delete' },
            { label: '可注册', value: 'available' },
            { label: '未知', value: 'unknown' },
          ]}
        />
        <DatePicker.RangePicker
          placeholder={['到期开始', '到期结束']}
          value={expiryStart && expiryEnd ? [dayjs(expiryStart), dayjs(expiryEnd)] : null}
          onChange={(dates) => {
            if (dates) {
              setExpiryStart(dates[0]?.format('YYYY-MM-DD') || '');
              setExpiryEnd(dates[1]?.format('YYYY-MM-DD') || '');
            } else {
              setExpiryStart('');
              setExpiryEnd('');
            }
            setPage(1);
          }}
          style={{ width: 240 }}
        />
        <Select
          placeholder="排序方式"
          value={`${sortField}-${sortOrder}`}
          onChange={(val) => {
            const [field, order] = val.split('-');
            setSortField(field);
            setSortOrder(order);
            setPage(1);
          }}
          style={{ width: 160 }}
            options={[
              { label: '到期时间↑', value: 'expiry_date-asc' },
              { label: '到期时间↓', value: 'expiry_date-desc' },
              { label: '按状态排序', value: 'status-desc' },
            { label: '注册时间↑', value: 'creation_date-asc' },
            { label: '注册时间↓', value: 'creation_date-desc' },
            { label: '最后检查↑', value: 'last_checked-asc' },
            { label: '最后检查↓', value: 'last_checked-desc' },
            { label: 'ID↑', value: 'id-asc' },
            { label: 'ID↓', value: 'id-desc' },
          ]}
        />
        <Button icon={<SearchOutlined />} onClick={handleSearch}>查询</Button>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setAddModalVisible(true)}>
          添加域名
        </Button>
        <Upload
          accept=".xlsx,.xls"
          showUploadList={false}
          beforeUpload={(file) => { handleImport(file); return false; }}
        >
          <Button icon={<UploadOutlined />}>Excel导入</Button>
        </Upload>
        <Tooltip title="检查选中域名的WHOIS信息">
          <Button
            icon={<ReloadOutlined />}
            loading={checkLoading}
            onClick={handleCheck}
            disabled={selectedRowKeys.length === 0}
          >
            {checkProgress.total > 0
              ? `WHOIS检查 ${checkProgress.current}/${checkProgress.total}`
              : `WHOIS检查 (${selectedRowKeys.length})`}
          </Button>
        </Tooltip>
      </Space>

      <Table
        rowKey="id"
        columns={columns}
        dataSource={data}
        loading={loading}
        rowSelection={{
          selectedRowKeys,
          onChange: (keys) => setSelectedRowKeys(keys as number[]),
        }}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); },
        }}
        size="middle"
        scroll={{ x: 1000 }}
      />

      <Modal
        title="添加域名"
        open={addModalVisible}
        onOk={handleAdd}
        onCancel={() => { setAddModalVisible(false); form.resetFields(); }}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            name="domain"
            label="域名"
            rules={[{ required: true, message: '请输入域名' }]}
          >
            <Input placeholder="例如: example.com" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default DomainsPage;
