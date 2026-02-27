package godaddy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	ProductionBaseURL = "https://api.godaddy.com"
	SandboxBaseURL    = "https://api.ote-godaddy.com"
)

// Client GoDaddy API 客户端
type Client struct {
	APIKey     string
	APISecret  string
	BaseURL    string
	HTTPClient *http.Client
}

// Address 地址（GoDaddy Contact 要求 addressMailing 为嵌套对象）
type Address struct {
	Address1   string `json:"address1"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postalCode"`
	Country    string `json:"country"` // ISO 3166-1 alpha-2, e.g., "US", "CN"
}

// ContactInfo 联系人信息（域名注册需要；addressMailing 必须为嵌套对象否则 API 422）
type ContactInfo struct {
	FirstName       string  `json:"nameFirst"`
	LastName        string  `json:"nameLast"`
	Email           string  `json:"email"`
	Phone           string  `json:"phone"`
	Organization    string  `json:"organization,omitempty"`
	AddressMailing  Address `json:"addressMailing"`
}

// AvailabilityResponse 域名可用性检查响应
type AvailabilityResponse struct {
	Available  bool   `json:"available"`
	Domain     string `json:"domain"`
	Definitive bool   `json:"definitive"`
	Price      int    `json:"price"` // 价格（单位：微单位，如 1000000 = $1）
	Currency   string `json:"currency"`
	Period     int    `json:"period"` // 注册年限
}

// PurchaseRequest 购买域名请求
type PurchaseRequest struct {
	Domain  string      `json:"domain"`
	Consent Consent     `json:"consent"`
	Contact ContactInfo `json:"contactAdmin"`
	Period  int         `json:"period"` // 注册年限，默认 1
}

// Consent 同意条款
type Consent struct {
	AgreedAt      string   `json:"agreedAt"`
	AgreedBy      string   `json:"agreedBy"`
	AgreementKeys []string `json:"agreementKeys"`
}

// PurchaseResponse 购买域名响应
type PurchaseResponse struct {
	OrderID  int    `json:"orderId"`
	ItemCnt  int    `json:"itemCount"`
	Total    int    `json:"total"`
	Currency string `json:"currency"`
}

// ErrorResponse GoDaddy API 错误响应
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fields  []struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	} `json:"fields,omitempty"`
}

// NewClient 创建 GoDaddy 客户端
func NewClient(apiKey, apiSecret string, sandbox bool) *Client {
	baseURL := ProductionBaseURL
	if sandbox {
		baseURL = SandboxBaseURL
	}

	return &Client{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

const maxLogBody = 2048 // 请求/响应体日志最大长度，避免刷屏

// doRequest 执行 HTTP 请求，并记录请求地址、参数、响应状态与结果
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
	fullURL := c.BaseURL + path
	var reqBody io.Reader
	var reqBodyBytes []byte
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body failed: %w", err)
		}
		reqBodyBytes = jsonData
		reqBody = bytes.NewBuffer(jsonData)
	}

	log.Printf("[Godaddy] Request %s %s", method, fullURL)
	if len(reqBodyBytes) > 0 {
		logBody := string(reqBodyBytes)
		if len(logBody) > maxLogBody {
			logBody = logBody[:maxLogBody] + "...(truncated)"
		}
		log.Printf("[Godaddy] Request body: %s", logBody)
	}

	req, err := http.NewRequest(method, fullURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", c.APIKey, c.APISecret))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	respPreview := string(respBody)
	if len(respPreview) > maxLogBody {
		respPreview = respPreview[:maxLogBody] + "...(truncated)"
	}
	log.Printf("[Godaddy] Response status=%d body: %s", resp.StatusCode, respPreview)

	// 检查错误响应（422 附带 fields；402 附带支付配置提示）
	if resp.StatusCode >= 400 {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Message != "" {
			msg := errResp.Message
			if len(errResp.Fields) > 0 {
				for _, f := range errResp.Fields {
					msg += "; " + f.Path + ": " + f.Message
				}
			}
			if resp.StatusCode == 402 && errResp.Code == "INVALID_PAYMENT_INFO" {
				msg += "（OTE 环境需在 GoDaddy 开发者账号下为测试环境绑定支付方式，或联系 GoDaddy 支持开通 OTE 预充值）"
			}
			return nil, fmt.Errorf("API error [%d]: %s - %s", resp.StatusCode, errResp.Code, msg)
		}
		return nil, fmt.Errorf("API error [%d]: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CheckAvailable 检查域名是否可注册
// 返回: 是否可用, 价格（美元）, 错误
func (c *Client) CheckAvailable(domain string) (*AvailabilityResponse, error) {
	path := fmt.Sprintf("/v1/domains/available?domain=%s&checkType=FULL", domain)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result AvailabilityResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response failed: %w", err)
	}

	return &result, nil
}

// GetAgreements 获取域名注册协议
func (c *Client) GetAgreements(domain string) ([]string, error) {
	// 从域名中提取 TLD
	tld := domain
	if idx := len(domain) - len(domain); idx >= 0 {
		for i := len(domain) - 1; i >= 0; i-- {
			if domain[i] == '.' {
				tld = domain[i+1:]
				break
			}
		}
	}

	path := fmt.Sprintf("/v1/domains/agreements?tlds=%s&privacy=false", tld)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var agreements []struct {
		AgreementKey string `json:"agreementKey"`
		Title        string `json:"title"`
		Content      string `json:"content"`
	}

	if err := json.Unmarshal(respBody, &agreements); err != nil {
		return nil, fmt.Errorf("parse agreements failed: %w", err)
	}

	keys := make([]string, len(agreements))
	for i, a := range agreements {
		keys[i] = a.AgreementKey
	}

	return keys, nil
}

// Purchase 购买/注册域名
func (c *Client) Purchase(domain string, contact ContactInfo, years int) (*PurchaseResponse, error) {
	if years <= 0 {
		years = 1
	}

	// 获取协议 keys
	agreementKeys, err := c.GetAgreements(domain)
	if err != nil {
		return nil, fmt.Errorf("get agreements failed: %w", err)
	}

	// 构建请求
	purchaseReq := map[string]interface{}{
		"domain": domain,
		"period": years,
		"consent": map[string]interface{}{
			"agreedAt":      time.Now().UTC().Format(time.RFC3339),
			"agreedBy":      contact.Email,
			"agreementKeys": agreementKeys,
		},
		"contactAdmin":      contact,
		"contactBilling":    contact,
		"contactRegistrant": contact,
		"contactTech":       contact,
		"privacy":           false,
		"renewAuto":         true,
	}

	respBody, err := c.doRequest("POST", "/v1/domains/purchase", purchaseReq)
	if err != nil {
		return nil, err
	}

	var result PurchaseResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse purchase response failed: %w", err)
	}

	return &result, nil
}

// GetDomainInfo 获取域名信息（已注册的域名）
func (c *Client) GetDomainInfo(domain string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/v1/domains/%s", domain)

	respBody, err := c.doRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse domain info failed: %w", err)
	}

	return result, nil
}

// ValidateCredentials 验证 API 凭证是否有效
func (c *Client) ValidateCredentials() error {
	// 尝试获取 domains 列表来验证凭证
	_, err := c.doRequest("GET", "/v1/domains?limit=1", nil)
	if err != nil {
		return fmt.Errorf("credential validation failed: %w", err)
	}
	return nil
}
