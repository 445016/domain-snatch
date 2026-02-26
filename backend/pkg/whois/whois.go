package whois

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 域名状态常量
const (
	StatusRegistered    = "registered"     // 已注册，正常状态
	StatusExpired       = "expired"        // 已过期
	StatusGracePeriod   = "grace_period"  // 宽限期（原持有人可续费）
	StatusRedemption    = "redemption"    // 赎回期（高价赎回）
	StatusPendingDelete = "pending_delete" // 待删除（即将释放）
	StatusAvailable     = "available"     // 可注册
	StatusRestricted    = "restricted"     // 限制注册（政策/保留，无到期日，cron 不参与状态更新）
	StatusUnknown       = "unknown"       // 未知
)

type Result struct {
	Domain       string
	Status       string // registered / expired / grace_period / redemption / pending_delete / available / unknown
	ExpiryDate   *time.Time
	CreationDate *time.Time
	DeleteDate   *time.Time // 预计删除日期（从 WHOIS 解析或计算）
	Registrar    string
	CanRegister  bool
	WhoisRaw     string
	WhoisStatus  string // 原始 Domain Status 字段内容
}

// QueryFast 快速查询，直接使用 whois 命令，超时时间短（10秒）
// 适用于大多数常见 TLD，速度最快
// 注意：某些域名的 WHOIS 查询可能需要更长时间，如果 10 秒内没有返回数据，会继续尝试其他方式
func QueryFast(domain string) (*Result, error) {
	log.Printf("[WHOIS][Fast] Query start, domain=%s", domain)
	result, err := QueryWithTimeout(domain, 10*time.Second)
	if err == nil && result.WhoisRaw != "" && !IsOnlyIANAInfo(result.WhoisRaw) {
		log.Printf("[WHOIS][Fast] Query success, domain=%s, rawLen=%d", domain, len(result.WhoisRaw))
		return result, nil
	}
	if err != nil {
		log.Printf("[WHOIS][Fast] Query failed, domain=%s, err=%v", domain, err)
	}
	// 如果超时但没有数据，返回错误，让调用者尝试其他方式
	if result.WhoisRaw == "" {
		return result, fmt.Errorf("whois query timeout after 10s (got 0 bytes)")
	}
	return result, err
}

// isIdentityDigitalTLD 判断是否为 Identity Digital 管理的 TLD（.money, .finance 等），需直连 whois.nic.* 才能查到
func isIdentityDigitalTLD(domain string) bool {
	tld := ""
	if i := strings.LastIndex(domain, "."); i >= 0 && i+1 < len(domain) {
		tld = strings.ToLower(domain[i+1:])
	}
	switch tld {
	case "money", "finance", "capital", "live", "london", "wiki", "radio", "diet", "rest":
		return true
	default:
		return false
	}
}

// isRDAPOnlyTLD 判断是否为「whois 仅返回 IANA TLD 信息、需用 RDAP 才能查到域名」的 TLD（如 .app、.dev）
func isRDAPOnlyTLD(domain string) bool {
	tld := ""
	if i := strings.LastIndex(domain, "."); i >= 0 && i+1 < len(domain) {
		tld = strings.ToLower(domain[i+1:])
	}
	switch tld {
	case "app", "dev":
		return true
	default:
		return false
	}
}

// isAfiliasTLD 判断是否为 Afilias 管理的 TLD（.bz 等），默认 whois 只返回 IANA，需直连 whois.afilias.net
func isAfiliasTLD(domain string) bool {
	tld := ""
	if i := strings.LastIndex(domain, "."); i >= 0 && i+1 < len(domain) {
		tld = strings.ToLower(domain[i+1:])
	}
	switch tld {
	case "bz", "ag", "vc", "sc":
		return true
	default:
		return false
	}
}

// isNIXITLD 判断是否为印度 NIXI 管理的 TLD（.in），直连 whois.nixiregistry.in 可稳定拿到状态（含 restricted by registry policy）
func isNIXITLD(domain string) bool {
	tld := ""
	if i := strings.LastIndex(domain, "."); i >= 0 && i+1 < len(domain) {
		tld = strings.ToLower(domain[i+1:])
	}
	return tld == "in"
}

