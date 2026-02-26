package logic

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/model"
	"domain-snatch/pkg/lock"
	"domain-snatch/pkg/whois"

	"github.com/zeromicro/go-zero/core/logx"
)

type DomainCheckLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDomainCheckLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DomainCheckLogic {
	return &DomainCheckLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// updateDomainWithRetry 带重试机制的域名更新函数
// 处理数据库锁等待超时等临时错误
// 优化策略：使用内存锁确保同一时间只有一个任务在处理某个域名，快速失败（5秒超时），多次重试（最多5次），指数退避
// 确保 cron 和接口检测可以并发运行，互不影响
func (l *DomainCheckLogic) updateDomainWithRetry(ctx context.Context, domain *model.Domains, maxRetries int, retryDelay time.Duration) error {
	// 获取域名的互斥锁，确保同一时间只有一个任务在处理这个域名
	domainLock := lock.GetDomainLock(domain.Domain)
	domainLock.Lock()
	defer domainLock.Unlock()

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		// 每次重试前都重新获取最新记录，确保使用最新数据
		if i > 0 {
			latestDomain, findErr := l.svcCtx.DomainsModel.FindOneByDomain(ctx, domain.Domain)
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

		err := l.svcCtx.DomainsModel.Update(ctx, domain)
		if err == nil {
			if i > 0 {
				l.Infof("[DomainCheck] Update succeeded after %d retries, domain=%s", i, domain.Domain)
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
				l.Infof("[DomainCheck] ⚠️ Lock wait timeout, retrying (%d/%d), domain=%s, delay=%v",
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

func (l *DomainCheckLogic) DomainCheck(req *types.DomainCheckReq) (resp *types.DomainCheckResp, err error) {
	var count int64

	total := len(req.Ids)
	l.Infof("[DomainCheck] start batch WHOIS check, total=%d", total)

	updatedItems := make([]types.DomainItem, 0, total)

	for index, id := range req.Ids {
		l.Infof("[DomainCheck] checking (%d/%d), id=%d", index+1, total, id)

		domain, err := l.svcCtx.DomainsModel.FindOne(l.ctx, uint64(id))
		if err != nil {
			l.Errorf("[DomainCheck] load domain failed, id=%d, err=%v", id, err)
			continue
		}

		// 在 WHOIS 查询前获取锁，确保同一时间只有一个任务在处理这个域名
		domainLock := lock.GetDomainLock(domain.Domain)
		domainLock.Lock()

		now := time.Now()
		l.Infof("[DomainCheck] WHOIS query start, domain=%s", domain.Domain)

		var result *whois.Result
		// 单个查询：不使用速率限制，直接快速查询
		// 批量查询：使用速率限制，防止被 WHOIS 服务器限流
		if total == 1 {
			// 单个查询：优先使用快速查询，如果失败再使用完整查询逻辑
			result, err = whois.QueryFast(domain.Domain)
			if err != nil || result.WhoisRaw == "" || whois.IsOnlyIANAInfo(result.WhoisRaw) {
				// 快速查询失败，使用完整查询逻辑
				result, err = whois.Query(domain.Domain)
			}
		} else {
			// 批量查询：使用速率限制，防止被限流
			result, err = whois.QueryWithRateLimit(domain.Domain)
		}
		if err != nil {
			domainLock.Unlock()
			l.Errorf("[DomainCheck] WHOIS query failed, domain=%s, err=%v", domain.Domain, err)
			// 即使查询失败，也更新检查时间
			// 更新前重新获取最新记录，避免锁冲突
			latestDomain, findErr := l.svcCtx.DomainsModel.FindOneByDomain(l.ctx, domain.Domain)
			if findErr != nil {
				l.Errorf("[DomainCheck] FindOneByDomain failed, domain=%s, err=%v", domain.Domain, findErr)
				continue
			}
			latestDomain.LastChecked = sql.NullTime{Time: now, Valid: true}
			// 直接更新（不需要重试，因为已经在锁外）
			if updateErr := l.svcCtx.DomainsModel.Update(l.ctx, latestDomain); updateErr != nil {
				l.Errorf("[DomainCheck] update lastChecked failed, domain=%s, err=%v", domain.Domain, updateErr)
			}
			continue
		}

		// 更新前重新获取最新记录，避免使用过时数据导致锁冲突
		latestDomain, err := l.svcCtx.DomainsModel.FindOneByDomain(l.ctx, domain.Domain)
		if err != nil {
			domainLock.Unlock()
			l.Errorf("[DomainCheck] FindOneByDomain failed, domain=%s, err=%v", domain.Domain, err)
			continue
		}

		// 使用最新记录更新，但保留 WHOIS 查询结果
		oldStatus := latestDomain.Status
		latestDomain.Status = result.Status
		latestDomain.Registrar = result.Registrar
		latestDomain.WhoisStatus = result.WhoisStatus
		latestDomain.LastChecked = sql.NullTime{Time: now, Valid: true}

		// 记录解析结果，便于调试
		if result.ExpiryDate != nil {
			latestDomain.ExpiryDate = sql.NullTime{Time: *result.ExpiryDate, Valid: true}
			l.Infof("[DomainCheck] Parsed expiry date, domain=%s, expiryDate=%s",
				domain.Domain, result.ExpiryDate.Format("2006-01-02 15:04:05"))
		} else {
			l.Infof("[DomainCheck] No expiry date parsed, domain=%s (keeping existing value)", domain.Domain)
			// 如果解析失败，不清空旧值（保留现有值）
		}
		if result.CreationDate != nil {
			latestDomain.CreationDate = sql.NullTime{Time: *result.CreationDate, Valid: true}
			l.Infof("[DomainCheck] Parsed creation date, domain=%s, creationDate=%s",
				domain.Domain, result.CreationDate.Format("2006-01-02 15:04:05"))
		}
		if result.DeleteDate != nil {
			latestDomain.DeleteDate = sql.NullTime{Time: *result.DeleteDate, Valid: true}
			l.Infof("[DomainCheck] Parsed delete date, domain=%s, deleteDate=%s",
				domain.Domain, result.DeleteDate.Format("2006-01-02 15:04:05"))
		}
		if result.WhoisRaw != "" {
			latestDomain.WhoisRaw = sql.NullString{String: result.WhoisRaw, Valid: true}
		}

		// 直接更新（不需要重试，因为已经在锁内）
		updateErr := l.svcCtx.DomainsModel.Update(l.ctx, latestDomain)
		if updateErr != nil {
			domainLock.Unlock()
			l.Errorf("[DomainCheck] update domain failed, domain=%s, err=%v", domain.Domain, updateErr)
			continue
		}
		domainLock.Unlock()

		count++
		statusChanged := oldStatus != result.Status
		l.Infof("[DomainCheck] WHOIS query success, domain=%s, status=%s->%s, registrar=%s, canRegister=%v, expiryDate=%v, statusChanged=%v",
			latestDomain.Domain, oldStatus, result.Status, result.Registrar, result.CanRegister, result.ExpiryDate != nil, statusChanged)

		// 构造最新的域名信息，返回给前端用于局部刷新（使用更新后的最新记录）
		item := types.DomainItem{
			Id:          int64(latestDomain.Id),
			Domain:      latestDomain.Domain,
			Status:      latestDomain.Status,
			Registrar:   latestDomain.Registrar,
			WhoisStatus: latestDomain.WhoisStatus,
			Monitor:     latestDomain.Monitor,
			CreatedAt:   latestDomain.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if latestDomain.ExpiryDate.Valid {
			item.ExpiryDate = latestDomain.ExpiryDate.Time.Format("2006-01-02 15:04:05")
		}
		if latestDomain.CreationDate.Valid {
			item.CreationDate = latestDomain.CreationDate.Time.Format("2006-01-02 15:04:05")
		}
		if latestDomain.DeleteDate.Valid {
			item.DeleteDate = latestDomain.DeleteDate.Time.Format("2006-01-02 15:04:05")
		}
		if latestDomain.LastChecked.Valid {
			item.LastChecked = latestDomain.LastChecked.Time.Format("2006-01-02 15:04:05")
		}
		updatedItems = append(updatedItems, item)
	}

	l.Infof("[DomainCheck] batch WHOIS check completed, success=%d, total=%d", count, total)

	return &types.DomainCheckResp{
		Count: count,
		List:  updatedItems,
	}, nil
}
