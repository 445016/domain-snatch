package logic

import (
	"context"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetNotifySettingsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetNotifySettingsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetNotifySettingsLogic {
	return &GetNotifySettingsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetNotifySettingsLogic) GetNotifySettings() (resp *types.NotifySettingsResp, err error) {
	settings, err := l.svcCtx.NotifySettingsModel.FindFirst(l.ctx)
	if err != nil {
		return &types.NotifySettingsResp{
			ExpireDays: 30,
			Enabled:    0,
		}, nil
	}

	return &types.NotifySettingsResp{
		Id:         int64(settings.Id),
		WebhookUrl: settings.WebhookUrl,
		ExpireDays: settings.ExpireDays,
		Enabled:    settings.Enabled,
	}, nil
}
