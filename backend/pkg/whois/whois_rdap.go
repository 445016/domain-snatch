package whois

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Identity Digital RDAP 基础 URL（.money、.finance 等 TLD）
const identityDigitalRDAPBase = "https://rdap.identitydigital.services/rdap/domain/"

// RDAP 引导查询（ICANN bootstrap，可解析 .app、.dev 等 Google 及其他注册局 TLD）
const rdapBootstrapURL = "https://rdap.org/domain/"

// rdapDomain 解析 RDAP 域名查询响应（仅解析用到的字段）
type rdapDomain struct {
	LDHName     string        `json:"ldhName"`
	Status      []string      `json:"status"`
	Events      []rdapEvent   `json:"events"`
	Entities    []rdapEntity  `json:"entities"`
}

type rdapEvent struct {
	EventAction string `json:"eventAction"` // expiration, registration, last changed
	EventDate   string `json:"eventDate"`   // RFC3339
}

type rdapEntity struct {
	Roles      []string    `json:"roles"` // registrar, registrant, etc.
	VCardArray []any       `json:"vcardArray"`
}

var rdapHTTPClient = &http.Client{Timeout: 15 * time.Second}

func QueryWithIdentityDigitalRDAP(domain string) (*Result, error) {
	url := identityDigitalRDAPBase + domain
	data, err := fetchRDAP(url, domain)
	if err != nil {
		return &Result{Domain: domain, Status: StatusUnknown}, err
	}
	result := parseRDAPResponse(domain, data)
	log.Printf("[WHOIS][RDAP] Identity Digital parsed, domain=%s, status=%s, registrar=%s", domain, result.Status, result.Registrar)
	return result, nil
}

// QueryRDAPBootstrap 通过 rdap.org 引导查询（ICANN bootstrap），支持 .app、.dev 等仅 RDAP 的 TLD
func QueryRDAPBootstrap(domain string) (*Result, error) {
	url := rdapBootstrapURL + domain
	data, err := fetchRDAP(url, domain)
	if err != nil {
		return &Result{Domain: domain, Status: StatusUnknown}, err
	}
	result := parseRDAPResponse(domain, data)
	log.Printf("[WHOIS][RDAP] Bootstrap parsed, domain=%s, status=%s, registrar=%s", domain, result.Status, result.Registrar)
	return result, nil
}

func fetchRDAP(url, domain string) (*rdapDomain, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("rdap request: %w", err)
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := rdapHTTPClient.Do(req)
	if err != nil {
		log.Printf("[WHOIS][RDAP] HTTP failed, url=%s, err=%v", url, err)
		return nil, fmt.Errorf("rdap http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[WHOIS][RDAP] HTTP status %d, url=%s", resp.StatusCode, url)
		return nil, fmt.Errorf("rdap status %d", resp.StatusCode)
	}
	var data rdapDomain
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("[WHOIS][RDAP] JSON decode failed, domain=%s, err=%v", domain, err)
		return nil, fmt.Errorf("rdap decode: %w", err)
	}
	if data.LDHName == "" {
		data.LDHName = domain
	}
	return &data, nil
}

// parseRDAPResponse 将 RDAP 响应解析为 Result（与具体 RDAP 服务器无关）
func parseRDAPResponse(domain string, data *rdapDomain) *Result {
	result := &Result{
		Domain: domain,
		Status: StatusUnknown,
	}
	result.WhoisRaw = buildRDAPRawSummary(data)

	for _, e := range data.Events {
		t, err := time.Parse(time.RFC3339, e.EventDate)
		if err != nil {
			continue
		}
		switch strings.ToLower(e.EventAction) {
		case "expiration":
			result.ExpiryDate = &t
		case "registration":
			result.CreationDate = &t
		}
	}

	for _, ent := range data.Entities {
		for _, r := range ent.Roles {
			if strings.ToLower(r) != "registrar" {
				continue
			}
			result.Registrar = parseRDAPRegistrarName(ent.VCardArray)
			break
		}
		if result.Registrar != "" {
			break
		}
	}

	now := time.Now()
	if len(data.Status) > 0 {
		result.WhoisStatus = strings.Join(data.Status, ", ")
		for _, s := range data.Status {
			lower := strings.ToLower(s)
			if strings.Contains(lower, "delete prohibited") ||
				strings.Contains(lower, "renew prohibited") ||
				strings.Contains(lower, "transfer prohibited") ||
				strings.Contains(lower, "update prohibited") ||
				strings.Contains(lower, "associated") {
				result.Status = StatusRegistered
				result.CanRegister = false
				break
			}
		}
	}

	if result.Status == StatusUnknown && result.ExpiryDate != nil {
		if result.ExpiryDate.Before(now) {
			result.Status = StatusExpired
			result.CanRegister = true
		} else {
			result.Status = StatusRegistered
			result.CanRegister = false
		}
	}
	if result.Status == StatusUnknown {
		result.Status = StatusRegistered
		result.CanRegister = false
	}
	if result.ExpiryDate != nil && (result.Status == StatusExpired || result.Status == StatusRegistered) {
		d := result.ExpiryDate.AddDate(0, 0, 75)
		result.DeleteDate = &d
	}
	return result
}

// parseRDAPRegistrarName 从 RDAP entity 的 vcardArray 中取出注册商名称（fn）
func parseRDAPRegistrarName(vcardArray []any) string {
	if len(vcardArray) < 2 {
		return ""
	}
	arr, ok := vcardArray[1].([]any)
	if !ok {
		return ""
	}
	for _, row := range arr {
		parts, ok := row.([]any)
		if !ok || len(parts) < 4 {
			continue
		}
		key, _ := parts[0].(string)
		if strings.ToLower(key) == "fn" {
			if name, ok := parts[3].(string); ok {
				return strings.TrimSpace(name)
			}
		}
	}
	return ""
}

// buildRDAPRawSummary 生成便于存储的 RDAP 文本摘要（兼容现有 WhoisRaw 用途）
func buildRDAPRawSummary(d *rdapDomain) string {
	var b strings.Builder
	b.WriteString("RDAP Response\n")
	b.WriteString("Domain: " + d.LDHName + "\n")
	if len(d.Status) > 0 {
		b.WriteString("Status: " + strings.Join(d.Status, ", ") + "\n")
	}
	for _, e := range d.Events {
		b.WriteString(e.EventAction + ": " + e.EventDate + "\n")
	}
	return b.String()
}
