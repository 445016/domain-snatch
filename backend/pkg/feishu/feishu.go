package feishu

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	WebhookURL string
	HTTPClient *http.Client
}

func NewClient(webhookURL string) *Client {
	return &Client{
		WebhookURL: webhookURL,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SendText 发送纯文本消息
func (c *Client) SendText(text string) error {
	msg := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": text,
		},
	}
	return c.send(msg)
}

// SendDomainExpireCard 发送域名即将到期卡片消息
func (c *Client) SendDomainExpireCard(domains []DomainInfo) error {
	if len(domains) == 0 {
		return nil
	}

	elements := make([]interface{}, 0)
	elements = append(elements, map[string]interface{}{
		"tag": "div",
		"text": map[string]interface{}{
			"tag":     "lark_md",
			"content": fmt.Sprintf("共 **%d** 个域名即将到期，请注意处理！", len(domains)),
		},
	})
	elements = append(elements, map[string]interface{}{"tag": "hr"})

	for _, d := range domains {
		content := fmt.Sprintf("**%s**\n到期日期: %s\n注册商: %s\n状态: %s",
			d.Domain, d.ExpiryDate, d.Registrar, d.Status)
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": content,
			},
		})
		elements = append(elements, map[string]interface{}{"tag": "hr"})
	}

	card := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": "域名到期提醒",
				},
				"template": "red",
			},
			"elements": elements,
		},
	}
	return c.send(card)
}

// SendDomainAvailableCard 发送域名可注册卡片消息
func (c *Client) SendDomainAvailableCard(domains []DomainInfo) error {
	if len(domains) == 0 {
		return nil
	}

	elements := make([]interface{}, 0)
	elements = append(elements, map[string]interface{}{
		"tag": "div",
		"text": map[string]interface{}{
			"tag":     "lark_md",
			"content": fmt.Sprintf("发现 **%d** 个域名可以注册！", len(domains)),
		},
	})
	elements = append(elements, map[string]interface{}{"tag": "hr"})

	for _, d := range domains {
		content := fmt.Sprintf("**%s**\n状态: %s", d.Domain, d.Status)
		elements = append(elements, map[string]interface{}{
			"tag": "div",
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": content,
			},
		})
	}

	card := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": "域名可注册通知",
				},
				"template": "green",
			},
			"elements": elements,
		},
	}
	return c.send(card)
}

// SendSnatchStartCard 发送「开始抢注」前通知（执行抢注前先发此通知）
func (c *Client) SendSnatchStartCard(domain string) error {
	card := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": "即将抢注",
				},
				"template": "blue",
			},
			"elements": []interface{}{
				map[string]interface{}{
					"tag": "div",
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": fmt.Sprintf("**域名**: %s\n**状态**: 开始抢注", domain),
					},
				},
			},
		},
	}
	return c.send(card)
}

// SendSnatchResultCard 发送抢注结果卡片消息
func (c *Client) SendSnatchResultCard(domain, status, result string) error {
	template := "blue"
	if status == "success" {
		template = "green"
	} else if status == "failed" {
		template = "red"
	}

	card := map[string]interface{}{
		"msg_type": "interactive",
		"card": map[string]interface{}{
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": "抢注任务结果通知",
				},
				"template": template,
			},
			"elements": []interface{}{
				map[string]interface{}{
					"tag": "div",
					"text": map[string]interface{}{
						"tag":     "lark_md",
						"content": fmt.Sprintf("**域名**: %s\n**状态**: %s\n**结果**: %s", domain, status, result),
					},
				},
			},
		},
	}
	return c.send(card)
}

func (c *Client) send(msg interface{}) error {
	if c.WebhookURL == "" {
		return fmt.Errorf("webhook url is empty")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message failed: %w", err)
	}

	resp, err := c.HTTPClient.Post(c.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send message failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu api error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("feishu api error: code=%d, msg=%s", result.Code, result.Msg)
	}

	return nil
}

type DomainInfo struct {
	Domain     string
	ExpiryDate string
	Registrar  string
	Status     string
}
