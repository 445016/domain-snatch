package whois

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"
)

// QueryWithLib 使用likexian/whois库查询（更稳定，自带重试和超时）
func QueryWithLib(domain string) (*Result, error) {
	log.Printf("[WHOIS][Lib] Start, domain=%s", domain)
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	// 使用whois库查询（自带超时和重试机制）
	raw, err := whois.Whois(domain)
	if err != nil {
		// 检查是否是not found错误
		if strings.Contains(strings.ToLower(err.Error()), "not found") ||
			strings.Contains(strings.ToLower(err.Error()), "no match") {
			log.Printf("[WHOIS][Lib] Domain not found, domain=%s", domain)
			result.Status = "available"
			result.CanRegister = true
			return result, nil
		}
		log.Printf("[WHOIS][Lib] Query failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("whois query failed: %w", err)
	}

	log.Printf("[WHOIS][Lib] Got response, domain=%s, rawLen=%d, preview=%s",
		domain, len(raw), getPreview(raw, 200))

	raw = normalizeWhoisRaw(raw)
	result.WhoisRaw = raw

	// 检查是否未注册
	if containsNotFound(raw) {
		log.Printf("[WHOIS][Lib] Domain not found (containsNotFound), domain=%s", domain)
		result.Status = "available"
		result.CanRegister = true
		return result, nil
	}

	// 使用whois-parser解析结果
	parsed, err := whoisparser.Parse(raw)
	if err != nil {
		log.Printf("[WHOIS][Lib] Parser failed, using fallback parsing, domain=%s, err=%v", domain, err)
		// 解析失败，但有原始数据，尝试简单解析
		result.Status = "registered"
		result.Registrar = parseField(raw, []string{"Registrar:", "registrar:"})

		expiryDate := parseDate(raw, []string{
			"Registry Expiry Date:",
			"Expiry Date:",
			"Expire Date:", // .it 域名使用此格式
			"Expiration Date:",
			"Registrar Registration Expiration Date:",
		})
		if expiryDate != nil {
			result.ExpiryDate = expiryDate
			if expiryDate.Before(time.Now()) {
				result.Status = "expired"
				result.CanRegister = true
			}
		}
		log.Printf("[WHOIS][Lib] Fallback parse result, domain=%s, status=%s, registrar=%s, expiryDate=%v",
			domain, result.Status, result.Registrar, result.ExpiryDate != nil)
		return result, nil
	}

	// 使用解析后的结构化数据
	if parsed.Domain != nil {
		result.Registrar = parsed.Registrar.Name

		// 解析到期日期
		if parsed.Domain.ExpirationDate != "" {
			for _, layout := range dateLayouts {
				if t, err := time.Parse(layout, parsed.Domain.ExpirationDate); err == nil {
					result.ExpiryDate = &t
					if t.Before(time.Now()) {
						result.Status = "expired"
						result.CanRegister = true
					} else {
						result.Status = "registered"
						result.CanRegister = false
					}
					break
				}
			}
		} else {
			result.Status = "registered"
			result.CanRegister = false
		}

		// 解析创建日期
		if parsed.Domain.CreatedDate != "" {
			for _, layout := range dateLayouts {
				if t, err := time.Parse(layout, parsed.Domain.CreatedDate); err == nil {
					result.CreationDate = &t
					break
				}
			}
		}
		log.Printf("[WHOIS][Lib] Parse success, domain=%s, status=%s, registrar=%s, expiryDate=%v, creationDate=%v",
			domain, result.Status, result.Registrar, result.ExpiryDate != nil, result.CreationDate != nil)
	} else {
		log.Printf("[WHOIS][Lib] Parsed domain is nil, domain=%s", domain)
	}

	return result, nil
}

// QuerySmart 智能查询：优先使用库，失败时fallback到命令行
func QuerySmart(domain string) (*Result, error) {
	// 优先使用whois库（更稳定，支持更多TLD）
	result, err := QueryWithLib(domain)
	if err == nil {
		return result, nil
	}

	// 如果库查询失败，fallback到命令行
	result, err2 := QueryWithTimeout(domain, 10*time.Second)
	if err2 == nil {
		return result, nil
	}

	// 两种方法都失败，返回第一个错误
	return result, err
}
