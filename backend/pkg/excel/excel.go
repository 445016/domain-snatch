package excel

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

// 常见的二级TLD（需要特殊处理）
var secondLevelTLDs = map[string]bool{
	"co":   true,
	"com":  true,
	"org":  true,
	"net":  true,
	"gov":  true,
	"ac":   true,
	"edu":  true,
	"bank": true,
	"gen":  true,
	"firm": true,
	"ind":  true,
}

// 有效的TLD列表（扩展版）
var validTLDs = map[string]bool{
	"com": true, "net": true, "org": true, "cn": true, "io": true, "co": true,
	"info": true, "biz": true, "cc": true, "me": true, "tv": true, "xyz": true,
	"top": true, "tech": true, "online": true, "site": true, "club": true,
	"store": true, "app": true, "dev": true, "in": true, "bank": true,
	"credit": true, "money": true, "ai": true,
}

// ParseDomainsFromReader 从Excel文件读取器中解析域名列表（自动提取一级域名）
func ParseDomainsFromReader(reader io.Reader) ([]string, error) {
	f, err := excelize.OpenReader(reader)
	if err != nil {
		return nil, fmt.Errorf("open excel failed: %w", err)
	}
	defer f.Close()

	return extractDomains(f)
}

// ParseDomainsFromFile 从Excel文件路径中解析域名列表（自动提取一级域名）
func ParseDomainsFromFile(path string) ([]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open excel file failed: %w", err)
	}
	defer f.Close()

	return extractDomains(f)
}

// ParseDomainsFromText 从文本流中解析域名列表（每行一个 URL/域名/邮箱，与 Excel 解析使用相同的清洗与校验逻辑）
func ParseDomainsFromText(reader io.Reader) ([]string, error) {
	domainSet := make(map[string]bool)
	var domains []string

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.Contains(line, "@") {
			// 邮箱：取 @ 后部分
			parts := strings.SplitN(line, "@", 2)
			if len(parts) == 2 {
				d := cleanAndExtractRootDomain(parts[1])
				if d != "" && isValidDomain(d) && !domainSet[d] {
					domainSet[d] = true
					domains = append(domains, d)
				}
			}
		} else {
			domain := cleanAndExtractRootDomain(line)
			if domain != "" && isValidDomain(domain) && !domainSet[domain] {
				domainSet[domain] = true
				domains = append(domains, domain)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read text: %w", err)
	}
	return domains, nil
}

func extractDomains(f *excelize.File) ([]string, error) {
	domainSet := make(map[string]bool)
	var domains []string

	sheets := f.GetSheetList()
	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, cell := range row {
				cell = strings.TrimSpace(cell)
				if cell == "" {
					continue
				}

				var domain string

				// 尝试从邮箱中提取域名
				if strings.Contains(cell, "@") {
					parts := strings.Split(cell, ";")
					for _, part := range parts {
						part = strings.TrimSpace(part)
						if strings.Contains(part, "@") {
							d := strings.Split(part, "@")[1]
							d = cleanAndExtractRootDomain(d)
							if d != "" && isValidDomain(d) {
								if !domainSet[d] {
									domainSet[d] = true
									domains = append(domains, d)
								}
							}
						}
					}
				} else {
					// 尝试从URL或域名格式中提取
					domain = cleanAndExtractRootDomain(cell)
					if domain != "" && isValidDomain(domain) {
						if !domainSet[domain] {
							domainSet[domain] = true
							domains = append(domains, domain)
						}
					}
				}
			}
		}
	}

	return domains, nil
}

// cleanAndExtractRootDomain 清洗并提取一级域名
func cleanAndExtractRootDomain(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))

	// 尝试从括号中提取URL（处理 "Company Name (https://example.com)" 格式）
	if strings.Contains(s, "(http") {
		start := strings.Index(s, "(http")
		if start != -1 {
			end := strings.Index(s[start:], ")")
			if end != -1 {
				s = s[start+1 : start+end]
			} else {
				s = s[start+1:]
			}
		}
	}

	// 移除前导特殊字符
	s = strings.TrimLeft(s, "?!@#$%^&*")

	// 移除常见前缀
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")

	// 移除路径和查询参数
	if idx := strings.Index(s, "/"); idx > 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "?"); idx > 0 {
		s = s[:idx]
	}

	// 移除前后的特殊字符
	s = strings.Trim(s, "?!@#$%^&*()[]{}|\\:;\"'<>,\n\r\t ")

	// 移除 www. 前缀
	if strings.HasPrefix(s, "www.") {
		s = s[4:]
	}

	// 提取一级域名
	return extractRootDomain(s)
}

// extractRootDomain 提取一级域名
func extractRootDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return domain
	}

	// 检查是否是复合TLD（如 .co.in, .bank.in）
	if len(parts) >= 3 {
		// 倒数第二个部分是否是二级TLD
		secondLast := parts[len(parts)-2]
		if secondLevelTLDs[secondLast] {
			// 返回最后三部分作为根域名
			if len(parts) >= 3 {
				return strings.Join(parts[len(parts)-3:], ".")
			}
		}
	}

	// 标准情况：返回最后两部分
	return strings.Join(parts[len(parts)-2:], ".")
}

// isValidDomain 检查是否是有效的域名格式
func isValidDomain(domain string) bool {
	if domain == "" || len(domain) < 4 {
		return false
	}
	if strings.Contains(domain, " ") {
		return false
	}
	if !strings.Contains(domain, ".") {
		return false
	}

	// 域名正则：字母数字和连字符，以点分隔
	domainRegex := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	return domainRegex.MatchString(domain)
}

// isDomainLike 简单检测是否像域名（保留向后兼容）
func isDomainLike(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(s, " ") {
		return false
	}
	if !strings.Contains(s, ".") {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	tld := parts[len(parts)-1]
	return validTLDs[tld]
}
