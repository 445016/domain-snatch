package snatch

import (
	"context"
	"database/sql"
	"fmt"

	"domain-snatch/model"
	"domain-snatch/pkg/feishu"
	"domain-snatch/pkg/godaddy"
	"domain-snatch/pkg/whois"
)

// Contact 抢注联系人信息（与 config AutoSnatch.Contact 一致）
type Contact struct {
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

// Platform 抢注平台常量
const (
	PlatformGoDaddy   = "godaddy"
	PlatformDropCatch = "dropcatch"
)

// Executor 统一抢注执行器：先发「开始抢注」通知，再执行 WHOIS，再按配置平台（godaddy/dropcatch）注册或下 backorder
type Executor struct {
	GodaddyClient   *godaddy.Client
	WebhookURL      string
	Contact         Contact
	MaxRetries      int
	Platform        string // 默认平台，与 config AutoSnatch.Platform 一致；任务 target_registrar 可覆盖
	SnatchTasks     model.SnatchTasksModel
	NotifyLogs      model.NotifyLogsModel
}

// Execute 执行一次抢注：先发抢前通知，再 WHOIS，可注册则根据任务决定是否调用 GoDaddy 注册
func (e *Executor) Execute(ctx context.Context, task *model.SnatchTasks) error {
	// 1. 抢前通知
	if e.WebhookURL != "" {
		client := feishu.NewClient(e.WebhookURL)
		_ = client.SendSnatchStartCard(task.Domain)
	}

	// 2. WHOIS 检查
	result, err := whois.QueryWithRateLimit(task.Domain)
	if err != nil {
		task.LastError = sql.NullString{String: err.Error(), Valid: true}
		_ = e.SnatchTasks.Update(ctx, task)
		return fmt.Errorf("whois: %w", err)
	}

	if !result.CanRegister {
		return nil // 仍不可注册，不更新任务状态
	}

	// 3. 可注册：按配置平台选择执行（任务 target_registrar 优先，否则用 Platform）
	platform := task.TargetRegistrar
	if platform == "" {
		platform = e.Platform
	}
	if platform == "" {
		platform = PlatformGoDaddy
	}

	if task.AutoRegister == 1 && e.MaxRetries > 0 {
		switch platform {
		case PlatformGoDaddy:
			if e.GodaddyClient != nil {
				return e.doRegister(ctx, task)
			}
		case PlatformDropCatch:
			// DropCatch 客户端待实现时在此调用；暂无则走手动
			task.Status = "processing"
			task.Result = sql.NullString{String: "域名已可注册，平台为 dropcatch 暂未接入，请手动抢注", Valid: true}
			_ = e.SnatchTasks.Update(ctx, task)
			if e.WebhookURL != "" {
				client := feishu.NewClient(e.WebhookURL)
				_ = client.SendSnatchResultCard(task.Domain, "processing", "平台 dropcatch 暂未接入，请手动抢注")
				e.insertNotifyLog(ctx, task, "snatch_result", "域名 "+task.Domain+" 已可注册，请手动抢注")
			}
			return nil
		default:
			// 未知平台按手动处理
		}
	}

	// 手动模式或未配置对应客户端
	task.Status = "processing"
	task.Result = sql.NullString{String: "域名已可注册，请尽快手动完成注册", Valid: true}
	_ = e.SnatchTasks.Update(ctx, task)
	if e.WebhookURL != "" {
		client := feishu.NewClient(e.WebhookURL)
		_ = client.SendSnatchResultCard(task.Domain, "processing", "域名已可注册，请尽快手动完成注册")
		e.insertNotifyLog(ctx, task, "snatch_result", "域名 "+task.Domain+" 已可注册")
	}
	return nil
}

func (e *Executor) doRegister(ctx context.Context, task *model.SnatchTasks) error {
	if task.RetryCount >= int64(e.MaxRetries) {
		task.Status = "failed"
		task.Result = sql.NullString{String: fmt.Sprintf("自动注册失败，已达最大重试次数 %d", e.MaxRetries), Valid: true}
		_ = e.SnatchTasks.Update(ctx, task)
		return fmt.Errorf("max retries %d reached", e.MaxRetries)
	}

	// 检查可用性
	availability, err := e.GodaddyClient.CheckAvailable(task.Domain)
	if err != nil {
		task.RetryCount++
		task.LastError = sql.NullString{String: fmt.Sprintf("检查可用性失败: %v", err), Valid: true}
		_ = e.SnatchTasks.Update(ctx, task)
		return fmt.Errorf("check available: %w", err)
	}
	if !availability.Available {
		task.RetryCount++
		task.LastError = sql.NullString{String: "域名在 GoDaddy 不可注册", Valid: true}
		_ = e.SnatchTasks.Update(ctx, task)
		return fmt.Errorf("domain not available on GoDaddy")
	}

	contact := godaddy.ContactInfo{
		FirstName:    e.Contact.FirstName,
		LastName:     e.Contact.LastName,
		Email:        e.Contact.Email,
		Phone:        e.Contact.Phone,
		Organization: e.Contact.Organization,
		AddressMailing: godaddy.Address{
			Address1:   e.Contact.Address1,
			City:       e.Contact.City,
			State:      e.Contact.State,
			PostalCode: e.Contact.PostalCode,
			Country:    e.Contact.Country,
		},
	}

	purchaseResp, err := e.GodaddyClient.Purchase(task.Domain, contact, 1)
	if err != nil {
		task.RetryCount++
		task.LastError = sql.NullString{String: fmt.Sprintf("购买失败: %v", err), Valid: true}
		_ = e.SnatchTasks.Update(ctx, task)
		return fmt.Errorf("purchase: %w", err)
	}

	task.Status = "success"
	task.Result = sql.NullString{String: fmt.Sprintf("自动注册成功! OrderID: %d", purchaseResp.OrderID), Valid: true}
	_ = e.SnatchTasks.Update(ctx, task)

	if e.WebhookURL != "" {
		client := feishu.NewClient(e.WebhookURL)
		_ = client.SendSnatchResultCard(task.Domain, "success",
			fmt.Sprintf("域名 %s 自动注册成功! OrderID: %d", task.Domain, purchaseResp.OrderID))
		e.insertNotifyLog(ctx, task, "snatch_success", "域名 "+task.Domain+" 自动注册成功")
	}
	return nil
}

func (e *Executor) insertNotifyLog(ctx context.Context, task *model.SnatchTasks, notifyType, content string) {
	_, _ = e.NotifyLogs.Insert(ctx, &model.NotifyLogs{
		DomainId:   task.DomainId,
		Domain:     task.Domain,
		NotifyType: notifyType,
		Channel:    "feishu",
		Content:    sql.NullString{String: content, Valid: true},
		Status:     "sent",
	})
}
