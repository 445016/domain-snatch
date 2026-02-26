import React, { useEffect, useState } from 'react';
import { Card, Form, Input, InputNumber, Switch, Button, message, Spin } from 'antd';
import { BellOutlined, SaveOutlined } from '@ant-design/icons';
import { getNotifySettings, updateNotifySettings } from '../../api';

const NotifyPage: React.FC = () => {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    getNotifySettings()
      .then((res: any) => {
        form.setFieldsValue({
          webhookUrl: res.webhookUrl,
          expireDays: res.expireDays,
          enabled: res.enabled === 1,
        });
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleSave = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await updateNotifySettings({
        webhookUrl: values.webhookUrl || '',
        expireDays: values.expireDays,
        enabled: values.enabled ? 1 : 0,
      });
      message.success('保存成功');
    } catch (e) {
      message.error('保存失败');
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;
  }

  return (
    <div>
      <h2 style={{ marginBottom: 24, fontWeight: 600 }}>通知设置</h2>

      <Card
        style={{ maxWidth: 600, borderRadius: 12 }}
        title={
          <span>
            <BellOutlined style={{ marginRight: 8 }} />
            飞书通知配置
          </span>
        }
      >
        <Form
          form={form}
          layout="vertical"
          initialValues={{ expireDays: 30, enabled: false }}
        >
          <Form.Item
            name="enabled"
            label="启用通知"
            valuePropName="checked"
          >
            <Switch checkedChildren="开" unCheckedChildren="关" />
          </Form.Item>

          <Form.Item
            name="webhookUrl"
            label="飞书 Webhook URL"
            rules={[
              {
                type: 'url',
                message: '请输入有效的URL',
              },
            ]}
            extra="在飞书群中添加自定义机器人，复制 Webhook 地址粘贴到这里"
          >
            <Input placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/xxxxx" />
          </Form.Item>

          <Form.Item
            name="expireDays"
            label="到期提前提醒天数"
            extra="域名到期前多少天开始发送提醒通知"
          >
            <InputNumber min={1} max={365} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item>
            <Button
              type="primary"
              icon={<SaveOutlined />}
              loading={saving}
              onClick={handleSave}
            >
              保存设置
            </Button>
          </Form.Item>
        </Form>

        <div style={{
          marginTop: 24,
          padding: 16,
          background: '#f6f6f6',
          borderRadius: 8,
          fontSize: 13,
          color: '#595959',
        }}>
          <p style={{ fontWeight: 600, marginBottom: 8 }}>通知场景说明：</p>
          <ul style={{ paddingLeft: 20, margin: 0 }}>
            <li>域名即将到期 - 监控中的域名到期前 N 天自动发送飞书通知</li>
            <li>域名可注册 - 定时巡检发现域名已过期或可注册时发送通知</li>
            <li>抢注任务结果 - 抢注任务状态变更时发送通知</li>
          </ul>
        </div>
      </Card>
    </div>
  );
};

export default NotifyPage;
