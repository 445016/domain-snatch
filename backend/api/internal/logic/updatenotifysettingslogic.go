package logic

import (
	"context"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateNotifySettingsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateNotifySettingsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateNotifySettingsLogic {
	return &UpdateNotifySettingsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateNotifySettingsLogic) UpdateNotifySettings(req *types.UpdateNotifySettingsReq) (resp *types.CommonResp, err error) {
	settings, err := l.svcCtx.NotifySettingsModel.FindFirst(l.ctx)
	if err != nil {
		return nil, errors.New("通知设置不存在")
	}

	settings.WebhookUrl = req.WebhookUrl
	settings.ExpireDays = req.ExpireDays
	settings.Enabled = req.Enabled

	err = l.svcCtx.NotifySettingsModel.Update(l.ctx, settings)
	if err != nil {
		return &types.CommonResp{Code: 1, Msg: "更新失败"}, nil
	}

	return &types.CommonResp{Code: 0, Msg: "更新成功"}, nil
}
