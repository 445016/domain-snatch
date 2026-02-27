package cron

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"domain-snatch/model"
	lifecycle "domain-snatch/pkg/domain"
	"domain-snatch/pkg/feishu"
	"domain-snatch/pkg/godaddy"
	"domain-snatch/pkg/lock"
	"domain-snatch/pkg/snatch"
	"domain-snatch/pkg/whois"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// CheckIntervals 不同状态的检测间隔配置
type CheckIntervals struct {
	Registered    int // 秒
	Expired       int
	GracePeriod   int
	Redemption    int
	PendingDelete int
}

// ContactInfo 联系人配置
type ContactInfo struct {
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

// AutoSnatchConfig 自动抢注配置
type AutoSnatchConfig struct {
	Enabled        bool
	MaxRetries     int
	CheckIntervals CheckIntervals
	Contact        ContactInfo
}

// GoDaddyConfig GoDaddy配置
type GoDaddyConfig struct {
	APIKey    string
	APISecret string
	Sandbox   bool
	Enabled   bool
}

type CronService struct {
	domainsModel        model.DomainsModel
	snatchTasksModel    model.SnatchTasksModel
	notifySettingsModel model.NotifySettingsModel
	notifyLogsModel     model.NotifyLogsModel
	godaddyClient       *godaddy.Client
	autoSnatchConfig    *AutoSnatchConfig
	delayQueue          *snatch.DelayQueue
}

func NewCronService(dataSource string) *CronService {
	conn := sqlx.NewMysql(dataSource)
	return &CronService{
		domainsModel:        model.NewDomainsModel(conn),
		snatchTasksModel:    model.NewSnatchTasksModel(conn),
		notifySettingsModel: model.NewNotifySettingsModel(conn),
		notifyLogsModel:     model.NewNotifyLogsModel(conn),
	}
}

// SetGoDaddyClient 设置GoDaddy客户端
func (s *CronService) SetGoDaddyClient(config GoDaddyConfig) {
	if config.Enabled && config.APIKey != "" && config.APISecret != "" {
		s.godaddyClient = godaddy.NewClient(config.APIKey, config.APISecret, config.Sandbox)
		logx.Infof("[Cron] GoDaddy client initialized, sandbox=%v", config.Sandbox)
	}
}

// SetAutoSnatchConfig 设置自动抢注配置
func (s *CronService) SetAutoSnatchConfig(config AutoSnatchConfig) {
	s.autoSnatchConfig = &config
	logx.Infof("[Cron] AutoSnatch config set, enabled=%v, maxRetries=%d", config.Enabled, config.MaxRetries)
}

// SetDelayQueue 设置抢注延迟队列（用于按 delete_date 定时执行）
func (s *CronService) SetDelayQueue(queue *snatch.DelayQueue) {
	s.delayQueue = queue
	if queue != nil {
		logx.Info("[Cron] Delay queue set, enqueue on status update when pending_delete with delete_date")
	}
}

// buildExecutor 根据当前配置与通知设置构建统一抢注执行器（先发抢前通知再执行）
func (s *CronService) buildExecutor(settings *model.NotifySettings) *snatch.Executor {
	exec := &snatch.Executor{
		GodaddyClient: s.godaddyClient,
		SnatchTasks:   s.snatchTasksModel,
		NotifyLogs:    s.notifyLogsModel,
		MaxRetries:    3,
	}
	if settings != nil && settings.Enabled == 1 && settings.WebhookUrl != "" {
		exec.WebhookURL = settings.WebhookUrl
	}
	if s.autoSnatchConfig != nil {
		exec.MaxRetries = s.autoSnatchConfig.MaxRetries
		if exec.MaxRetries <= 0 {
			exec.MaxRetries = 3
		}
		exec.Contact = snatch.Contact{
			FirstName:    s.autoSnatchConfig.Contact.FirstName,
			LastName:     s.autoSnatchConfig.Contact.LastName,
			Email:        s.autoSnatchConfig.Contact.Email,
			Phone:        s.autoSnatchConfig.Contact.Phone,
			Organization: s.autoSnatchConfig.Contact.Organization,
			Address1:     s.autoSnatchConfig.Contact.Address1,
			City:         s.autoSnatchConfig.Contact.City,
			State:        s.autoSnatchConfig.Contact.State,
			PostalCode:   s.autoSnatchConfig.Contact.PostalCode,
			Country:      s.autoSnatchConfig.Contact.Country,
		}
	}
	return exec
}

// getCheckIntervalForPendingDelete 根据距离可注册时间的远近动态调整检测间隔
func (s *CronService) getCheckIntervalForPendingDelete(domain *model.Domains) time.Duration {
	if !domain.ExpiryDate.Valid {
		// 没有到期日期，使用默认间隔
		return 1 * time.Hour
	}

	// 计算可注册时间（到期日期 + 65天）
	expiryDate := domain.ExpiryDate.Time
	stages := lifecycle.CalculateLifecycleStages(&expiryDate)
	if stages == nil || stages.AvailableDate == nil {
		return 1 * time.Hour
	}

	availableDate := *stages.AvailableDate
	now := time.Now()
	duration := availableDate.Sub(now)

	// 根据距离可注册时间的距离动态调整检测间隔
	if duration <= 0 {
		// 可注册时间已过：30秒（高频检测）
		return 30 * time.Second
	} else if duration < 1*time.Hour {
		// 距离可注册时间 < 1小时：1分钟
		return 1 * time.Minute
	} else if duration < 6*time.Hour {
		// 距离可注册时间 1-6小时：5分钟
		return 5 * time.Minute
	} else if duration < 12*time.Hour {
		// 距离可注册时间 6-12小时：10分钟
		return 10 * time.Minute
	} else if duration < 24*time.Hour {
		// 距离可注册时间 12-24小时：30分钟
		return 30 * time.Minute
	} else {
		// 距离可注册时间 > 24小时：1小时
		return 1 * time.Hour
	}
}

// getCheckInterval 根据域名状态获取检测间隔
func (s *CronService) getCheckInterval(domain *model.Domains) time.Duration {
	// 对于 pending_delete 状态，使用动态间隔计算
	if domain.Status == whois.StatusPendingDelete {
		return s.getCheckIntervalForPendingDelete(domain)
	}

	if s.autoSnatchConfig == nil {
		// 默认间隔
		return 24 * time.Hour
	}

	intervals := s.autoSnatchConfig.CheckIntervals
	var seconds int

	switch domain.Status {
	case whois.StatusRedemption:
		seconds = intervals.Redemption
		if seconds <= 0 {
			seconds = 3600 // 默认1小时
		}
	case whois.StatusGracePeriod:
		seconds = intervals.GracePeriod
		if seconds <= 0 {
			seconds = 14400 // 默认4小时
		}
	case whois.StatusExpired:
		seconds = intervals.Expired
		if seconds <= 0 {
			seconds = 14400 // 默认4小时
		}
	default: // registered, unknown, etc.
		seconds = intervals.Registered
		if seconds <= 0 {
			seconds = 86400 // 默认1天
		}
	}

	return time.Duration(seconds) * time.Second
}

// isAtKeyTimePoint 判断域名今天是否在关键时间点
func (s *CronService) isAtKeyTimePoint(domain *model.Domains) bool {
	if !domain.ExpiryDate.Valid {
		return false
	}

	expiryDate := domain.ExpiryDate.Time
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	expiryDay := time.Date(expiryDate.Year(), expiryDate.Month(), expiryDate.Day(), 0, 0, 0, 0, expiryDate.Location())

	// 到期日当天
	if expiryDay.Equal(today) {
		return true
	}

	// 宽限期结束当天（到期日+30天）
	gracePeriodEndDay := expiryDay.AddDate(0, 0, 30)
	if gracePeriodEndDay.Equal(today) {
		return true
	}

	// 赎回期结束当天（到期日+60天）
	redemptionEndDay := expiryDay.AddDate(0, 0, 60)
	if redemptionEndDay.Equal(today) {
		return true
	}

	// 待删除期结束当天（到期日+65天）
	pendingDeleteEndDay := expiryDay.AddDate(0, 0, 65)
	if pendingDeleteEndDay.Equal(today) {
		return true
	}

	return false
}

// shouldCheck 判断域名是否需要检测
func (s *CronService) shouldCheck(domain *model.Domains) bool {
	// 如果今天在关键时间点，强制检测
	if s.isAtKeyTimePoint(domain) {
		return true
	}

	if !domain.LastChecked.Valid {
		return true // 从未检测过
	}

	interval := s.getCheckInterval(domain)
	nextCheck := domain.LastChecked.Time.Add(interval)

	return time.Now().After(nextCheck)
}

// RunWhoisCheck 定时WHOIS巡检（智能频率）
func (s *CronService) RunWhoisCheck() {
	ctx := context.Background()
	logx.Info("[Cron] Starting smart WHOIS check...")

	domains, err := s.domainsModel.FindMonitored(ctx)
	if err != nil {
		logx.Errorf("[Cron] Find monitored domains failed: %v", err)
		return
	}

	total := len(domains)
	checked := 0
	skipped := 0

	logx.Infof("[Cron] Found %d monitored domains, applying smart frequency...", total)

	for index, domain := range domains {
		// 智能频率检测：根据域名状态决定是否需要检测
		if !s.shouldCheck(domain) {
			skipped++
			continue
		}

		checked++
		logx.Infof("[Cron][WHOIS] checking (%d/%d), domain=%s, status=%s",
			checked, total-skipped, domain.Domain, domain.Status)

		start := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			logx.Errorf("[Cron][WHOIS] query failed, domain=%s, err=%v", domain.Domain, err)
			continue
		}

		// 更新域名信息
		oldStatus := domain.Status
		domain.Status = result.Status
		domain.Registrar = result.Registrar
		domain.WhoisStatus = result.WhoisStatus
		now := time.Now()
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

		if err := s.domainsModel.Update(ctx, domain); err != nil {
			logx.Errorf("[Cron][WHOIS] update domain failed, domain=%s, err=%v", domain.Domain, err)
		} else {
			logx.Infof("[Cron][WHOIS] success, domain=%s, status=%s->%s, whoisStatus=%s, cost=%s",
				domain.Domain, oldStatus, result.Status, result.WhoisStatus, time.Since(start).String())

			// 状态变化通知
			if oldStatus != result.Status {
				// 高优先级状态使用原有通知逻辑
				if isHighPriorityStatus(result.Status) {
					s.notifyStatusChange(ctx, domain, oldStatus, result.Status)
				}
				// 非已注册状态发送通知
				if result.Status != whois.StatusRegistered {
					s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
				}
			}
		}

		// 进度显示
		if index > 0 && index%50 == 0 {
			logx.Infof("[Cron][WHOIS] progress: %d/%d checked, %d skipped", checked, total, skipped)
		}
	}

	logx.Infof("[Cron] WHOIS check completed: %d checked, %d skipped (not due)", checked, skipped)
}

