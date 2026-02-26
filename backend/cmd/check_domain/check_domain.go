package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"domain-snatch/model"
	"domain-snatch/pkg/configutil"
	"domain-snatch/pkg/whois"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var (
	configFile = flag.String("f", "api/etc/domain.yaml", "项目配置文件路径，用于读取数据库连接")
	domainFlag = flag.String("domain", "", "要检查的域名，例如: example.com（不指定则检查所有或按时间筛选）")
	timeFlag   = flag.String("time", "", "参考时间：与 cron 一致，只更新「当前状态结束时间≤该时间」的域名。留空或 now=当前时间；或 2006-01-02、2006-01-02 15:04:05。仅在不指定 -domain 时生效")
	sleepSec   = flag.Int("sleep", 2, "批量模式下，每个域名之间的休眠秒数")
	statusFlag = flag.String("status", "", "仅检查指定状态的域名（如: unknown, registered, expired），仅「全部域名」模式生效")
	orderDesc  = flag.Bool("desc", false, "倒序检查（从最新添加的域名开始），仅「全部域名」模式生效")
	limitFlag  = flag.Int64("limit", 0, "限制检查数量（0=不限制），仅「全部域名」模式生效")
)

func timeFlagPresent() bool {
	for _, arg := range os.Args[1:] {
		if arg == "-time" || strings.HasPrefix(arg, "-time=") {
			return true
		}
	}
	return false
}

