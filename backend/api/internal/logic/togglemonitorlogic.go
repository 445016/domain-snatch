package logic

import (
	"context"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ToggleMonitorLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewToggleMonitorLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ToggleMonitorLogic {
	return &ToggleMonitorLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ToggleMonitorLogic) ToggleMonitor(req *types.ToggleMonitorReq) (resp *types.CommonResp, err error) {
	domain, err := l.svcCtx.DomainsModel.FindOne(l.ctx, uint64(req.Id))
	if err != nil {
		return nil, errors.New("域名不存在")
	}

	domain.Monitor = req.Monitor
	err = l.svcCtx.DomainsModel.Update(l.ctx, domain)
	if err != nil {
		return &types.CommonResp{Code: 1, Msg: "更新失败"}, nil
	}

	return &types.CommonResp{Code: 0, Msg: "更新成功"}, nil
}