func Query(domain string) (*Result, error) {
	log.Printf("[WHOIS] Query start, domain=%s", domain)

	// .money、.finance 等 Identity Digital TLD：WHOIS 常返回 "TLD is not supported"，需先直连 WHOIS 再 RDAP 回退
	if isIdentityDigitalTLD(domain) {
		log.Printf("[WHOIS] Identity Digital TLD detected, trying QueryWithIdentityDigitalServer first, domain=%s", domain)
		result, err := QueryWithIdentityDigitalServer(domain)
		if err == nil && result != nil && result.WhoisRaw != "" && !IsOnlyIANAInfo(result.WhoisRaw) {
			log.Printf("[WHOIS] QueryWithIdentityDigitalServer success, domain=%s, rawLen=%d", domain, len(result.WhoisRaw))
			return result, nil
		}
		if err != nil {
			log.Printf("[WHOIS] QueryWithIdentityDigitalServer failed, trying RDAP, domain=%s, err=%v", domain, err)
		}
		// WHOIS 不可用时使用 RDAP（如 .money 已仅支持 RDAP）
		rdapResult, rdapErr := QueryWithIdentityDigitalRDAP(domain)
		if rdapErr == nil && rdapResult != nil && rdapResult.WhoisRaw != "" {
			log.Printf("[WHOIS] QueryWithIdentityDigitalRDAP success, domain=%s, status=%s", domain, rdapResult.Status)
			return rdapResult, nil
		}
		if rdapErr != nil {
			log.Printf("[WHOIS] QueryWithIdentityDigitalRDAP failed, fallback to PTY, domain=%s, err=%v", domain, rdapErr)
		}
	}

	// .app、.dev 等 TLD 的 whois 只返回 IANA TLD 说明，无域名级数据；先走 RDAP 避免返回错误解析结果
	if isRDAPOnlyTLD(domain) {
		log.Printf("[WHOIS] RDAP-only TLD (.app/.dev) detected, trying QueryRDAPBootstrap first, domain=%s", domain)
		rdapResult, rdapErr := QueryRDAPBootstrap(domain)
		if rdapErr == nil && rdapResult != nil && rdapResult.WhoisRaw != "" {
			log.Printf("[WHOIS] QueryRDAPBootstrap success, domain=%s, status=%s", domain, rdapResult.Status)
			return rdapResult, nil
		}
		if rdapErr != nil {
			log.Printf("[WHOIS] QueryRDAPBootstrap failed, fallback to PTY, domain=%s, err=%v", domain, rdapErr)
		}
	}

	// .bz、.ag 等 Afilias TLD：默认 whois 只返回 IANA，需直连 whois.afilias.net
	if isAfiliasTLD(domain) {
		log.Printf("[WHOIS] Afilias TLD (.bz etc.) detected, trying QueryWithAfiliasServer first, domain=%s", domain)
		afiliasResult, afiliasErr := QueryWithAfiliasServer(domain)
		if afiliasErr == nil && afiliasResult != nil && afiliasResult.WhoisRaw != "" && !IsOnlyIANAInfo(afiliasResult.WhoisRaw) {
			log.Printf("[WHOIS] QueryWithAfiliasServer success, domain=%s, status=%s", domain, afiliasResult.Status)
			return afiliasResult, nil
		}
		if afiliasErr != nil {
			log.Printf("[WHOIS] QueryWithAfiliasServer failed, fallback to PTY, domain=%s, err=%v", domain, afiliasErr)
		}
	}

	// .in（印度）：直连 whois.nixiregistry.in 可稳定拿到状态（含 restricted by registry policy）；限制注册类无到期日
	if isNIXITLD(domain) {
		log.Printf("[WHOIS] NIXI TLD (.in) detected, trying QueryWithNIXIServer first, domain=%s", domain)
		nixiResult, nixiErr := QueryWithNIXIServer(domain)
		if nixiErr == nil && nixiResult != nil && nixiResult.WhoisRaw != "" {
			log.Printf("[WHOIS] QueryWithNIXIServer success, domain=%s, status=%s", domain, nixiResult.Status)
			return nixiResult, nil
		}
		if nixiErr != nil {
			log.Printf("[WHOIS] QueryWithNIXIServer failed, fallback to PTY, domain=%s, err=%v", domain, nixiErr)
		}
	}

	// 统一使用 QueryWithPTY
	log.Printf("[WHOIS] Using QueryWithPTY, domain=%s", domain)
	result, err := QueryWithPTY(domain, 10*time.Second)
	if err == nil && result != nil && result.WhoisRaw != "" && !IsOnlyIANAInfo(result.WhoisRaw) {
		log.Printf("[WHOIS] QueryWithPTY success, domain=%s, rawLen=%d", domain, len(result.WhoisRaw))
		return result, nil
	}
	if err != nil {
		log.Printf("[WHOIS] QueryWithPTY failed, domain=%s, err=%v", domain, err)
	}
	// PTY 仅返回 IANA 或失败时（如 .app 等仅 RDAP 的 TLD），尝试 RDAP 引导查询
	if err != nil || (result != nil && IsOnlyIANAInfo(result.WhoisRaw)) {
		log.Printf("[WHOIS] Trying RDAP bootstrap (rdap.org), domain=%s", domain)
		rdapResult, rdapErr := QueryRDAPBootstrap(domain)
		if rdapErr == nil && rdapResult != nil && rdapResult.WhoisRaw != "" {
			log.Printf("[WHOIS] QueryRDAPBootstrap success, domain=%s, status=%s", domain, rdapResult.Status)
			return rdapResult, nil
		}
		if rdapErr != nil {
			log.Printf("[WHOIS] QueryRDAPBootstrap failed, domain=%s, err=%v", domain, rdapErr)
		}
	}
	if result == nil {
		result = &Result{
			Domain: domain,
			Status: "unknown",
		}
	}
	return result, err
}

// IsOnlyIANAInfo 检查 WHOIS 原始数据是否只包含 IANA 的 TLD 信息，没有实际域名信息
func IsOnlyIANAInfo(raw string) bool {
	if raw == "" {
		return false
	}

	// 检查是否包含 IANA WHOIS server 标记
	if !strings.Contains(raw, "IANA WHOIS server") {
		return false
	}

	// 检查是否包含实际域名信息的关键字段
	hasDomainInfo := strings.Contains(raw, "Domain Name:") ||
		strings.Contains(raw, "Registry Expiry Date:") ||
		strings.Contains(raw, "Registry Expires On:") ||
		strings.Contains(raw, "Expiry Date:") ||
		strings.Contains(raw, "Expires On:") ||
		strings.Contains(raw, "Registrar:") ||
		strings.Contains(raw, "Sponsoring Registrar:")

	// 如果包含 IANA 标记但没有实际域名信息，说明只返回了 IANA 信息
	return !hasDomainInfo
}

