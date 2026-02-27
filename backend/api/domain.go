package main

import (
	"flag"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"domain-snatch/api/internal/config"
	"domain-snatch/api/internal/handler"
	"domain-snatch/api/internal/svc"
	"domain-snatch/cron"
	"domain-snatch/pkg/configutil"
	"domain-snatch/pkg/snatch"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

const defaultConfigPath = "etc/domain.yaml"

var configFile = flag.String("f", defaultConfigPath, "config file path; overridden by APP_ENV when -f is default")

func main() {
	flag.Parse()

	path := configutil.ResolveConfigPath(*configFile, defaultConfigPath)
	var c config.Config
	conf.MustLoad(path, &c)
	// 日志与配置共用目录层级：统一写到 backend/logs（与 backend/etc 同级）
	if c.Log.Path != "" {
		c.Log.Path = filepath.Join(filepath.Dir(filepath.Dir(path)), "logs")
	}

	server := rest.MustNewServer(c.RestConf, rest.WithCors("*"))
	defer server.Stop()

	// 添加全局 CORS 中间件，处理 OPTIONS 预检请求
	server.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next(w, r)
		}
	})

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// 启动定时任务
	cronSvc := cron.NewCronService(c.Mysql.DataSource)
	cronSvc.SetGoDaddyClient(cron.GoDaddyConfig{
		APIKey:    c.GoDaddy.APIKey,
		APISecret: c.GoDaddy.APISecret,
		Sandbox:   c.GoDaddy.Sandbox,
		Enabled:   c.GoDaddy.Enabled,
	})
	cronSvc.SetAutoSnatchConfig(cron.AutoSnatchConfig{
		Enabled:    c.AutoSnatch.Enabled,
		MaxRetries: c.AutoSnatch.MaxRetries,
		CheckIntervals: cron.CheckIntervals{
			Registered:    c.AutoSnatch.CheckIntervals.Registered,
			Expired:       c.AutoSnatch.CheckIntervals.Expired,
			GracePeriod:   c.AutoSnatch.CheckIntervals.GracePeriod,
			Redemption:    c.AutoSnatch.CheckIntervals.Redemption,
			PendingDelete: c.AutoSnatch.CheckIntervals.PendingDelete,
		},
		Contact: cron.ContactInfo{
			FirstName:    c.AutoSnatch.Contact.FirstName,
			LastName:     c.AutoSnatch.Contact.LastName,
			Email:        c.AutoSnatch.Contact.Email,
			Phone:        c.AutoSnatch.Contact.Phone,
			Organization: c.AutoSnatch.Contact.Organization,
			Address1:     c.AutoSnatch.Contact.Address1,
			City:         c.AutoSnatch.Contact.City,
			State:        c.AutoSnatch.Contact.State,
			PostalCode:   c.AutoSnatch.Contact.PostalCode,
			Country:      c.AutoSnatch.Contact.Country,
		},
	})
	go startCronJobs(cronSvc, c)

	fmt.Printf("Starting server at %s:%d... (config: %s)\n", c.Host, c.Port, path)
	server.Start()
}

func startCronJobs(cronSvc *cron.CronService, c config.Config) {
	fmt.Println("[Cron] Starting cron jobs (RunStatusUpdateTask + delay queue worker)...")

	// 只运行状态更新任务（更新到期时间小于当前时间的域名状态）
	go func() {
		fmt.Println("[Cron] Status update task started (runs every 5 minutes)")
		fmt.Printf("[Cron] Running initial status update task at %s\n", time.Now().Format("2006-01-02 15:04:05"))
		cronSvc.RunStatusUpdateTask()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			fmt.Printf("[Cron] Triggering status update task at %s (interval: 5 minutes)\n", time.Now().Format("2006-01-02 15:04:05"))
			cronSvc.RunStatusUpdateTask()
		}
	}()

	// 延迟队列：有 Redis 时启动，按计划时间执行抢注；状态更新中发现 pending_delete+delete_date 会入队
	if len(c.Cache) > 0 {
		rds := c.Cache[0].NewRedis()
		queue := snatch.NewDelayQueue(rds)
		cronSvc.SetDelayQueue(queue)
		go cronSvc.RunDelayQueueWorker(queue)
		fmt.Println("[Cron] Delay queue worker started (polls every 2s)")
	}

	fmt.Println("[Cron] Cron jobs started successfully")
}
