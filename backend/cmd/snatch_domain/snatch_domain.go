package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"strings"

	"domain-snatch/model"
	"domain-snatch/pkg/configutil"
	"domain-snatch/pkg/godaddy"
	"domain-snatch/pkg/snatch"

	_ "github.com/go-sql-driver/mysql"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type config struct {
	Mysql struct {
		DataSource string
	}
	GoDaddy struct {
		APIKey    string
		APISecret string
		Sandbox   bool
		Enabled   bool
	}
	AutoSnatch struct {
		Enabled    bool
		MaxRetries int
		Contact    struct {
			FirstName    string
			LastName     string
			Email        string
			Phone        string
			Organization string
			Address1     string
			City         string
			State        string
			PostalCode   string
			Country      string
		}
	}
}

var (
	configFile = flag.String("f", "api/etc/domain.yaml", "项目配置文件路径")
	domainFlag = flag.String("domain", "", "要抢注的域名（与 -task-id 二选一）")
	taskIDFlag = flag.Int64("task-id", 0, "要执行的抢注任务 ID（与 -domain 二选一）")
)

func main() {
	flag.Parse()

	if *domainFlag == "" && *taskIDFlag <= 0 {
		log.Fatal("请指定 -domain=example.com 或 -task-id=123")
	}
	if *domainFlag != "" && *taskIDFlag > 0 {
		log.Fatal("-domain 与 -task-id 只能指定其一")
	}

	configPath := configutil.ResolveConfigPath(*configFile, "api/etc/domain.yaml")
	var cfg config
	if err := conf.Load(configPath, &cfg); err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if cfg.Mysql.DataSource == "" {
		log.Fatal("配置中 Mysql.DataSource 为空")
	}

	conn := sqlx.NewMysql(cfg.Mysql.DataSource)
	snatchModel := model.NewSnatchTasksModel(conn)
	notifySettingsModel := model.NewNotifySettingsModel(conn)
	notifyLogsModel := model.NewNotifyLogsModel(conn)
	ctx := context.Background()

	settings, _ := notifySettingsModel.FindFirst(ctx)
	webhookURL := ""
	if settings != nil && settings.Enabled == 1 && settings.WebhookUrl != "" {
		webhookURL = settings.WebhookUrl
	}

	var godaddyClient *godaddy.Client
	if cfg.GoDaddy.Enabled && cfg.GoDaddy.APIKey != "" && cfg.GoDaddy.APISecret != "" {
		godaddyClient = godaddy.NewClient(cfg.GoDaddy.APIKey, cfg.GoDaddy.APISecret, cfg.GoDaddy.Sandbox)
	}

	maxRetries := cfg.AutoSnatch.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	exec := &snatch.Executor{
		GodaddyClient: godaddyClient,
		WebhookURL:    webhookURL,
		SnatchTasks:   snatchModel,
		NotifyLogs:    notifyLogsModel,
		MaxRetries:    maxRetries,
		Contact: snatch.Contact{
			FirstName:    cfg.AutoSnatch.Contact.FirstName,
			LastName:     cfg.AutoSnatch.Contact.LastName,
			Email:        cfg.AutoSnatch.Contact.Email,
			Phone:        cfg.AutoSnatch.Contact.Phone,
			Organization: cfg.AutoSnatch.Contact.Organization,
			Address1:     cfg.AutoSnatch.Contact.Address1,
			City:         cfg.AutoSnatch.Contact.City,
			State:        cfg.AutoSnatch.Contact.State,
			PostalCode:   cfg.AutoSnatch.Contact.PostalCode,
			Country:      cfg.AutoSnatch.Contact.Country,
		},
	}

	var task *model.SnatchTasks
	if *taskIDFlag > 0 {
		task, _ = snatchModel.FindOne(ctx, uint64(*taskIDFlag))
		if task == nil {
			log.Fatalf("任务不存在: task-id=%d", *taskIDFlag)
		}
		if task.Status != "pending" && task.Status != "processing" {
			log.Fatalf("任务状态不允许执行: status=%s", task.Status)
		}
	} else {
		domain := strings.TrimSpace(strings.ToLower(*domainFlag))
		if domain == "" {
			log.Fatal("-domain 不能为空")
		}
		pending, err := snatchModel.FindPending(ctx)
		if err != nil {
			log.Fatalf("查询任务失败: %v", err)
		}
		for _, t := range pending {
			if t.Domain == domain {
				task = t
				break
			}
		}
		if task == nil {
			res, err := snatchModel.Insert(ctx, &model.SnatchTasks{
				DomainId:        0,
				Domain:          domain,
				Status:          "pending",
				Priority:        0,
				TargetRegistrar: "",
				AutoRegister:    1,
				RetryCount:      0,
				LastError:       sql.NullString{},
				Result:          sql.NullString{},
			})
			if err != nil {
				log.Fatalf("创建抢注任务失败: %v", err)
			}
			id, _ := res.LastInsertId()
			task, _ = snatchModel.FindOne(ctx, uint64(id))
			if task == nil {
				log.Fatal("创建任务后查询失败")
			}
			fmt.Printf("已创建抢注任务 id=%d, domain=%s\n", task.Id, task.Domain)
		}
	}

	fmt.Printf("开始抢注: domain=%s, taskId=%d\n", task.Domain, task.Id)
	if err := exec.Execute(ctx, task); err != nil {
		log.Printf("执行失败: %v", err)
		return
	}
	fmt.Println("执行完成，请查看飞书通知与任务状态")
}