// QueryWithInfoServer 直接查询 .info 域名的 WHOIS 服务器
func QueryWithInfoServer(domain string) (*Result, error) {
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	// .info 域名的 WHOIS 服务器
	whoisServers := []string{
		"whois.afilias.net",
		"whois.identity.digital",
	}

	for _, server := range whoisServers {
		log.Printf("[WHOIS][InfoServer] Trying server=%s, domain=%s", server, domain)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		cmd := exec.CommandContext(ctx, "whois", "-h", server, domain)
		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			log.Printf("[WHOIS][InfoServer] Query failed, server=%s, domain=%s, err=%v, outputLen=%d",
				server, domain, err, len(output))
			continue
		}

		if len(output) == 0 {
			log.Printf("[WHOIS][InfoServer] Empty output, server=%s, domain=%s", server, domain)
			continue
		}

		raw := string(output)
		log.Printf("[WHOIS][InfoServer] Got response, server=%s, domain=%s, rawLen=%d, preview=%s",
			server, domain, len(raw), getPreview(raw, 200))

		// 检查是否包含实际域名信息（不是只有 IANA 信息）
		hasDomainInfo := strings.Contains(raw, "Domain Name:") ||
			strings.Contains(raw, "Registry Expiry Date:") ||
			strings.Contains(raw, "Registry Expires On:") ||
			strings.Contains(raw, "Expiry Date:")

		if hasDomainInfo {
			log.Printf("[WHOIS][InfoServer] Found domain info, server=%s, domain=%s", server, domain)
			raw = normalizeWhoisRaw(raw)
			result.WhoisRaw = raw
			parsed, err := parseWhoisData(result, raw)
			if err == nil {
				log.Printf("[WHOIS][InfoServer] Parse success, server=%s, domain=%s, status=%s, registrar=%s, expiryDate=%v",
					server, domain, parsed.Status, parsed.Registrar, parsed.ExpiryDate != nil)
			}
			return parsed, err
		} else {
			log.Printf("[WHOIS][InfoServer] No domain info found (IANA only?), server=%s, domain=%s", server, domain)
		}
	}

	log.Printf("[WHOIS][InfoServer] All servers failed, domain=%s", domain)
	return result, fmt.Errorf("failed to query .info domain from all servers")
}

// QueryWithIdentityDigitalServer 直接查询 Identity Digital 管理的 TLD（.money, .finance, .capital, .live 等）的 WHOIS 服务器
func QueryWithIdentityDigitalServer(domain string) (*Result, error) {
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	// Identity Digital 的 WHOIS 服务器
	whoisServers := []string{
		"whois.identity.digital",
		"whois.nic.money",   // .money 专用服务器
		"whois.nic.finance", // .finance 专用服务器
		"whois.nic.capital", // .capital 专用服务器
		"whois.nic.live",    // .live 专用服务器
	}

	for _, server := range whoisServers {
		log.Printf("[WHOIS][IdentityDigital] Trying server=%s, domain=%s", server, domain)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		cmd := exec.CommandContext(ctx, "whois", "-h", server, domain)
		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			log.Printf("[WHOIS][IdentityDigital] Query failed, server=%s, domain=%s, err=%v, outputLen=%d",
				server, domain, err, len(output))
			continue
		}

		if len(output) == 0 {
			log.Printf("[WHOIS][IdentityDigital] Empty output, server=%s, domain=%s", server, domain)
			continue
		}

		raw := string(output)
		log.Printf("[WHOIS][IdentityDigital] Got response, server=%s, domain=%s, rawLen=%d, preview=%s",
			server, domain, len(raw), getPreview(raw, 200))

		// 检查是否包含实际域名信息（不是只有 IANA 信息）
		hasDomainInfo := strings.Contains(raw, "Domain Name:") ||
			strings.Contains(raw, "Registry Expiry Date:") ||
			strings.Contains(raw, "Registry Expires On:") ||
			strings.Contains(raw, "Expiry Date:") ||
			strings.Contains(raw, "Expires On:") ||
			strings.Contains(raw, "Registrar:") ||
			strings.Contains(raw, "Sponsoring Registrar:") ||
			strings.Contains(raw, "Domain Status:") ||
			strings.Contains(raw, "Status:")

		if hasDomainInfo {
			log.Printf("[WHOIS][IdentityDigital] Found domain info, server=%s, domain=%s", server, domain)
			raw = normalizeWhoisRaw(raw)
			result.WhoisRaw = raw
			parsed, err := parseWhoisData(result, raw)
			if err == nil {
				log.Printf("[WHOIS][IdentityDigital] Parse success, server=%s, domain=%s, status=%s, registrar=%s, expiryDate=%v",
					server, domain, parsed.Status, parsed.Registrar, parsed.ExpiryDate != nil)
			}
			return parsed, err
		} else {
			log.Printf("[WHOIS][IdentityDigital] No domain info found (IANA only?), server=%s, domain=%s", server, domain)
		}
	}

	log.Printf("[WHOIS][IdentityDigital] All servers failed, domain=%s", domain)
	return result, fmt.Errorf("failed to query Identity Digital domain from all servers")
}

