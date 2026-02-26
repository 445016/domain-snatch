package logic

import (
	"context"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DashboardStatsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDashboardStatsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DashboardStatsLogic {
	return &DashboardStatsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DashboardStatsLogic) DashboardStats() (resp *types.DashboardStatsResp, err error) {
	totalDomains, _ := l.svcCtx.DomainsModel.Count(l.ctx, "", "", -1, "", "")
	monitorCount, _ := l.svcCtx.DomainsModel.CountMonitor(l.ctx)
	expiringSoon, _ := l.svcCtx.DomainsModel.CountExpiringSoon(l.ctx, 30)
	availableCount, _ := l.svcCtx.DomainsModel.CountByStatus(l.ctx, "available")
	snatchPending, _ := l.svcCtx.SnatchTasksModel.CountByStatus(l.ctx, "pending")
	snatchSuccess, _ := l.svcCtx.SnatchTasksModel.CountByStatus(l.ctx, "success")

	return &types.DashboardStatsResp{
		TotalDomains:   totalDomains,
		MonitorCount:   monitorCount,
		ExpiringSoon:   expiringSoon,
		AvailableCount: availableCount,
		SnatchPending:  snatchPending,
		SnatchSuccess:  snatchSuccess,
	}, nil
}