func parseTimeRef(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "now") {
		return time.Now(), nil
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			if f == "2006-01-02" {
				return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q，请使用 now、2006-01-02 或 2006-01-02 15:04:05", s)
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: check_domain [选项]\n\n")
		fmt.Fprintf(os.Stderr, "三种模式:\n")
		fmt.Fprintf(os.Stderr, "  1) 某个域名: -domain=example.com\n")
		fmt.Fprintf(os.Stderr, "  2) 全部域名: 不指定 -domain 且不指定 -time（可配合 -status/-limit）\n")
		fmt.Fprintf(os.Stderr, "  3) 指定时间: 不指定 -domain 且指定 -time（参考时间，默认当前时间）。只更新「当前状态结束时间≤该时间」的域名（与 cron 一致）\n\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	dataSource, err := configutil.LoadDataSource(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	conn := sqlx.NewMysql(dataSource)
	domainsModel := model.NewDomainsModel(conn)
	ctx := context.Background()

	if *domainFlag != "" {
		checkSingleDomain(ctx, domainsModel, *domainFlag)
		return
	}
	if timeFlagPresent() {
		refTime, err := parseTimeRef(*timeFlag)
		if err != nil {
			log.Fatalf("解析 -time 失败: %v", err)
		}
		checkDomainsByTime(ctx, domainsModel, refTime, *sleepSec)
		return
	}
	checkAllDomains(ctx, domainsModel, *sleepSec, *statusFlag, *orderDesc, *limitFlag)
}

// applyWhoisResult 将 WHOIS 结果写入 domain 结构体（不写库），供单条与批量共用
func applyWhoisResult(domain *model.Domains, result *whois.Result, now time.Time) {
	domain.Status = result.Status
	domain.Registrar = result.Registrar
	domain.WhoisStatus = result.WhoisStatus
	domain.LastChecked = sql.NullTime{Time: now, Valid: true}
	if result.ExpiryDate != nil {
		domain.ExpiryDate = sql.NullTime{Time: *result.ExpiryDate, Valid: true}
	}
	if result.CreationDate != nil {
		domain.CreationDate = sql.NullTime{Time: *result.CreationDate, Valid: true}
	}
	if result.DeleteDate != nil {
		domain.DeleteDate = sql.NullTime{Time: *result.DeleteDate, Valid: true}
	}
	if result.WhoisRaw != "" {
		domain.WhoisRaw = sql.NullString{String: result.WhoisRaw, Valid: true}
	}
}

// checkSingleDomain 检查单个域名
func checkSingleDomain(ctx context.Context, domainsModel model.DomainsModel, domainName string) {
	fmt.Printf("正在查询域名: %s\n\n", domainName)

	domain, err := domainsModel.FindOneByDomain(ctx, domainName)
	if err != nil {
		log.Fatalf("域名不存在: %v", err)
	}

	now := time.Now()
	fmt.Printf("[DomainCheck] WHOIS query start, domain=%s\n", domain.Domain)

	result, err := whois.QueryWithRateLimit(domain.Domain)
	if err != nil {
		log.Printf("[DomainCheck] WHOIS query failed, domain=%s, err=%v\n", domain.Domain, err)
		domain.LastChecked = sql.NullTime{Time: now, Valid: true}
		if updateErr := domainsModel.Update(ctx, domain); updateErr != nil {
			log.Fatalf("[DomainCheck] update lastChecked failed after error, domain=%s, err=%v", domain.Domain, updateErr)
		}
		return
	}

	applyWhoisResult(domain, result, now)
	if result.ExpiryDate != nil {
		fmt.Printf("[DomainCheck] Parsed expiry date, domain=%s, expiryDate=%s\n",
			domain.Domain, result.ExpiryDate.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Printf("[DomainCheck] No expiry date parsed, domain=%s (keeping existing value)\n", domain.Domain)
	}
	if result.CreationDate != nil {
		fmt.Printf("[DomainCheck] Parsed creation date, domain=%s, creationDate=%s\n",
			domain.Domain, result.CreationDate.Format("2006-01-02 15:04:05"))
	}
	if result.WhoisStatus != "" {
		fmt.Printf("[DomainCheck] WHOIS Status: %s\n", result.WhoisStatus)
	}

	if err := domainsModel.Update(ctx, domain); err != nil {
		log.Fatalf("[DomainCheck] update domain failed, domain=%s, err=%v", domain.Domain, err)
	}

	fmt.Printf("[DomainCheck] WHOIS query success, domain=%s, status=%s, registrar=%s, canRegister=%v, expiryDate=%v\n",
		domain.Domain, result.Status, result.Registrar, result.CanRegister, result.ExpiryDate != nil)

	fmt.Println("\n=== 更新完成 ===")
	fmt.Printf("域名: %s\n", domain.Domain)
	fmt.Printf("状态: %s\n", domain.Status)
	fmt.Printf("注册商: %s\n", domain.Registrar)
	if domain.ExpiryDate.Valid {
		fmt.Printf("到期日期: %s\n", domain.ExpiryDate.Time.Format("2006-01-02 15:04:05"))
	}
	if domain.CreationDate.Valid {
		fmt.Printf("注册日期: %s\n", domain.CreationDate.Time.Format("2006-01-02 15:04:05"))
	}
}

// checkDomainsByTime 按参考时间筛选：只更新「当前状态结束时间 ≤ refTime」的域名（与 cron 逻辑一致）
func checkDomainsByTime(ctx context.Context, domainsModel model.DomainsModel, refTime time.Time, sleepSec int) {
	domains, err := domainsModel.FindDomainsWithExpiredOrNullExpiryAsOf(ctx, refTime)
	if err != nil {
		log.Fatalf("按时间查询域名失败: %v", err)
	}
	total := len(domains)
	if total == 0 {
		fmt.Printf("参考时间 %s 下没有需要更新的域名\n", refTime.Format("2006-01-02 15:04:05"))
		return
	}

	fmt.Printf("参考时间: %s\n", refTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("符合条件的域名数: %d（当前状态结束时间 ≤ 参考时间）\n", total)
	fmt.Printf("每个域名间隔约 %d 秒\n\n", sleepSec)

	var success, failed int64
	startTime := time.Now()

	for i, domain := range domains {
		now := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			fmt.Printf("  [%d/%d] ✗ %s: 查询失败 - %v\n", i+1, total, domain.Domain, err)
			domain.LastChecked = sql.NullTime{Time: now, Valid: true}
			if updateErr := domainsModel.Update(ctx, domain); updateErr != nil {
				fmt.Printf("    更新失败: %v\n", updateErr)
			}
			failed++
			time.Sleep(time.Duration(sleepSec) * time.Second)
			continue
		}

		applyWhoisResult(domain, result, now)
		if err := domainsModel.Update(ctx, domain); err != nil {
			fmt.Printf("  [%d/%d] ✗ %s: 保存失败 - %v\n", i+1, total, domain.Domain, err)
			failed++
		} else {
			expiryStr := "未知"
			if result.ExpiryDate != nil {
				expiryStr = result.ExpiryDate.Format("2006-01-02")
			}
			fmt.Printf("  [%d/%d] ✓ %s: %s (到期: %s, 注册商: %s)\n",
				i+1, total, domain.Domain, result.Status, expiryStr, result.Registrar)
			success++
		}

		if (i+1)%10 == 0 || i+1 == total {
			elapsed := time.Since(startTime)
			remaining := time.Duration(float64(elapsed) / float64(i+1) * float64(total-i-1))
			fmt.Printf("\n进度: %d/%d (%.1f%%) | 成功: %d | 失败: %d | 已用时: %s | 预计剩余: %s\n\n",
				i+1, total, float64(i+1)/float64(total)*100, success, failed,
				elapsed.Round(time.Second), remaining.Round(time.Second))
		}

		time.Sleep(time.Duration(sleepSec) * time.Second)
	}

	fmt.Println("\n=== 检查完成 ===")
	fmt.Printf("总计: %d 个域名\n", total)
	fmt.Printf("✓ 成功: %d 个\n", success)
	fmt.Printf("✗ 失败: %d 个\n", failed)
	fmt.Printf("总耗时: %s\n", time.Since(startTime).Round(time.Second))
}

// checkAllDomains 检查所有/指定状态的域名
func checkAllDomains(ctx context.Context, domainsModel model.DomainsModel, sleepSec int, statusFilter string, descOrder bool, limit int64) {
	// 排序方式
	sortOrder := "asc"
	if descOrder {
		sortOrder = "desc"
	}

	// 查询数量
	pageSize := int64(100000)
	if limit > 0 {
		pageSize = limit
	}

	// 查询域名（monitor=-1 表示不过滤监控状态）
	domains, err := domainsModel.FindList(ctx, 1, pageSize, "", statusFilter, -1, "", "", "id", sortOrder)
	if err != nil {
		log.Fatalf("查询域名列表失败: %v", err)
	}

	total := len(domains)
	if total == 0 {
		fmt.Println("没有找到符合条件的域名")
		return
	}

	// 打印查询条件
	filterDesc := "所有域名"
	if statusFilter != "" {
		filterDesc = fmt.Sprintf("状态为 %s 的域名", statusFilter)
	}

	orderDesc := ""
	if descOrder {
		orderDesc = "（倒序）"
	}

	fmt.Printf("开始批量检查 %d 个%s的WHOIS信息%s...\n", total, filterDesc, orderDesc)
	fmt.Println("注意：这个过程可能需要较长时间，系统会自动调整查询速度")
	fmt.Printf("每个域名间隔约 %d 秒，避免被限流\n\n", sleepSec)

	var success, failed int64
	startTime := time.Now()

	for i, domain := range domains {
		now := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			fmt.Printf("  [%d/%d] ✗ %s: 查询失败 - %v\n", i+1, total, domain.Domain, err)
			domain.LastChecked = sql.NullTime{Time: now, Valid: true}
			if updateErr := domainsModel.Update(ctx, domain); updateErr != nil {
				fmt.Printf("    更新失败: %v\n", updateErr)
			}
			failed++
			time.Sleep(time.Duration(sleepSec) * time.Second)
			continue
		}

		applyWhoisResult(domain, result, now)

		if err := domainsModel.Update(ctx, domain); err != nil {
			fmt.Printf("  [%d/%d] ✗ %s: 保存失败 - %v\n", i+1, total, domain.Domain, err)
			failed++
		} else {
			expiryStr := "未知"
			if result.ExpiryDate != nil {
				expiryStr = result.ExpiryDate.Format("2006-01-02")
			}
			fmt.Printf("  [%d/%d] ✓ %s: %s (到期: %s, 注册商: %s)\n",
				i+1, total, domain.Domain, result.Status, expiryStr, result.Registrar)
			success++
		}

		// 每10个或最后一个显示进度
		if (i+1)%10 == 0 || i+1 == total {
			elapsed := time.Since(startTime)
			remaining := time.Duration(float64(elapsed) / float64(i+1) * float64(total-i-1))
			fmt.Printf("\n进度: %d/%d (%.1f%%) | 成功: %d | 失败: %d | 已用时: %s | 预计剩余: %s\n\n",
				i+1, total, float64(i+1)/float64(total)*100, success, failed,
				elapsed.Round(time.Second), remaining.Round(time.Second))
		}

		time.Sleep(time.Duration(sleepSec) * time.Second)
	}

	fmt.Println("\n=== 检查完成 ===")
	fmt.Printf("总计: %d 个域名\n", total)
	fmt.Printf("✓ 成功: %d 个\n", success)
	fmt.Printf("✗ 失败: %d 个\n", failed)
	fmt.Printf("总耗时: %s\n", time.Since(startTime).Round(time.Second))
}