// QueryWithAfiliasServer 直接查询 Afilias 管理的 TLD（.bz、.ag 等），默认 whois 只返回 IANA
func QueryWithAfiliasServer(domain string) (*Result, error) {
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}
	server := "whois.afilias.net"
	log.Printf("[WHOIS][Afilias] Trying server=%s, domain=%s", server, domain)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	cmd := exec.CommandContext(ctx, "whois", "-h", server, domain)
	output, err := cmd.CombinedOutput()
	cancel()
	if err != nil {
		log.Printf("[WHOIS][Afilias] Query failed, domain=%s, err=%v, outputLen=%d", domain, err, len(output))
		return result, fmt.Errorf("whois afilias: %w", err)
	}
	raw := string(output)
	if len(raw) == 0 {
		return result, fmt.Errorf("whois afilias: empty output")
	}
	hasDomainInfo := strings.Contains(raw, "Domain Name:") ||
		strings.Contains(raw, "Registry Expiry Date:") ||
		strings.Contains(raw, "Registry Expires On:") ||
		strings.Contains(raw, "Expiry Date:") ||
		strings.Contains(raw, "Registrar:")
	if !hasDomainInfo {
		log.Printf("[WHOIS][Afilias] No domain info in response, domain=%s", domain)
		return result, fmt.Errorf("whois afilias: no domain info")
	}
	raw = normalizeWhoisRaw(raw)
	result.WhoisRaw = raw
	parsed, err := parseWhoisData(result, raw)
	if err == nil {
		log.Printf("[WHOIS][Afilias] Parse success, domain=%s, status=%s, registrar=%s", domain, parsed.Status, parsed.Registrar)
	}
	return parsed, err
}

// QueryWithNIXIServer 直接查询 NIXI 管理的 .in 域名（whois.nixiregistry.in），稳定返回状态；限制注册类无到期日
func QueryWithNIXIServer(domain string) (*Result, error) {
	result := &Result{
		Domain: domain,
		Status: StatusUnknown,
	}
	server := "whois.nixiregistry.in"
	log.Printf("[WHOIS][NIXI] Trying server=%s, domain=%s", server, domain)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	cmd := exec.CommandContext(ctx, "whois", "-h", server, domain)
	output, err := cmd.CombinedOutput()
	cancel()
	if err != nil {
		log.Printf("[WHOIS][NIXI] Query failed, domain=%s, err=%v, outputLen=%d", domain, err, len(output))
		return result, fmt.Errorf("whois nixi: %w", err)
	}
	raw := string(output)
	if len(raw) == 0 {
		return result, fmt.Errorf("whois nixi: empty output")
	}
	hasDomainInfo := strings.Contains(raw, "Domain Name:") ||
		strings.Contains(raw, "restricted by registry policy") ||
		strings.Contains(raw, "not available for registration")
	if !hasDomainInfo {
		log.Printf("[WHOIS][NIXI] No domain info in response, domain=%s", domain)
		return result, fmt.Errorf("whois nixi: no domain info")
	}
	raw = normalizeWhoisRaw(raw)
	result.WhoisRaw = raw
	parsed, err := parseWhoisData(result, raw)
	if err == nil {
		log.Printf("[WHOIS][NIXI] Parse success, domain=%s, status=%s, hasExpiry=%v", domain, parsed.Status, parsed.ExpiryDate != nil)
	}
	return parsed, err
}

// QueryWithVerisignServer 直接查询 .com 和 .net 域名的 VeriSign WHOIS 服务器
// 避免通过 IANA 重定向，提高查询速度和成功率
func QueryWithVerisignServer(domain string) (*Result, error) {
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	// VeriSign 的 WHOIS 服务器（管理 .com 和 .net）
	whoisServers := []string{
		"whois.verisign-grs.com",
		"whois.verisign.com",
	}

	for _, server := range whoisServers {
		log.Printf("[WHOIS][Verisign] Trying server=%s, domain=%s", server, domain)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

		cmd := exec.CommandContext(ctx, "whois", "-h", server, domain)
		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			log.Printf("[WHOIS][Verisign] Query failed, server=%s, domain=%s, err=%v, outputLen=%d",
				server, domain, err, len(output))
			continue
		}

		if len(output) == 0 {
			log.Printf("[WHOIS][Verisign] Empty output, server=%s, domain=%s", server, domain)
			continue
		}

		raw := string(output)
		log.Printf("[WHOIS][Verisign] Got response, server=%s, domain=%s, rawLen=%d, preview=%s",
			server, domain, len(raw), getPreview(raw, 200))

		// 检查是否包含实际域名信息（不是只有 IANA 信息）
		hasDomainInfo := strings.Contains(raw, "Domain Name:") ||
			strings.Contains(raw, "Registry Expiry Date:") ||
			strings.Contains(raw, "Registry Expires On:") ||
			strings.Contains(raw, "Expiry Date:")

		if hasDomainInfo {
			log.Printf("[WHOIS][Verisign] Found domain info, server=%s, domain=%s", server, domain)
			raw = normalizeWhoisRaw(raw)
			result.WhoisRaw = raw
			parsed, err := parseWhoisData(result, raw)
			if err == nil {
				log.Printf("[WHOIS][Verisign] Parse success, server=%s, domain=%s, status=%s, registrar=%s, expiryDate=%v",
					server, domain, parsed.Status, parsed.Registrar, parsed.ExpiryDate != nil)
			}
			return parsed, err
		} else {
			log.Printf("[WHOIS][Verisign] No domain info found (IANA only?), server=%s, domain=%s", server, domain)
		}
	}

	log.Printf("[WHOIS][Verisign] All servers failed, domain=%s", domain)
	return result, fmt.Errorf("failed to query .com/.net domain from VeriSign servers")
}