// isHighPriorityStatus 判断是否是高优先级状态
func isHighPriorityStatus(status string) bool {
	return status == whois.StatusPendingDelete ||
		status == whois.StatusRedemption ||
		status == whois.StatusAvailable
}

// notifyStatusChange 状态变化通知（保留原有逻辑，用于高优先级状态）
func (s *CronService) notifyStatusChange(ctx context.Context, domain *model.Domains, oldStatus, newStatus string) {
	settings, _ := s.notifySettingsModel.FindFirst(ctx)
	if settings == nil || settings.Enabled == 0 || settings.WebhookUrl == "" {
		return
	}

	var message string
	switch newStatus {
	case whois.StatusPendingDelete:
		message = fmt.Sprintf("域名 %s 进入待删除状态，即将释放！", domain.Domain)
	case whois.StatusRedemption:
		message = fmt.Sprintf("域名 %s 进入赎回期", domain.Domain)
	case whois.StatusAvailable:
		message = fmt.Sprintf("域名 %s 已可注册！", domain.Domain)
	default:
		return
	}

	client := feishu.NewClient(settings.WebhookUrl)
	client.SendSnatchResultCard(domain.Domain, newStatus, message)

	s.notifyLogsModel.Insert(ctx, &model.NotifyLogs{
		DomainId:   domain.Id,
		Domain:     domain.Domain,
		NotifyType: "status_change",
		Channel:    "feishu",
		Content:    sql.NullString{String: message, Valid: true},
		Status:     "sent",
	})
}

