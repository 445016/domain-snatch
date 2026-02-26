package whois

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

// QueryWithPTY 使用伪终端执行whois，可以实时捕获输出
func QueryWithPTY(domain string, timeout time.Duration) (*Result, error) {
	log.Printf("[WHOIS][PTY] Start, domain=%s, timeout=%v", domain, timeout)
	result := &Result{
		Domain: domain,
		Status: "unknown",
	}

	cmd := exec.Command("whois", domain)
	
	// 使用PTY启动命令（模拟真实终端）
	ptmx, err := pty.Start(cmd)
	if err != nil {
		log.Printf("[WHOIS][PTY] Start failed, domain=%s, err=%v", domain, err)
		return result, fmt.Errorf("start pty failed: %w", err)
	}
	defer ptmx.Close()

	// 实时读取输出
	var output bytes.Buffer
	done := make(chan error, 1)

	go func() {
		_, err := io.Copy(&output, ptmx)
		done <- err
	}()

	// 等待完成或超时
	select {
	case <-done:
		// 命令正常结束
		raw := output.String()
		log.Printf("[WHOIS][PTY] Command completed, domain=%s, rawLen=%d, preview=%s", 
			domain, len(raw), getPreview(raw, 200))
		raw = normalizeWhoisRaw(raw)
		result.WhoisRaw = raw
		parsed, err := parseWhoisData(result, raw)
		if err == nil {
			log.Printf("[WHOIS][PTY] Parse success, domain=%s, status=%s, registrar=%s, expiryDate=%v", 
				domain, parsed.Status, parsed.Registrar, parsed.ExpiryDate != nil)
		} else {
			log.Printf("[WHOIS][PTY] Parse failed, domain=%s, err=%v", domain, err)
		}
		return parsed, err

	case <-time.After(timeout):
		// 超时，获取已读取的数据
		raw := output.String()
		log.Printf("[WHOIS][PTY] Timeout, domain=%s, rawLen=%d, preview=%s", 
			domain, len(raw), getPreview(raw, 200))
		raw = normalizeWhoisRaw(raw)
		result.WhoisRaw = raw

		// 关闭PTY会导致命令终止
		ptmx.Close()
		cmd.Process.Kill()
		
		// 如果数据足够，就解析它
		if len(raw) > 500 && containsValidWhoisData(raw) {
			parsed, err := parseWhoisData(result, raw)
			if err == nil && parsed.Status != "unknown" {
				log.Printf("[WHOIS][PTY] Parse success after timeout, domain=%s, status=%s", domain, parsed.Status)
				return parsed, nil
			}
		}
		
		if len(raw) > 0 {
			log.Printf("[WHOIS][PTY] Timeout with data, domain=%s, rawLen=%d", domain, len(raw))
			return result, fmt.Errorf("whois timeout but got %d bytes of data", len(raw))
		}
		
		log.Printf("[WHOIS][PTY] Timeout with no data, domain=%s", domain)
		return result, fmt.Errorf("whois query timeout after %v", timeout)
	}
}

// containsValidWhoisData 检查数据是否包含有效的WHOIS信息
func containsValidWhoisData(raw string) bool {
	return strings.Contains(raw, "Registry Expiry Date:") ||
		strings.Contains(raw, "Expiry Date:") ||
		strings.Contains(raw, "Expire Date:") || // .it 域名使用此格式
		strings.Contains(raw, "Expiration Date:") ||
		strings.Contains(raw, "Domain Name:")
}