// getPreview 获取字符串的前 N 个字符，用于日志预览
func getPreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func QueryWithTimeout(domain string, timeout time.Duration) (*Result, error) {
	log.Printf("[WHOIS][Timeout] Start, domain=%s, timeout=%v", domain, timeout)
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	cmd := exec.Command("whois", domain)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[WHOIS][Timeout] Create stdout pipe failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("create stdout pipe failed: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[WHOIS][Timeout] Create stderr pipe failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("create stderr pipe failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[WHOIS][Timeout] Start command failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("start whois command failed: %w", err)
	}

	// 实时读取输出，并在获取到足够信息后提前终止
	var output bytes.Buffer
	var wg sync.WaitGroup
	var mu sync.Mutex
	var hasEnoughInfo atomic.Bool // 标记是否已获取到足够信息

	wg.Add(2)

	// 检查是否已获取到足够信息的函数
	checkEnoughInfo := func(line string) bool {
		// 检查是否包含关键字段
		hasExpiryDate := strings.Contains(line, "Registry Expiry Date:") ||
			strings.Contains(line, "Expiry Date:") ||
			strings.Contains(line, "Expire Date:") ||
			strings.Contains(line, "Expiration Date:")
		hasDomainName := strings.Contains(line, "Domain Name:")
		hasRegistrar := strings.Contains(line, "Registrar:")

		// 如果同时有域名和到期日期，或者有域名和注册商，说明信息足够
		if hasDomainName && (hasExpiryDate || hasRegistrar) {
			return true
		}

		// 如果已经读取到 VeriSign 的域名信息部分（以 # whois.verisign-grs.com 为标记）
		// 并且有到期日期，说明信息足够
		if strings.Contains(line, "# whois.verisign-grs.com") ||
			strings.Contains(line, "# whois.iana.org") {
			mu.Lock()
			currentOutput := output.String()
			mu.Unlock()
			if hasExpiryDate && strings.Contains(currentOutput, "Domain Name:") {
				return true
			}
		}

		// 如果检测到开始查询 Registrar WHOIS Server（如 # whois.godaddy.com），
		// 说明已经获取到主信息，可以提前终止
		if strings.HasPrefix(strings.TrimSpace(line), "# whois.") &&
			!strings.Contains(line, "verisign-grs.com") &&
			!strings.Contains(line, "iana.org") {
			mu.Lock()
			currentOutput := output.String()
			mu.Unlock()
			// 如果已经包含域名和到期日期，说明信息足够
			if strings.Contains(currentOutput, "Domain Name:") &&
				(strings.Contains(currentOutput, "Registry Expiry Date:") ||
					strings.Contains(currentOutput, "Expiry Date:")) {
				return true
			}
		}

		return false
	}

	// 读取stdout
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			output.WriteString(line)
			output.WriteString("\n")
			mu.Unlock()

			// 检查是否已获取到足够信息
			if checkEnoughInfo(line) && !hasEnoughInfo.Load() {
				hasEnoughInfo.Store(true)
				// 等待一小段时间，确保关键信息都已读取
				time.Sleep(200 * time.Millisecond)
				// 终止命令进程
				if cmd.Process != nil {
					cmd.Process.Kill()
				}
				log.Printf("[WHOIS][Timeout] Got enough info, terminating early, domain=%s", domain)
				break
			}
		}
	}()

	// 读取stderr
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			mu.Lock()
			output.WriteString(line)
			output.WriteString("\n")
			mu.Unlock()
		}
	}()

	// 等待命令完成或超时
	done := make(chan error, 1)
	go func() {
		wg.Wait() // 等待输出读取完成
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// 命令正常结束
		mu.Lock()
		raw := output.String()
		mu.Unlock()
		log.Printf("[WHOIS][Timeout] Command completed, domain=%s, rawLen=%d, preview=%s",
			domain, len(raw), getPreview(raw, 200))
		raw = normalizeWhoisRaw(raw)
		result.WhoisRaw = raw
		parsed, err := parseWhoisData(result, raw)
		if err == nil {
			log.Printf("[WHOIS][Timeout] Parse success, domain=%s, status=%s, registrar=%s, expiryDate=%v",
				domain, parsed.Status, parsed.Registrar, parsed.ExpiryDate != nil)
		} else {
			log.Printf("[WHOIS][Timeout] Parse failed, domain=%s, err=%v", domain, err)
		}
		return parsed, err

	case <-time.After(timeout):
		// 超时，但尝试使用已获取的数据
		mu.Lock()
		raw := output.String()
		mu.Unlock()
		log.Printf("[WHOIS][Timeout] Timeout, domain=%s, rawLen=%d, preview=%s",
			domain, len(raw), getPreview(raw, 200))

		// 杀掉进程（如果还在运行）
		if cmd.Process != nil {
			cmd.Process.Kill()
		}

		// 等待一下，让输出读取完成
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		raw = output.String() // 再次获取，可能已经读取到更多数据
		mu.Unlock()

		raw = normalizeWhoisRaw(raw)
		result.WhoisRaw = raw

		// 如果获取到足够的数据（包含关键字段），就使用它
		if len(raw) > 500 && (strings.Contains(raw, "Registry Expiry Date:") ||
			strings.Contains(raw, "Expiry Date:") ||
			strings.Contains(raw, "Expire Date:") || // .it 域名使用此格式
			strings.Contains(raw, "Expiration Date:") ||
			strings.Contains(raw, "Domain Name:") ||
			strings.Contains(raw, "Registrar:")) {
			parsed, err := parseWhoisData(result, raw)
			if err == nil {
				log.Printf("[WHOIS][Timeout] Parse success after timeout, domain=%s, status=%s, rawLen=%d",
					domain, parsed.Status, len(raw))
			}
			return parsed, err
		}

		if len(raw) > 100 && !containsNotFound(raw) {
			parsed, err := parseWhoisData(result, raw)
			if err == nil {
				log.Printf("[WHOIS][Timeout] Parse success (small data), domain=%s, status=%s, rawLen=%d",
					domain, parsed.Status, len(raw))
			}
			return parsed, err
		}

		log.Printf("[WHOIS][Timeout] Timeout with insufficient data, domain=%s, rawLen=%d", domain, len(raw))
		return result, fmt.Errorf("whois query timeout after %v (got %d bytes)", timeout, len(raw))
	}
}

