package logic

import (
	"context"
	"time"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/pkg/domain"

	"github.com/zeromicro/go-zero/core/logx"
)

type DomainListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDomainListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DomainListLogic {
	return &DomainListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DomainListLogic) DomainList(req *types.DomainListReq) (resp *types.DomainListResp, err error) {
	total, err := l.svcCtx.DomainsModel.Count(l.ctx, req.Status, req.Keyword, req.Monitor, req.ExpiryStart, req.ExpiryEnd)
	if err != nil {
		return nil, err
	}

	list, err := l.svcCtx.DomainsModel.FindList(l.ctx, req.Page, req.PageSize, req.Status, req.Keyword, req.Monitor, req.ExpiryStart, req.ExpiryEnd, req.SortField, req.SortOrder)
	if err != nil {
		return nil, err
	}

	items := make([]types.DomainItem, 0, len(list))
	for _, d := range list {
		item := types.DomainItem{
			Id:          int64(d.Id),
			Domain:      d.Domain,
			Status:      d.Status,
			Registrar:   d.Registrar,
			WhoisStatus: d.WhoisStatus,
			Monitor:     d.Monitor,
			CreatedAt:   d.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		if d.ExpiryDate.Valid {
			item.ExpiryDate = d.ExpiryDate.Time.Format("2006-01-02 15:04:05")

			// 只有在域名已过期或处于过期相关状态时，才计算生命周期各阶段时间
			// registered/restricted 且未过期时，不填充生命周期阶段数据
			shouldCalculateStages := (d.Status != "registered" && d.Status != "restricted") ||
				d.ExpiryDate.Time.Before(time.Now()) ||
				d.Status == "expired" ||
				d.Status == "grace_period" ||
				d.Status == "redemption" ||
				d.Status == "pending_delete" ||
				d.Status == "available"

			if shouldCalculateStages {
				// 计算生命周期各阶段时间
				stages := domain.CalculateLifecycleStages(&d.ExpiryDate.Time)
				if stages != nil {
					item.LifecycleStages = &types.LifecycleStagesItem{
						ExpiryDate: stages.ExpiryDate.Format("2006-01-02 15:04:05"),
					}
					if stages.GracePeriodEnd != nil {
						item.LifecycleStages.GracePeriodEnd = stages.GracePeriodEnd.Format("2006-01-02 15:04:05")
					}
					if stages.RedemptionStart != nil {
						item.LifecycleStages.RedemptionStart = stages.RedemptionStart.Format("2006-01-02 15:04:05")
					}
					if stages.RedemptionEnd != nil {
						item.LifecycleStages.RedemptionEnd = stages.RedemptionEnd.Format("2006-01-02 15:04:05")
					}
					if stages.PendingDeleteStart != nil {
						item.LifecycleStages.PendingDeleteStart = stages.PendingDeleteStart.Format("2006-01-02 15:04:05")
					}
					if stages.PendingDeleteEnd != nil {
						item.LifecycleStages.PendingDeleteEnd = stages.PendingDeleteEnd.Format("2006-01-02 15:04:05")
					}
					if stages.AvailableDate != nil {
						item.LifecycleStages.AvailableDate = stages.AvailableDate.Format("2006-01-02 15:04:05")
					}
				}
			}
		}
		if d.CreationDate.Valid {
			item.CreationDate = d.CreationDate.Time.Format("2006-01-02 15:04:05")
		}
		if d.DeleteDate.Valid {
			item.DeleteDate = d.DeleteDate.Time.Format("2006-01-02 15:04:05")
		}
		if d.LastChecked.Valid {
			item.LastChecked = d.LastChecked.Time.Format("2006-01-02 15:04:05")
		}
		items = append(items, item)
	}

	return &types.DomainListResp{
		Total: total,
		List:  items,
	}, nil
}