// notifyDomainStatus 通知域名状态（非已注册/限制注册状态时发送通知）
func (s *CronService) notifyDomainStatus(ctx context.Context, domain *model.Domains, oldStatus, newStatus string) {
	if newStatus == whois.StatusRegistered || newStatus == whois.StatusRestricted {
		return
	}

	settings, _ := s.notifySettingsModel.FindFirst(ctx)
	if settings == nil || settings.Enabled == 0 || settings.WebhookUrl == "" {
		return
	}

	// 构建通知消息
	var statusText string
	switch newStatus {
	case whois.StatusExpired:
		statusText = "已过期"
	case whois.StatusGracePeriod:
		statusText = "宽限期"
	case whois.StatusRedemption:
		statusText = "赎回期"
	case whois.StatusPendingDelete:
		statusText = "待删除"
	case whois.StatusAvailable:
		statusText = "可注册"
	default:
		statusText = "未知"
	}

	message := fmt.Sprintf("域名 %s 状态变更：%s -> %s", domain.Domain, oldStatus, statusText)
	if domain.ExpiryDate.Valid {
		message += fmt.Sprintf("\n到期日期：%s", domain.ExpiryDate.Time.Format("2006-01-02 15:04:05"))
		// 计算可注册时间
		stages := lifecycle.CalculateLifecycleStages(&domain.ExpiryDate.Time)
		if stages != nil && stages.AvailableDate != nil {
			message += fmt.Sprintf("\n可注册时间：%s", stages.AvailableDate.Format("2006-01-02 15:04:05"))
		}
	}

	client := feishu.NewClient(settings.WebhookUrl)
	client.SendSnatchResultCard(domain.Domain, newStatus, message)

	s.notifyLogsModel.Insert(ctx, &model.NotifyLogs{
		DomainId:   domain.Id,
		Domain:     domain.Domain,
		NotifyType: "status_change",
		Channel:    "feishu",
		Content:    sql.NullString{String: message, Valid: true},
		Status:     "sent",
	})
}

// RunExpireNotify 到期提醒
func (s *CronService) RunExpireNotify() {
	ctx := context.Background()
	logx.Info("[Cron] Starting expire notification...")

	settings, err := s.notifySettingsModel.FindFirst(ctx)
	if err != nil || settings.Enabled == 0 || settings.WebhookUrl == "" {
		logx.Info("[Cron] Notification disabled or not configured")
		return
	}

	// 查找即将到期的域名
	expiring, err := s.domainsModel.FindExpiringSoon(ctx, settings.ExpireDays)
	if err != nil {
		logx.Errorf("[Cron] Find expiring domains failed: %v", err)
		return
	}

	if len(expiring) == 0 {
		logx.Info("[Cron] No expiring domains found")
		return
	}

	// 构建通知信息
	domainInfos := make([]feishu.DomainInfo, 0, len(expiring))
	for _, d := range expiring {
		info := feishu.DomainInfo{
			Domain:    d.Domain,
			Registrar: d.Registrar,
			Status:    d.Status,
		}
		if d.ExpiryDate.Valid {
			info.ExpiryDate = d.ExpiryDate.Time.Format("2006-01-02 15:04:05")
		}
		domainInfos = append(domainInfos, info)
	}

	// 发送飞书通知
	client := feishu.NewClient(settings.WebhookUrl)
	err = client.SendDomainExpireCard(domainInfos)

	status := "sent"
	if err != nil {
		status = "failed"
		logx.Errorf("[Cron] Send feishu notification failed: %v", err)
	}

	// 记录通知日志
	for _, d := range expiring {
		s.notifyLogsModel.Insert(ctx, &model.NotifyLogs{
			DomainId:   d.Id,
			Domain:     d.Domain,
			NotifyType: "expire_warning",
			Channel:    "feishu",
			Content:    sql.NullString{String: fmt.Sprintf("域名 %s 即将到期", d.Domain), Valid: true},
			Status:     status,
		})
	}

	logx.Infof("[Cron] Expire notification completed, notified %d domains", len(expiring))
}

// RunSnatchCheck 抢注任务检查（支持自动注册）；统一通过 snatch.Executor 执行，先发抢前通知再 WHOIS/注册
func (s *CronService) RunSnatchCheck() {
	ctx := context.Background()
	logx.Info("[Cron] Starting snatch task check...")

	tasks, err := s.snatchTasksModel.FindPending(ctx)
	if err != nil {
		logx.Errorf("[Cron] Find pending tasks failed: %v", err)
		return
	}

	settings, _ := s.notifySettingsModel.FindFirst(ctx)
	exec := s.buildExecutor(settings)

	total := len(tasks)
	for index, task := range tasks {
		logx.Infof("[Cron][Snatch] checking (%d/%d), id=%d, domain=%s, status=%s, autoRegister=%d",
			index+1, total, task.Id, task.Domain, task.Status, task.AutoRegister)
		start := time.Now()
		if err := exec.Execute(ctx, task); err != nil {
			logx.Errorf("[Cron][Snatch] execute failed, domain=%s, err=%v", task.Domain, err)
		} else {
			logx.Infof("[Cron][Snatch] done, domain=%s, taskId=%d, cost=%s", task.Domain, task.Id, time.Since(start).String())
		}
	}

	logx.Info("[Cron] Snatch task check completed")
}

// RunDelayQueueWorker 延迟队列消费：循环取出到点任务并执行抢注（先发抢前通知再执行）
func (s *CronService) RunDelayQueueWorker(queue *snatch.DelayQueue) {
	if queue == nil {
		return
	}
	ctx := context.Background()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		taskID, ok, err := queue.Poll(ctx)
		if err != nil {
			logx.Errorf("[Cron][DelayQueue] poll error: %v", err)
			continue
		}
		if !ok {
			continue
		}
		task, err := s.snatchTasksModel.FindOne(ctx, uint64(taskID))
		if err != nil || task == nil {
			logx.Infof("[Cron][DelayQueue] task not found or already done: taskId=%d", taskID)
			continue
		}
		if task.Status != "pending" && task.Status != "processing" {
			continue
		}
		settings, _ := s.notifySettingsModel.FindFirst(ctx)
		exec := s.buildExecutor(settings)
		if err := exec.Execute(ctx, task); err != nil {
			logx.Errorf("[Cron][DelayQueue] execute failed, domain=%s, err=%v", task.Domain, err)
		} else {
			logx.Infof("[Cron][DelayQueue] executed, domain=%s, taskId=%d", task.Domain, taskID)
		}
	}
}