// 最大保存的WHOIS原始长度，防止异常循环输出撑爆数据库
const maxWhoisRawLen = 20000

// normalizeWhoisRaw 对原始WHOIS文本做裁剪与截断，避免无限循环输出
func normalizeWhoisRaw(raw string) string {
	// 优先尝试保留从第一段 "Domain Name:" 开始到第一次 ">>> Last update of WHOIS database" 之间
	start := strings.Index(raw, "Domain Name:")
	if start >= 0 {
		endMarker := ">>> Last update of WHOIS database"
		rel := raw[start:]
		if end := strings.Index(rel, endMarker); end > 0 {
			raw = rel[:end+len(endMarker)]
		} else {
			raw = rel
		}
	} else {
		// 如果没有找到 "Domain Name:"，尝试查找实际的域名信息部分
		// 某些 TLD（如 .info）的 WHOIS 输出可能包含 IANA 信息和实际域名信息两部分
		// 查找包含实际域名信息的模式（通常在 IANA 信息之后）

		// 方法1: 查找第二个 "domain:"（第一个通常是 TLD 信息，第二个是实际域名）
		firstDomainIdx := strings.Index(raw, "domain:")
		if firstDomainIdx >= 0 {
			secondDomainIdx := strings.Index(raw[firstDomainIdx+7:], "domain:")
			if secondDomainIdx >= 0 {
				// 找到第二个 domain:，向前查找这个块的开始
				actualStart := firstDomainIdx + 7 + secondDomainIdx
				// 向前查找最近的空行或主要字段作为块开始
				for i := actualStart; i > 0 && i > actualStart-500; i-- {
					if i > 1 && raw[i-1] == '\n' && raw[i-2] == '\n' {
						start = i
						break
					}
				}
				if start < 0 {
					start = actualStart
				}
			}
		}

		// 方法2: 如果还是没找到，查找包含到期日期等关键字段的部分
		// 这通常表示实际的域名信息（而不是 IANA 的 TLD 信息）
		if start < 0 {
			expiryPatterns := []string{
				"Registry Expiry Date:",
				"Registry Expires On:",
				"Expiry Date:",
				"Expires On:",
				"Expiration Date:",
			}
			for _, pattern := range expiryPatterns {
				if idx := strings.Index(raw, pattern); idx >= 0 {
					// 向前查找 1000 个字符，找到这个块的开始
					blockStart := idx
					if blockStart > 1000 {
						blockStart = idx - 1000
					}
					// 查找最近的连续两个换行符（表示新块的开始）
					for i := blockStart; i < idx; i++ {
						if i > 1 && raw[i-1] == '\n' && raw[i-2] == '\n' {
							start = i
							break
						}
					}
					if start < 0 {
						start = blockStart
					}
					break
				}
			}
		}

		// 如果找到了实际域名信息的开始位置，提取该部分
		if start >= 0 {
			endMarker := ">>> Last update of WHOIS database"
			rel := raw[start:]
			if end := strings.Index(rel, endMarker); end > 0 {
				raw = rel[:end+len(endMarker)]
			} else {
				raw = rel
			}
		}
		// 如果都没找到，保留原始数据（但会在后面截断）
	}

	// 最终长度再做一次硬截断，保证不会超过数据库字段上限太多
	if len(raw) > maxWhoisRawLen {
		raw = raw[:maxWhoisRawLen]
	}
	return raw
}

