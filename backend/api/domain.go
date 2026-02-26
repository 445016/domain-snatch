package main

import (
	"flag"
	"fmt"
	"net/http"
	"time"

	"domain-snatch/api/internal/config"
	"domain-snatch/api/internal/handler"
	"domain-snatch/api/internal/svc"
	"domain-snatch/cron"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/domain.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

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
	go startCronJobs(cronSvc)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}

func startCronJobs(cronSvc *cron.CronService) {
	fmt.Println("[Cron] Starting cron jobs (only RunStatusUpdateTask)...")

	// 只运行状态更新任务（更新到期时间小于当前时间的域名状态）
	go func() {
		fmt.Println("[Cron] Status update task started (runs every 5 minutes)")
		// 启动时立即执行一次
		fmt.Printf("[Cron] Running initial status update task at %s\n", time.Now().Format("2006-01-02 15:04:05"))
		cronSvc.RunStatusUpdateTask()

		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			fmt.Printf("[Cron] Triggering status update task at %s (interval: 5 minutes)\n", time.Now().Format("2006-01-02 15:04:05"))
			cronSvc.RunStatusUpdateTask()
		}
	}()

	fmt.Println("[Cron] Cron job started successfully (only RunStatusUpdateTask)")
}