// RunPendingDeleteCheck 高频检测待删除域名（单独的高频任务）
func (s *CronService) RunPendingDeleteCheck() {
	ctx := context.Background()
	logx.Info("[Cron] Starting pending delete high-frequency check...")

	// 查找所有待删除状态的抢注任务
	tasks, err := s.snatchTasksModel.FindPending(ctx)
	if err != nil {
		logx.Errorf("[Cron] Find pending tasks failed: %v", err)
		return
	}

	// 只检查关联域名状态为 pending_delete 的任务
	for _, task := range tasks {
		domain, err := s.domainsModel.FindOne(ctx, uint64(task.DomainId))
		if err != nil {
			continue
		}

		if domain.Status == whois.StatusPendingDelete {
			logx.Infof("[Cron][PendingDelete] high-freq check, domain=%s", task.Domain)

			result, err := whois.QueryWithRateLimit(task.Domain)
			if err != nil {
				logx.Errorf("[Cron][PendingDelete] WHOIS query failed, domain=%s, err=%v", task.Domain, err)
				continue
			}

			if result.CanRegister {
				logx.Infof("[Cron][PendingDelete] domain released! domain=%s", task.Domain)
				settings, _ := s.notifySettingsModel.FindFirst(ctx)
				exec := s.buildExecutor(settings)
				_ = exec.Execute(ctx, task)
			}
		}
	}

	logx.Info("[Cron] Pending delete check completed")
}

// RunDailyStatusUpdate 每日状态更新：检测关键时间点的域名和已到可注册时间的域名
func (s *CronService) RunDailyStatusUpdate() {
	ctx := context.Background()
	logx.Info("[Cron] Starting daily status update...")

	// 1. 查找所有在关键时间点的域名（今天可能发生状态变化的域名）
	keyTimePointDomains, err := s.domainsModel.FindDomainsAtKeyTimePoints(ctx)
	if err != nil {
		logx.Errorf("[Cron] Find domains at key time points failed: %v", err)
	} else {
		logx.Infof("[Cron] Found %d domains at key time points", len(keyTimePointDomains))
	}

	// 2. 查找已到可注册时间的域名
	availableDomains, err := s.domainsModel.FindAvailableOrPastDomains(ctx)
	if err != nil {
		logx.Errorf("[Cron] Find available or past domains failed: %v", err)
	} else {
		logx.Infof("[Cron] Found %d available or past domains", len(availableDomains))
	}

	// 3. 合并去重
	domainMap := make(map[uint64]*model.Domains)
	for _, d := range keyTimePointDomains {
		domainMap[d.Id] = d
	}
	for _, d := range availableDomains {
		domainMap[d.Id] = d
	}

	total := len(domainMap)
	if total == 0 {
		logx.Info("[Cron] No domains need daily status update")
		return
	}

	logx.Infof("[Cron] Total %d unique domains need status update", total)

	checked := 0
	for _, domain := range domainMap {
		checked++
		logx.Infof("[Cron][DailyUpdate] checking (%d/%d), domain=%s, status=%s",
			checked, total, domain.Domain, domain.Status)

		start := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			logx.Errorf("[Cron][DailyUpdate] WHOIS query failed, domain=%s, err=%v", domain.Domain, err)
			continue
		}

		// 更新域名信息
		oldStatus := domain.Status
		domain.Status = result.Status
		domain.Registrar = result.Registrar
		domain.WhoisStatus = result.WhoisStatus
		now := time.Now()
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

		if err := s.domainsModel.Update(ctx, domain); err != nil {
			logx.Errorf("[Cron][DailyUpdate] update domain failed, domain=%s, err=%v", domain.Domain, err)
		} else {
			logx.Infof("[Cron][DailyUpdate] success, domain=%s, status=%s->%s, whoisStatus=%s, cost=%s",
				domain.Domain, oldStatus, result.Status, result.WhoisStatus, time.Since(start).String())

			// 触发通知：如果状态不是 registered 且状态发生变化
			if oldStatus != result.Status && result.Status != whois.StatusRegistered {
				s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
			}
		}
	}

	logx.Infof("[Cron] Daily status update completed: %d domains checked", checked)
}

// RunPreciseStatusCheck 精确状态检测：检测即将到达关键时间点的域名（基于时分秒）
// 建议每1-5分钟运行一次，确保在精确时间点检测状态变化
func (s *CronService) RunPreciseStatusCheck() {
	ctx := context.Background()
	logx.Info("[Cron] Starting precise status check...")

	// 查找即将到达关键时间点的域名（提前5分钟开始检测，延后5分钟结束检测）
	// 这样可以覆盖时间点的前后范围，确保不遗漏
	domains, err := s.domainsModel.FindDomainsNearKeyTimePoints(ctx, 5, 5)
	if err != nil {
		logx.Errorf("[Cron] Find domains near key time points failed: %v", err)
		return
	}

	total := len(domains)
	if total == 0 {
		logx.Info("[Cron] No domains near key time points")
		return
	}

	logx.Infof("[Cron] Found %d domains near key time points", total)

	checked := 0
	for _, domain := range domains {
		checked++
		logx.Infof("[Cron][PreciseCheck] checking (%d/%d), domain=%s, status=%s",
			checked, total, domain.Domain, domain.Status)

		start := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			logx.Errorf("[Cron][PreciseCheck] WHOIS query failed, domain=%s, err=%v", domain.Domain, err)
			continue
		}

		// 更新域名信息
		oldStatus := domain.Status
		domain.Status = result.Status
		domain.Registrar = result.Registrar
		domain.WhoisStatus = result.WhoisStatus
		now := time.Now()
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

		if err := s.domainsModel.Update(ctx, domain); err != nil {
			logx.Errorf("[Cron][PreciseCheck] update domain failed, domain=%s, err=%v", domain.Domain, err)
		} else {
			logx.Infof("[Cron][PreciseCheck] success, domain=%s, status=%s->%s, whoisStatus=%s, cost=%s",
				domain.Domain, oldStatus, result.Status, result.WhoisStatus, time.Since(start).String())

			// 触发通知：如果状态不是 registered 且状态发生变化
			if oldStatus != result.Status && result.Status != whois.StatusRegistered {
				s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
			}
		}
	}

	logx.Infof("[Cron] Precise status check completed: %d domains checked", checked)
}