func parseWhoisData(result *Result, raw string) (*Result, error) {
	// 先处理"限制注册"的情况（如 .in 域名的 "restricted by registry policy"）：单独状态，cron 可通过状态过滤掉
	if containsRestricted(raw) {
		log.Printf("[WHOIS][Parse] Domain is restricted (not available for registration), domain=%s", result.Domain)
		result.Status = StatusRestricted
		result.CanRegister = false
		result.WhoisStatus = "restricted by registry policy"
		// 仍尝试解析到期日（部分注册局在限制说明后仍有日期）
		if expiry := parseDate(raw, []string{"Registry Expiry Date:", "Expiry Date:", "Expires On:"}); expiry != nil {
			result.ExpiryDate = expiry
		}
		return result, nil
	}

	// 先处理"未注册"/"未找到"的情况
	if containsNotFound(raw) {
		domainLower := strings.ToLower(result.Domain)

		// 特殊情况：.om 这类后缀，注册局WHOIS经常返回 "No Data Found"，
		// 但在注册商侧依然不可注册（保留/政策限制），不能简单当成 available
		if strings.HasSuffix(domainLower, ".om") {
			// 标记为未知状态，不允许自动判定可注册，交给上层或人工判断
			result.Status = StatusUnknown
			result.CanRegister = false
			return result, nil
		}

		// 其他后缀仍按"未找到=可注册"处理
		result.Status = StatusAvailable
		result.CanRegister = true
		return result, nil
	}

	// 解析 Domain Status 字段（用于判断生命周期状态）
	domainStatus := parseDomainStatus(raw)
	result.WhoisStatus = domainStatus
	log.Printf("[WHOIS][Parse] Domain status parsed, domain=%s, status=%s", result.Domain, domainStatus)

	// Parse expiry date first (needed for lifecycle status validation)
	expiryDate := parseDate(raw, []string{
		"Registry Expiry Date:",
		"Registry Expires On:", // .info 等域名可能使用此格式
		"Expiry Date:",
		"Expire Date:", // .it 域名使用此格式
		"Expiration Date:",
		"Expires On:", // 部分 TLD 使用此格式
		"paid-till:",
		"Registrar Registration Expiration Date:",
		"expire:",
		"Expiry date:",
		"Expiration Time:",
		"Expires:", // 简化格式
	})
	if expiryDate != nil {
		result.ExpiryDate = expiryDate
		log.Printf("[WHOIS][Parse] Expiry date parsed, domain=%s, expiryDate=%s",
			result.Domain, expiryDate.Format("2006-01-02 15:04:05"))
	}

	// 根据 Domain Status 判断生命周期状态，但需要验证到期日期
	lifecycleStatus := determineLifecycleStatus(domainStatus)
	now := time.Now()

	// 如果从 Domain Status 判断出生命周期状态，需要验证到期日期
	if lifecycleStatus != "" {
		// 宽限期、赎回期、待删除期都应该是域名到期后的状态
		// 如果到期日期是未来的，不应该设置这些状态
		if lifecycleStatus == StatusGracePeriod || lifecycleStatus == StatusRedemption || lifecycleStatus == StatusPendingDelete {
			if expiryDate != nil && expiryDate.After(now) {
				// 到期日期是未来的，不应该是宽限期/赎回期/待删除期
				// 应该是已注册状态
				log.Printf("[WHOIS][Parse] Domain status '%s' ignored because expiry date is in future, domain=%s, expiryDate=%s",
					lifecycleStatus, result.Domain, expiryDate.Format("2006-01-02 15:04:05"))
				lifecycleStatus = "" // 清空，让后续逻辑根据到期日期判断
			} else if expiryDate == nil {
				// 没有到期日期，但 WHOIS 显示生命周期状态，可能是误判
				// 保守处理：如果到期日期未知，不设置生命周期状态
				log.Printf("[WHOIS][Parse] Domain status '%s' ignored because no expiry date, domain=%s",
					lifecycleStatus, result.Domain)
				lifecycleStatus = ""
			}
		}

		if lifecycleStatus != "" {
			result.Status = lifecycleStatus
			// pending_delete 和 redemption 状态下不能注册，但即将可以
			if lifecycleStatus == StatusPendingDelete || lifecycleStatus == StatusRedemption || lifecycleStatus == StatusGracePeriod {
				result.CanRegister = false
				log.Printf("[WHOIS][Parse] Domain in lifecycle transition, domain=%s, status=%s", result.Domain, lifecycleStatus)
			}
		}
	}

	// 只有在没有从 Domain Status 确定状态时，才根据到期日期判断
	if lifecycleStatus == "" {
		if expiryDate != nil {
			if expiryDate.Before(now) {
				result.Status = StatusExpired
				result.CanRegister = true
			} else {
				result.Status = StatusRegistered
				result.CanRegister = false
			}
		} else {
			// 没有到期日期，默认为已注册
			result.Status = StatusRegistered
			result.CanRegister = false
		}
	}

	// 计算预计删除日期（到期后约 75-80 天，这里用 75 天）
	if expiryDate != nil {
		if result.Status == StatusExpired || result.Status == StatusGracePeriod ||
			result.Status == StatusRedemption || result.Status == StatusPendingDelete {
			deleteDate := expiryDate.AddDate(0, 0, 75) // 到期后约75天删除
			result.DeleteDate = &deleteDate
			log.Printf("[WHOIS][Parse] Estimated delete date, domain=%s, deleteDate=%s",
				result.Domain, deleteDate.Format("2006-01-02"))
		}
	} else {
		log.Printf("[WHOIS][Parse] No expiry date found, domain=%s, rawPreview=%s",
			result.Domain, getPreview(raw, 500))
	}

	// Parse creation date
	result.CreationDate = parseDate(raw, []string{
		"Creation Date:",
		"Created Date:",
		"created:",
		"Registration Time:",
	})

	// Parse registrar
	result.Registrar = parseRegistrar(raw)

	return result, nil
}

// parseDomainStatus 解析 WHOIS 中的 Domain Status 字段
func parseDomainStatus(raw string) string {
	var statuses []string

	// Domain Status 可能有多行
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lowerLine := strings.ToLower(line)

		// 匹配 "Domain Status:" 或 "Status:" 开头的行
		if strings.HasPrefix(lowerLine, "domain status:") {
			value := strings.TrimSpace(line[len("Domain Status:"):])
			// 有些 WHOIS 返回格式是 "Domain Status: clientTransferProhibited https://..."
			// 只取第一个空格前的部分
			if spaceIdx := strings.Index(value, " "); spaceIdx > 0 {
				value = value[:spaceIdx]
			}
			if value != "" {
				statuses = append(statuses, value)
			}
		} else if strings.HasPrefix(lowerLine, "status:") && !strings.HasPrefix(lowerLine, "status: free") {
			value := strings.TrimSpace(line[len("Status:"):])
			if spaceIdx := strings.Index(value, " "); spaceIdx > 0 {
				value = value[:spaceIdx]
			}
			if value != "" {
				statuses = append(statuses, value)
			}
		}
	}

	return strings.Join(statuses, ", ")
}

