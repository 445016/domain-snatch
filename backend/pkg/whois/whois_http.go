package whois

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// QueryHTTP 使用HTTP API查询WHOIS（备用方案）
func QueryHTTP(domain string) (*Result, error) {
	log.Printf("[WHOIS][HTTP] Start, domain=%s", domain)
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	// 使用jsonwhois.com的免费API
	url := fmt.Sprintf("https://jsonwhois.com/api/v1/whois?domain=%s", domain)
	
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("[WHOIS][HTTP] Request failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WHOIS][HTTP] Non-200 status, domain=%s, statusCode=%d", domain, resp.StatusCode)
		return result, fmt.Errorf("http request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[WHOIS][HTTP] Read response failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("read response failed: %w", err)
	}

	log.Printf("[WHOIS][HTTP] Got response, domain=%s, bodyLen=%d, preview=%s", 
		domain, len(body), getPreview(string(body), 200))

	var apiResp struct {
		Status string `json:"status"`
		Result struct {
			RegistryExpiryDate string `json:"registry_expiry_date"`
			CreationDate       string `json:"creation_date"`
			Registrar          string `json:"registrar"`
			DomainName         string `json:"domain_name"`
			RawText            string `json:"raw_text"`
		} `json:"result"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		log.Printf("[WHOIS][HTTP] Parse JSON failed, domain=%s, err=%v, bodyPreview=%s", 
			domain, err, getPreview(string(body), 200))
		return result, fmt.Errorf("parse response failed: %w", err)
	}

	result.WhoisRaw = apiResp.Result.RawText
	
	// 解析状态
	if apiResp.Status == "available" || strings.Contains(strings.ToLower(result.WhoisRaw), "no match") {
		log.Printf("[WHOIS][HTTP] Domain available, domain=%s", domain)
		result.Status = "available"
		result.CanRegister = true
		return result, nil
	}

	result.Status = "registered"
	result.Registrar = apiResp.Result.Registrar

	// 解析日期
	if apiResp.Result.RegistryExpiryDate != "" {
		log.Printf("[WHOIS][HTTP] Found expiry date, domain=%s, expiryDate=%s", domain, apiResp.Result.RegistryExpiryDate)
		for _, layout := range dateLayouts {
			if t, err := time.Parse(layout, apiResp.Result.RegistryExpiryDate); err == nil {
				result.ExpiryDate = &t
				if t.Before(time.Now()) {
					result.Status = "expired"
					result.CanRegister = true
				} else {
					result.CanRegister = false
				}
				log.Printf("[WHOIS][HTTP] Parsed expiry date, domain=%s, expiryDate=%s, layout=%s", 
					domain, t.Format("2006-01-02 15:04:05"), layout)
				break
			}
		}
	} else {
		log.Printf("[WHOIS][HTTP] No expiry date in API response, domain=%s", domain)
	}

	if apiResp.Result.CreationDate != "" {
		log.Printf("[WHOIS][HTTP] Found creation date, domain=%s, creationDate=%s", domain, apiResp.Result.CreationDate)
		for _, layout := range dateLayouts {
			if t, err := time.Parse(layout, apiResp.Result.CreationDate); err == nil {
				result.CreationDate = &t
				log.Printf("[WHOIS][HTTP] Parsed creation date, domain=%s, creationDate=%s, layout=%s", 
					domain, t.Format("2006-01-02 15:04:05"), layout)
				break
			}
		}
	}

	log.Printf("[WHOIS][HTTP] Success, domain=%s, status=%s, registrar=%s, expiryDate=%v, creationDate=%v", 
		domain, result.Status, result.Registrar, result.ExpiryDate != nil, result.CreationDate != nil)
	return result, nil
}

// QueryWithRetry 带重试的查询，先尝试命令行，失败后使用HTTP API
func QueryWithRetry(domain string) (*Result, error) {
	// 第一次尝试：使用whois命令（5秒超时）
	result, err := QueryWithTimeout(domain, 5*time.Second)
	if err == nil {
		return result, nil
	}

	// 如果超时，尝试HTTP API
	if strings.Contains(err.Error(), "timeout") {
		return QueryHTTP(domain)
	}

	return result, err
}