// RunRealtimeCheck 实时检测：检测即将变成可注册状态的域名
// 每30秒运行一次，对即将可注册的域名进行高频检测
func (s *CronService) RunRealtimeCheck() {
	ctx := context.Background()
	logx.Info("[Cron] Starting realtime check for near-available domains...")

	// 查找即将变成可注册状态的域名（提前24小时开始实时检测）
	domains, err := s.domainsModel.FindDomainsNearAvailable(ctx, 24)
	if err != nil {
		logx.Errorf("[Cron] Find domains near available failed: %v", err)
		return
	}

	total := len(domains)
	if total == 0 {
		logx.Info("[Cron] No domains near available status")
		return
	}

	logx.Infof("[Cron] Found %d domains near available status, starting realtime check", total)

	checked := 0
	for _, domain := range domains {
		checked++

		// 计算可注册时间
		if !domain.ExpiryDate.Valid {
			continue
		}

		expiryDate := domain.ExpiryDate.Time
		stages := lifecycle.CalculateLifecycleStages(&expiryDate)
		if stages == nil || stages.AvailableDate == nil {
			continue
		}

		availableDate := *stages.AvailableDate
		now := time.Now()
		timeUntilAvailable := availableDate.Sub(now)

		logx.Infof("[Cron][Realtime] checking (%d/%d), domain=%s, status=%s, availableIn=%s",
			checked, total, domain.Domain, domain.Status, timeUntilAvailable.String())

		start := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			logx.Errorf("[Cron][Realtime] WHOIS query failed, domain=%s, err=%v", domain.Domain, err)
			continue
		}

		// 更新域名信息
		oldStatus := domain.Status
		domain.Status = result.Status
		domain.Registrar = result.Registrar
		domain.WhoisStatus = result.WhoisStatus
		now = time.Now()
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

		if err := s.domainsModel.Update(ctx, domain); err != nil {
			logx.Errorf("[Cron][Realtime] update domain failed, domain=%s, err=%v", domain.Domain, err)
		} else {
			logx.Infof("[Cron][Realtime] success, domain=%s, status=%s->%s, whoisStatus=%s, cost=%s",
				domain.Domain, oldStatus, result.Status, result.WhoisStatus, time.Since(start).String())

			// 如果域名已可注册，发送通知
			if result.CanRegister || result.Status == whois.StatusAvailable {
				logx.Infof("[Cron][Realtime] Domain is now available! domain=%s", domain.Domain)
				if oldStatus != result.Status {
					s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
				}
			} else if oldStatus != result.Status && result.Status != whois.StatusRegistered {
				// 状态变化且非已注册，发送通知
				s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
			}
		}
	}

	logx.Infof("[Cron] Realtime check completed: %d domains checked", checked)
}

// RunSnatchGuardian 守护进程：检测可注册时间 < 5分钟的域名，自动创建抢注任务并高频检测
func (s *CronService) RunSnatchGuardian() {
	ctx := context.Background()
	logx.Info("[Cron] Starting snatch guardian process...")

	// 查找可注册时间 < 5分钟的域名
	domains, err := s.domainsModel.FindDomainsWithinMinutesToAvailable(ctx, 5)
	if err != nil {
		logx.Errorf("[Cron] Find domains within 5 minutes to available failed: %v", err)
		return
	}

	total := len(domains)
	if total == 0 {
		logx.Info("[Cron] No domains within 5 minutes to available")
		return
	}

	logx.Infof("[Cron] Found %d domains within 5 minutes to available, creating snatch tasks...", total)

	settings, _ := s.notifySettingsModel.FindFirst(ctx)

	for _, domain := range domains {
		// 检查是否已存在抢注任务
		existingTasks, err := s.snatchTasksModel.FindPending(ctx)
		if err != nil {
			logx.Errorf("[Cron][Guardian] Find pending tasks failed: %v", err)
			continue
		}

		// 检查该域名是否已有待处理的抢注任务
		hasTask := false
		for _, task := range existingTasks {
			if task.DomainId == domain.Id && (task.Status == "pending" || task.Status == "processing") {
				hasTask = true
				// 如果任务存在但未启用自动注册，更新为自动注册并提高优先级
				if task.AutoRegister == 0 {
					task.AutoRegister = 1
					task.Priority = 9999 // 最高优先级
					if err := s.snatchTasksModel.Update(ctx, task); err != nil {
						logx.Errorf("[Cron][Guardian] Update task failed, domain=%s, err=%v", domain.Domain, err)
					} else {
						logx.Infof("[Cron][Guardian] Updated task to auto-register, domain=%s, taskId=%d", domain.Domain, task.Id)
					}
				}
				break
			}
		}

		// 如果不存在任务，创建新的抢注任务
		if !hasTask {
			task := &model.SnatchTasks{
				DomainId:     domain.Id,
				Domain:       domain.Domain,
				Status:       "pending",
				Priority:     9999, // 最高优先级
				AutoRegister: 1,    // 自动注册
				RetryCount:   0,
			}

			result, err := s.snatchTasksModel.Insert(ctx, task)
			if err != nil {
				logx.Errorf("[Cron][Guardian] Create snatch task failed, domain=%s, err=%v", domain.Domain, err)
				continue
			}

			taskId, _ := result.LastInsertId()
			logx.Infof("[Cron][Guardian] Created snatch task, domain=%s, taskId=%d, autoRegister=1, priority=9999",
				domain.Domain, taskId)
		}
	}

	// 对已创建的抢注任务进行高频检测
	pendingTasks, err := s.snatchTasksModel.FindPending(ctx)
	if err != nil {
		logx.Errorf("[Cron][Guardian] Find pending tasks failed: %v", err)
		return
	}

	// 只检测高优先级的任务（即将可注册的域名）
	guardianTasks := make([]*model.SnatchTasks, 0)
	for _, task := range pendingTasks {
		if task.Priority >= 9999 && task.AutoRegister == 1 {
			// 验证域名是否真的在5分钟内可注册
			domain, err := s.domainsModel.FindOne(ctx, uint64(task.DomainId))
			if err != nil {
				continue
			}
			if !domain.ExpiryDate.Valid {
				continue
			}
			stages := lifecycle.CalculateLifecycleStages(&domain.ExpiryDate.Time)
			if stages != nil && stages.AvailableDate != nil {
				timeUntilAvailable := time.Until(*stages.AvailableDate)
				if timeUntilAvailable <= 5*time.Minute && timeUntilAvailable >= 0 {
					guardianTasks = append(guardianTasks, task)
				}
			}
		}
	}

	if len(guardianTasks) == 0 {
		logx.Info("[Cron][Guardian] No guardian tasks to process")
		return
	}

	logx.Infof("[Cron][Guardian] Processing %d guardian tasks (high priority, auto-register)", len(guardianTasks))

	// 高频检测这些任务
	for _, task := range guardianTasks {
		logx.Infof("[Cron][Guardian] checking task, domain=%s, taskId=%d", task.Domain, task.Id)

		start := time.Now()
		result, err := whois.QueryWithRateLimit(task.Domain)
		if err != nil {
			logx.Errorf("[Cron][Guardian] WHOIS query failed, domain=%s, err=%v", task.Domain, err)
			task.LastError = sql.NullString{String: err.Error(), Valid: true}
			s.snatchTasksModel.Update(ctx, task)
			continue
		}

		if result.CanRegister {
			logx.Infof("[Cron][Guardian] Domain is available! Starting snatch, domain=%s, taskId=%d", task.Domain, task.Id)
			exec := s.buildExecutor(settings)
			_ = exec.Execute(ctx, task)
			logx.Infof("[Cron][Guardian] Snatch triggered, domain=%s, taskId=%d, cost=%s",
				task.Domain, task.Id, time.Since(start).String())
		} else {
			// 域名还不可注册，记录日志
			logx.Infof("[Cron][Guardian] Domain not available yet, domain=%s, status=%s, whoisStatus=%s, cost=%s",
				task.Domain, result.Status, result.WhoisStatus, time.Since(start).String())
		}
	}

	logx.Infof("[Cron] Snatch guardian process completed: %d tasks processed", len(guardianTasks))
}