// determineLifecycleStatus 根据 Domain Status 确定生命周期状态
func determineLifecycleStatus(domainStatus string) string {
	lower := strings.ToLower(domainStatus)

	// 按优先级检查状态（pendingDelete > redemption > gracePeriod）
	// pendingDelete - 待删除，即将释放
	if strings.Contains(lower, "pendingdelete") || strings.Contains(lower, "pending delete") ||
		strings.Contains(lower, "pending_delete") || strings.Contains(lower, "pendingremove") {
		return StatusPendingDelete
	}

	// redemptionPeriod - 赎回期
	if strings.Contains(lower, "redemptionperiod") || strings.Contains(lower, "redemption") ||
		strings.Contains(lower, "pendingrestorefp") || strings.Contains(lower, "pendingrestore") {
		return StatusRedemption
	}

	// autoRenewPeriod / gracePeriod - 宽限期
	if strings.Contains(lower, "autorenewperiod") || strings.Contains(lower, "graceperiod") ||
		strings.Contains(lower, "renewperiod") || strings.Contains(lower, "grace period") {
		return StatusGracePeriod
	}

	// 正常已注册状态不返回，由后续逻辑处理
	return ""
}

func containsRestricted(s string) bool {
	lower := strings.ToLower(s)
	restrictedPatterns := []string{
		"not available for registration",
		"restricted by registry policy",
		"restricted by policy",
		"registry policy",
		"reserved",
		"blocked",
	}
	for _, p := range restrictedPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func containsNotFound(s string) bool {
	lower := strings.ToLower(s)
	notFoundPatterns := []string{
		"no match for",
		"not found",
		"no entries found",
		"no data found",
		"status: free",
		"status: available",
		"is free",
		"no object found",
		"no matching record",
	}
	for _, p := range notFoundPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func parseDate(raw string, labels []string) *time.Time {
	for _, label := range labels {
		idx := strings.Index(raw, label)
		if idx < 0 {
			continue
		}
		line := raw[idx+len(label):]
		nlIdx := strings.Index(line, "\n")
		if nlIdx > 0 {
			line = line[:nlIdx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		log.Printf("[WHOIS][ParseDate] Found label=%s, value=%s", label, line)
		for _, layout := range dateLayouts {
			t, err := time.Parse(layout, line)
			if err == nil {
				log.Printf("[WHOIS][ParseDate] Success, label=%s, value=%s, parsed=%s, layout=%s",
					label, line, t.Format("2006-01-02 15:04:05"), layout)
				return &t
			}
		}
		log.Printf("[WHOIS][ParseDate] Failed to parse, label=%s, value=%s", label, line)
	}
	return nil
}

// parseRegistrar 解析注册商信息，支持多种格式
func parseRegistrar(raw string) string {
	// 先尝试标准单行格式
	registrar := parseField(raw, []string{
		"Registrar:",
		"Sponsoring Registrar:",
		"registrar:",
	})
	if registrar != "" {
		return registrar
	}

	// 尝试 .it 域名的多行块格式：
	// Registrar
	//   Organization:     MarkMonitor International Limited
	//   Name:             MARKMONITOR-REG
	//   ...
	// 查找 Registrar 块的位置（可能在行首或换行后）
	registrarBlockStart := -1
	if strings.HasPrefix(raw, "Registrar\n") {
		registrarBlockStart = len("Registrar\n")
	} else if strings.HasPrefix(raw, "Registrar\r\n") {
		registrarBlockStart = len("Registrar\r\n")
	} else {
		patterns := []string{"\nRegistrar\n", "\r\nRegistrar\r\n", "\nRegistrar\r\n", "\r\nRegistrar\n"}
		for _, pattern := range patterns {
			if pos := strings.Index(raw, pattern); pos >= 0 {
				registrarBlockStart = pos + len(pattern)
				break
			}
		}
	}

	if registrarBlockStart >= 0 {
		// 从 Registrar 块开始，查找下一个主要块（不以空格/制表符开头的非空行）作为结束位置
		remaining := raw[registrarBlockStart:]
		lines := strings.Split(remaining, "\n")

		blockEnd := len(raw)
		for i := 0; i < len(lines); i++ {
			line := lines[i]
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			// 如果遇到新的主要块（不以空格/制表符开头），结束 Registrar 块
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				// 计算结束位置
				pos := registrarBlockStart
				for j := 0; j < i; j++ {
					pos += len(lines[j]) + 1 // +1 for \n
				}
				blockEnd = pos
				break
			}
		}

		registrarBlock := raw[registrarBlockStart:blockEnd]

		// 在块中查找 Organization: 字段（可能带前导空格）
		org := parseField(registrarBlock, []string{
			"Organization:",
			"  Organization:",
			"\tOrganization:",
		})
		if org != "" {
			return org
		}

		// 如果没有 Organization，尝试 Name 字段
		name := parseField(registrarBlock, []string{
			"Name:",
			"  Name:",
			"\tName:",
		})
		if name != "" {
			return name
		}
	}

	return ""
}

func parseField(raw string, labels []string) string {
	for _, label := range labels {
		idx := strings.Index(raw, label)
		if idx < 0 {
			continue
		}
		line := raw[idx+len(label):]
		nlIdx := strings.Index(line, "\n")
		if nlIdx > 0 {
			line = line[:nlIdx]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"02-Jan-2006",
	"02/01/2006",
	"01/02/2006",
	"2006/01/02",
	"2006.01.02",
	"20060102",
	"January 02 2006",
	"02-January-2006",
}
