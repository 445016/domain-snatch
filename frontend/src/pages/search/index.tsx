import React, { useState } from 'react';
import { Input, Button, Card, Descriptions, Tag, Spin, Empty, message } from 'antd';
import { SearchOutlined, GlobalOutlined } from '@ant-design/icons';
import { whoisQuery } from '../../api';

interface WhoisResult {
  domain: string;
  status: string;
  expiryDate: string;
  creationDate: string;
  registrar: string;
  canRegister: boolean;
  whoisRaw: string;
}

const SearchPage: React.FC = () => {
  const [domain, setDomain] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<WhoisResult | null>(null);
  const [showRaw, setShowRaw] = useState(false);

  const handleSearch = async () => {
    if (!domain.trim()) {
      message.warning('请输入域名');
      return;
    }
    setLoading(true);
    setResult(null);
    try {
      const res: any = await whoisQuery({ domain: domain.trim() });
      setResult(res);
    } catch (e: any) {
      message.error(e?.response?.data?.message || '查询失败');
    } finally {
      setLoading(false);
    }
  };

  const statusMap: Record<string, { color: string; text: string }> = {
    registered: { color: 'blue', text: '已注册' },
    restricted: { color: 'default', text: '限制注册' },
    expired: { color: 'red', text: '已过期' },
    available: { color: 'green', text: '可注册' },
    unknown: { color: 'default', text: '未知' },
  };

  return (
    <div>
      <h2 style={{ marginBottom: 24, fontWeight: 600 }}>域名查询</h2>

      <Card style={{ maxWidth: 800, margin: '0 auto', borderRadius: 12 }}>
        <div style={{ display: 'flex', gap: 12, marginBottom: 24 }}>
          <Input
            size="large"
            prefix={<GlobalOutlined />}
            placeholder="输入域名查询 WHOIS 信息，例如: example.com"
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            onPressEnter={handleSearch}
          />
          <Button
            type="primary"
            size="large"
            icon={<SearchOutlined />}
            loading={loading}
            onClick={handleSearch}
          >
            查询
          </Button>
        </div>

        {loading && (
          <div style={{ textAlign: 'center', padding: 60 }}>
            <Spin size="large" tip="正在查询 WHOIS 信息..." />
          </div>
        )}

        {!loading && !result && (
          <Empty description="输入域名后点击查询" style={{ padding: 40 }} />
        )}

        {!loading && result && (
          <div>
            <Descriptions
              bordered
              column={1}
              size="middle"
              style={{ marginBottom: 16 }}
            >
              <Descriptions.Item label="域名">
                <span style={{ fontWeight: 600, fontSize: 16 }}>{result.domain}</span>
              </Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={statusMap[result.status]?.color || 'default'}>
                  {statusMap[result.status]?.text || result.status}
                </Tag>
                {result.canRegister && (
                  <Tag color="green" style={{ marginLeft: 8 }}>可注册</Tag>
                )}
              </Descriptions.Item>
              <Descriptions.Item label="到期日期">
                {result.expiryDate || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="注册日期">
                {result.creationDate || '-'}
              </Descriptions.Item>
              <Descriptions.Item label="注册商">
                {result.registrar || '-'}
              </Descriptions.Item>
            </Descriptions>

            {result.whoisRaw && (
              <div>
                <Button
                  type="link"
                  onClick={() => setShowRaw(!showRaw)}
                  style={{ padding: 0 }}
                >
                  {showRaw ? '收起' : '查看'} WHOIS 原始信息
                </Button>
                {showRaw && (
                  <pre style={{
                    marginTop: 12,
                    padding: 16,
                    background: '#f5f5f5',
                    borderRadius: 8,
                    maxHeight: 400,
                    overflow: 'auto',
                    fontSize: 12,
                    lineHeight: 1.6,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                  }}>
                    {result.whoisRaw}
                  </pre>
                )}
              </div>
            )}
          </div>
        )}
      </Card>
    </div>
  );
};

export default SearchPage;
