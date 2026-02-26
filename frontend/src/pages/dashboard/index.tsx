import React, { useEffect, useState } from 'react';
import { Card, Col, Row, Statistic, Spin } from 'antd';
import {
  GlobalOutlined,
  EyeOutlined,
  WarningOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  TrophyOutlined,
} from '@ant-design/icons';
import { getDashboardStats } from '../../api';

interface Stats {
  totalDomains: number;
  monitorCount: number;
  expiringSoon: number;
  availableCount: number;
  snatchPending: number;
  snatchSuccess: number;
}

const DashboardPage: React.FC = () => {
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    getDashboardStats()
      .then((res: any) => setStats(res))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  }

  const cards = [
    {
      title: '域名总数',
      value: stats?.totalDomains || 0,
      icon: <GlobalOutlined />,
      color: '#1677ff',
      bg: '#e6f4ff',
    },
    {
      title: '监控中',
      value: stats?.monitorCount || 0,
      icon: <EyeOutlined />,
      color: '#52c41a',
      bg: '#f6ffed',
    },
    {
      title: '即将到期',
      value: stats?.expiringSoon || 0,
      icon: <WarningOutlined />,
      color: '#faad14',
      bg: '#fffbe6',
    },
    {
      title: '可注册',
      value: stats?.availableCount || 0,
      icon: <CheckCircleOutlined />,
      color: '#13c2c2',
      bg: '#e6fffb',
    },
    {
      title: '待抢注',
      value: stats?.snatchPending || 0,
      icon: <ClockCircleOutlined />,
      color: '#722ed1',
      bg: '#f9f0ff',
    },
    {
      title: '抢注成功',
      value: stats?.snatchSuccess || 0,
      icon: <TrophyOutlined />,
      color: '#eb2f96',
      bg: '#fff0f6',
    },
  ];

  return (
    <div>
      <h2 style={{ marginBottom: 24, fontWeight: 600 }}>仪表盘</h2>
      <Row gutter={[16, 16]}>
        {cards.map((card) => (
          <Col xs={24} sm={12} lg={8} key={card.title}>
            <Card
              hoverable
              style={{ borderRadius: 12, border: 'none', background: card.bg }}
              bodyStyle={{ padding: '24px' }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                <div style={{
                  width: 56,
                  height: 56,
                  borderRadius: 12,
                  background: card.color,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 24,
                  color: '#fff',
                }}>
                  {card.icon}
                </div>
                <Statistic
                  title={<span style={{ color: '#595959' }}>{card.title}</span>}
                  value={card.value}
                  valueStyle={{ color: card.color, fontWeight: 700 }}
                />
              </div>
            </Card>
          </Col>
        ))}
      </Row>
    </div>
  );
};

export default DashboardPage;