// RunStatusUpdateTask 状态更新任务：更新到期时间小于当前时间的所有域名状态
// 包括新导入没有到期时间的域名（通过 WHOIS 查询获取到期时间）

// updateDomainWithRetry 带重试机制的域名更新函数
// 处理数据库锁等待超时等临时错误
// 优化策略：使用内存锁确保同一时间只有一个任务在处理某个域名，快速失败（5秒超时），多次重试（最多5次），指数退避
// 确保 cron 和接口检测可以并发运行，互不影响
func (s *CronService) updateDomainWithRetry(ctx context.Context, domain *model.Domains, maxRetries int, retryDelay time.Duration) error {
	// 获取域名的互斥锁，确保同一时间只有一个任务在处理这个域名
	domainLock := lock.GetDomainLock(domain.Domain)
	domainLock.Lock()
	defer domainLock.Unlock()

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		// 每次重试前都重新获取最新记录，确保使用最新数据
		if i > 0 {
			latestDomain, findErr := s.domainsModel.FindOneByDomain(ctx, domain.Domain)
			if findErr == nil {
				// 保留 WHOIS 查询结果，但使用最新的数据库记录（ID、时间戳等）
				whoisStatus := domain.Status
				whoisRegistrar := domain.Registrar
				whoisWhoisStatus := domain.WhoisStatus
				whoisExpiryDate := domain.ExpiryDate
				whoisCreationDate := domain.CreationDate
				whoisDeleteDate := domain.DeleteDate
				whoisWhoisRaw := domain.WhoisRaw
				whoisLastChecked := domain.LastChecked

				// 使用最新记录
				*domain = *latestDomain

				// 恢复 WHOIS 查询结果
				domain.Status = whoisStatus
				domain.Registrar = whoisRegistrar
				domain.WhoisStatus = whoisWhoisStatus
				domain.ExpiryDate = whoisExpiryDate
				domain.CreationDate = whoisCreationDate
				domain.DeleteDate = whoisDeleteDate
				domain.WhoisRaw = whoisWhoisRaw
				domain.LastChecked = whoisLastChecked
			}
		}

		err := s.domainsModel.Update(ctx, domain)
		if err == nil {
			if i > 0 {
				logx.Infof("[Cron] Update succeeded after %d retries, domain=%s", i, domain.Domain)
			}
			return nil
		}

		lastErr = err
		errStr := err.Error()

		// 检查是否是锁等待超时错误
		if strings.Contains(errStr, "Lock wait timeout") ||
			strings.Contains(errStr, "1205") ||
			strings.Contains(errStr, "try restarting transaction") {
			if i < maxRetries-1 {
				logx.Infof("[Cron] ⚠️ Lock wait timeout, retrying (%d/%d), domain=%s, delay=%v",
					i+1, maxRetries, domain.Domain, retryDelay)
				time.Sleep(retryDelay)
				retryDelay = retryDelay * 2 // 指数退避：100ms -> 200ms -> 400ms
				continue
			}
		}

		// 其他错误直接返回
		return err
	}

	return lastErr
}

