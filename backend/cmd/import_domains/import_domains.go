package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"domain-snatch/model"
	"domain-snatch/pkg/configutil"
	"domain-snatch/pkg/excel"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var (
	configFile = flag.String("f", "api/etc/domain.yaml", "项目配置文件路径，用于读取数据库连接")
	fileFlag   = flag.String("file", "", "要导入的文件路径（支持 .xlsx, .xls, .txt, .csv）")
)

func main() {
	flag.Parse()

	if *fileFlag == "" {
		fmt.Println("用法: go run ./cmd/import_domains -file=path/to/file")
		fmt.Println("\n支持的文件格式:")
		fmt.Println("  - Excel: .xlsx, .xls（与 API 导入使用相同解析）")
		fmt.Println("  - 文本: .txt, .csv（每行一个 URL/域名/邮箱，与 Excel 相同清洗与校验）")
		fmt.Println("\n选项:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ext := strings.ToLower(filepath.Ext(*fileFlag))
	var domains []string
	var err error

	fmt.Printf("正在读取文件: %s\n", *fileFlag)

	switch ext {
	case ".xlsx", ".xls":
		domains, err = excel.ParseDomainsFromFile(*fileFlag)
		if err != nil {
			log.Fatalf("解析 Excel 失败: %v", err)
		}
		fmt.Printf("从 Excel 中提取到 %d 个唯一域名\n", len(domains))

	case ".txt", ".csv", "":
		file, errOpen := os.Open(*fileFlag)
		if errOpen != nil {
			log.Fatalf("打开文件失败: %v", errOpen)
		}
		defer file.Close()
		domains, err = excel.ParseDomainsFromText(file)
		if err != nil {
			log.Fatalf("解析文本失败: %v", err)
		}
		fmt.Printf("从文本中提取到 %d 个唯一域名\n", len(domains))

	default:
		log.Fatalf("不支持的文件格式: %s（支持 .xlsx, .xls, .txt, .csv）", ext)
	}

	if len(domains) == 0 {
		fmt.Println("文件中没有找到有效域名")
		return
	}

	configPath := configutil.ResolveConfigPath(*configFile, "api/etc/domain.yaml")
	dataSource, err := configutil.LoadDataSource(configPath)
	if err != nil {
		log.Fatal(err)
	}
	conn := sqlx.NewMysql(dataSource)
	domainsModel := model.NewDomainsModel(conn)
	ctx := context.Background()

	// 与 API 一致：逐条 FindOneByDomain，存在则算 failed；否则 Insert，失败也算 failed
	var success, failed int64
	for _, domain := range domains {
		_, err := domainsModel.FindOneByDomain(ctx, domain)
		if err == nil {
			failed++
			continue
		}
		_, err = domainsModel.Insert(ctx, &model.Domains{
			Domain:  domain,
			Status:  "unknown",
			Monitor: 0,
		})
		if err != nil {
			failed++
			continue
		}
		success++
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("导入完成（与 API /import 逻辑一致）\n")
	fmt.Printf("========================================\n")
	fmt.Printf("解析总数: %d\n", len(domains))
	fmt.Printf("成功: %d\n", success)
	fmt.Printf("未入库: %d（已存在或写入失败）\n", failed)
}