func (s *CronService) RunStatusUpdateTask() {
	ctx := context.Background()
	startTime := time.Now()
	logx.Infof("[Cron][StatusUpdate] ===== Task started at %s =====", startTime.Format("2006-01-02 15:04:05"))
	logx.Infof("[Cron][StatusUpdate] Query: (1) registered 且 (到期<=NOW 或 无到期) (2) 非 registered/restricted 的所有状态")

	// 待检测域名：1) 已注册且到期<=当前或无到期 2) 除已注册、限制注册外的所有状态
	domains, err := s.domainsModel.FindDomainsToCheck(ctx)
	if err != nil {
		logx.Errorf("[Cron][StatusUpdate] ❌ Query failed: %v", err)
		logx.Infof("[Cron][StatusUpdate] ===== Task failed at %s (duration: %s) =====",
			time.Now().Format("2006-01-02 15:04:05"), time.Since(startTime).String())
		return
	}

	total := len(domains)
	if total == 0 {
		logx.Infof("[Cron][StatusUpdate] ✅ Result: No domains need status update (found: 0)")
		logx.Infof("[Cron][StatusUpdate] ===== Task completed at %s (duration: %s, checked: 0, result: no domains to update) =====",
			time.Now().Format("2006-01-02 15:04:05"), time.Since(startTime).String())
		return
	}

	logx.Infof("[Cron][StatusUpdate] ✅ Result: Found %d domains need status update (found: %d)", total, total)
	logx.Infof("[Cron][StatusUpdate] Starting check for %d domains...", total)
	// 记录前5个域名的详细信息，便于诊断
	for i, d := range domains {
		if i >= 5 {
			break
		}
		expiryInfo := "NULL"
		if d.ExpiryDate.Valid {
			expiryInfo = d.ExpiryDate.Time.Format("2006-01-02 15:04:05")
		}
		logx.Infof("[Cron][StatusUpdate] Domain[%d]: domain=%s, status=%s, expiryDate=%s, monitor=%d",
			i+1, d.Domain, d.Status, expiryInfo, d.Monitor)
	}

	checked := 0
	successCount := 0
	errorCount := 0
	for _, domain := range domains {
		checked++
		logx.Infof("[Cron][StatusUpdate] [%d/%d] Checking domain=%s, currentStatus=%s, hasExpiryDate=%v",
			checked, total, domain.Domain, domain.Status, domain.ExpiryDate.Valid)

		// 在 WHOIS 查询前获取锁，确保同一时间只有一个任务在处理这个域名
		domainLock := lock.GetDomainLock(domain.Domain)
		domainLock.Lock()

		start := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)

		queryDuration := time.Since(start)
		if queryDuration > 20*time.Second {
			logx.Infof("[Cron][StatusUpdate] [%d/%d] ⚠️ WHOIS query took too long, domain=%s, duration=%s",
				checked, total, domain.Domain, queryDuration.String())
		}

		if err != nil {
			domainLock.Unlock()
			errorCount++
			logx.Errorf("[Cron][StatusUpdate] [%d/%d] WHOIS query failed, domain=%s, err=%v, duration=%s",
				checked, total, domain.Domain, err, queryDuration.String())
			continue
		}

		// 更新域名信息
		oldStatus := domain.Status
		domain.Status = result.Status
		domain.Registrar = result.Registrar
		domain.WhoisStatus = result.WhoisStatus
		now := time.Now()
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

		// 更新前重新获取最新记录，避免使用过时数据导致锁冲突
		latestDomain, err := s.domainsModel.FindOneByDomain(ctx, domain.Domain)
		if err != nil {
			domainLock.Unlock()
			logx.Errorf("[Cron][StatusUpdate] [%d/%d] FindOneByDomain failed, domain=%s, err=%v",
				checked, total, domain.Domain, err)
			errorCount++
			continue
		}

		// 使用最新记录更新，但保留 WHOIS 查询结果
		latestDomain.Status = result.Status
		latestDomain.Registrar = result.Registrar
		latestDomain.WhoisStatus = result.WhoisStatus
		latestDomain.LastChecked = sql.NullTime{Time: now, Valid: true}

		if result.ExpiryDate != nil {
			latestDomain.ExpiryDate = sql.NullTime{Time: *result.ExpiryDate, Valid: true}
		}
		if result.CreationDate != nil {
			latestDomain.CreationDate = sql.NullTime{Time: *result.CreationDate, Valid: true}
		}
		if result.DeleteDate != nil {
			latestDomain.DeleteDate = sql.NullTime{Time: *result.DeleteDate, Valid: true}
		}
		if result.WhoisRaw != "" {
			latestDomain.WhoisRaw = sql.NullString{String: result.WhoisRaw, Valid: true}
		}

		// 直接更新（不需要重试，因为已经在锁内）
		updateErr := s.domainsModel.Update(ctx, latestDomain)
		if updateErr != nil {
			domainLock.Unlock()
			errorCount++
			logx.Errorf("[Cron][StatusUpdate] [%d/%d] Update failed, domain=%s, err=%v",
				checked, total, domain.Domain, updateErr)
		} else {
			domainLock.Unlock()
			successCount++
			statusChanged := oldStatus != result.Status
			logx.Infof("[Cron][StatusUpdate] [%d/%d] ✓ Success, domain=%s, status=%s->%s, whoisStatus=%s, cost=%s, statusChanged=%v",
				checked, total, domain.Domain, oldStatus, result.Status, result.WhoisStatus, time.Since(start).String(), statusChanged)

			// 触发通知：如果状态不是 registered 且状态发生变化
			if statusChanged && result.Status != whois.StatusRegistered && result.Status != whois.StatusRestricted {
				logx.Infof("[Cron][StatusUpdate] [%d/%d] Sending notification for domain=%s, status=%s->%s",
					checked, total, domain.Domain, oldStatus, result.Status)
				s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
			}
			// 延迟队列：pending_delete 且已有 delete_date 时入队，到点由 worker 执行抢注
			if s.delayQueue != nil && result.Status == whois.StatusPendingDelete && result.DeleteDate != nil {
				task, _ := s.snatchTasksModel.FindOneByDomainId(ctx, uint64(latestDomain.Id))
				if task != nil {
					if err := s.delayQueue.Add(ctx, int64(task.Id), *result.DeleteDate); err != nil {
						logx.Errorf("[Cron][StatusUpdate] delay queue add failed, domain=%s, taskId=%d, err=%v", domain.Domain, task.Id, err)
					} else {
						logx.Infof("[Cron][StatusUpdate] enqueued snatch task, domain=%s, taskId=%d, executeAt=%s", domain.Domain, task.Id, result.DeleteDate.Format("2006-01-02 15:04:05"))
					}
				}
			}
		}
	}

	duration := time.Since(startTime)
	logx.Infof("[Cron][StatusUpdate] ✅ Task result: total=%d, checked=%d, success=%d, errors=%d", total, checked, successCount, errorCount)
	logx.Infof("[Cron][StatusUpdate] ===== Task completed at %s (duration: %s, total: %d, checked: %d, success: %d, errors: %d) =====",
		time.Now().Format("2006-01-02 15:04:05"), duration.String(), total, checked, successCount, errorCount)
}

// RunHighFrequencyCheck 高频检测任务：检测处于删除期且接近可注册时间的域名
// 根据距离可注册时间的远近动态调整检测频率
func (s *CronService) RunHighFrequencyCheck() {
	ctx := context.Background()
	startTime := time.Now()
	logx.Infof("[Cron][HighFreq] ===== Task started at %s =====", startTime.Format("2006-01-02 15:04:05"))

	// 查找处于删除期且接近可注册时间的域名（提前24小时开始检测）
	domains, err := s.domainsModel.FindPendingDeleteDomainsNearAvailable(ctx, 24)
	if err != nil {
		logx.Errorf("[Cron][HighFreq] Find pending delete domains near available failed: %v", err)
		logx.Infof("[Cron][HighFreq] ===== Task failed at %s (duration: %s) =====",
			time.Now().Format("2006-01-02 15:04:05"), time.Since(startTime).String())
		return
	}

	total := len(domains)
	if total == 0 {
		logx.Infof("[Cron][HighFreq] No pending_delete domains near available (within 24 hours)")
		logx.Infof("[Cron][HighFreq] ===== Task completed at %s (duration: %s, checked: 0) =====",
			time.Now().Format("2006-01-02 15:04:05"), time.Since(startTime).String())
		return
	}

	logx.Infof("[Cron][HighFreq] Found %d pending_delete domains near available (within 24 hours), starting high-frequency check...", total)

	checked := 0
	skippedCount := 0
	successCount := 0
	errorCount := 0
	for _, domain := range domains {
		if !domain.ExpiryDate.Valid {
			skippedCount++
			logx.Infof("[Cron][HighFreq] Skipping domain=%s (no expiry date)", domain.Domain)
			continue
		}

		// 计算可注册时间和距离
		expiryDate := domain.ExpiryDate.Time
		stages := lifecycle.CalculateLifecycleStages(&expiryDate)
		if stages == nil || stages.AvailableDate == nil {
			skippedCount++
			logx.Infof("[Cron][HighFreq] Skipping domain=%s (cannot calculate available date)", domain.Domain)
			continue
		}

		availableDate := *stages.AvailableDate
		now := time.Now()
		timeUntilAvailable := availableDate.Sub(now)

		// 根据距离可注册时间的远近决定是否检测
		// 如果距离太远（>24小时），跳过本次检测
		if timeUntilAvailable > 24*time.Hour {
			skippedCount++
			logx.Infof("[Cron][HighFreq] Skipping domain=%s (available date too far: %s)",
				domain.Domain, timeUntilAvailable.String())
			continue
		}

		// 检查是否需要检测（根据距离动态调整）
		shouldCheck := false
		var checkInterval time.Duration

		if timeUntilAvailable <= 0 {
			// 可注册时间已过：必须检测
			shouldCheck = true
			checkInterval = 10 * time.Second
		} else if timeUntilAvailable < 5*time.Minute {
			// 距离可注册时间 < 5分钟：每10秒检测
			shouldCheck = true
			checkInterval = 10 * time.Second
		} else if timeUntilAvailable < 30*time.Minute {
			// 距离可注册时间 5-30分钟：每30秒检测
			if !domain.LastChecked.Valid || now.Sub(domain.LastChecked.Time) >= 30*time.Second {
				shouldCheck = true
				checkInterval = 30 * time.Second
			}
		} else if timeUntilAvailable < 2*time.Hour {
			// 距离可注册时间 30分钟-2小时：每1分钟检测
			if !domain.LastChecked.Valid || now.Sub(domain.LastChecked.Time) >= 1*time.Minute {
				shouldCheck = true
				checkInterval = 1 * time.Minute
			}
		} else {
			// 距离可注册时间 2-24小时：每5分钟检测
			if !domain.LastChecked.Valid || now.Sub(domain.LastChecked.Time) >= 5*time.Minute {
				shouldCheck = true
				checkInterval = 5 * time.Minute
			}
		}

		if !shouldCheck {
			skippedCount++
			logx.Infof("[Cron][HighFreq] Skipping domain=%s (not due yet, lastChecked=%v, interval=%s)",
				domain.Domain, domain.LastChecked.Valid, checkInterval.String())
			continue
		}

		checked++
		logx.Infof("[Cron][HighFreq] [%d/%d] Checking domain=%s, status=%s, availableIn=%s, checkInterval=%s",
			checked, total, domain.Domain, domain.Status, timeUntilAvailable.String(), checkInterval.String())

		start := time.Now()
		result, err := whois.QueryWithRateLimit(domain.Domain)
		if err != nil {
			errorCount++
			logx.Errorf("[Cron][HighFreq] [%d/%d] WHOIS query failed, domain=%s, err=%v",
				checked, total, domain.Domain, err)
			continue
		}

		// 更新域名信息
		oldStatus := domain.Status
		domain.Status = result.Status
		domain.Registrar = result.Registrar
		domain.WhoisStatus = result.WhoisStatus
		now = time.Now()
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

		if err := s.domainsModel.Update(ctx, domain); err != nil {
			errorCount++
			logx.Errorf("[Cron][HighFreq] [%d/%d] Update failed, domain=%s, err=%v",
				checked, total, domain.Domain, err)
		} else {
			successCount++
			statusChanged := oldStatus != result.Status
			logx.Infof("[Cron][HighFreq] [%d/%d] ✓ Success, domain=%s, status=%s->%s, whoisStatus=%s, cost=%s, statusChanged=%v",
				checked, total, domain.Domain, oldStatus, result.Status, result.WhoisStatus, time.Since(start).String(), statusChanged)

			// 触发通知：状态变化且非 registered/restricted
			if statusChanged && result.Status != whois.StatusRegistered && result.Status != whois.StatusRestricted {
				logx.Infof("[Cron][HighFreq] [%d/%d] Sending notification for domain=%s, status=%s->%s",
					checked, total, domain.Domain, oldStatus, result.Status)
				s.notifyDomainStatus(ctx, domain, oldStatus, result.Status)
			}
		}
	}

	duration := time.Since(startTime)
	logx.Infof("[Cron][HighFreq] ===== Task completed at %s (duration: %s, total: %d, checked: %d, success: %d, errors: %d, skipped: %d) =====",
		time.Now().Format("2006-01-02 15:04:05"), duration.String(), total, checked, successCount, errorCount, skippedCount)
}
