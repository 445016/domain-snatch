package logic

import (
	"context"
	"database/sql"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateSnatchTaskLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateSnatchTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateSnatchTaskLogic {
	return &UpdateSnatchTaskLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateSnatchTaskLogic) UpdateSnatchTask(req *types.UpdateSnatchTaskReq) (resp *types.CommonResp, err error) {
	task, err := l.svcCtx.SnatchTasksModel.FindOne(l.ctx, uint64(req.Id))
	if err != nil {
		return nil, errors.New("任务不存在")
	}

	task.Status = req.Status
	if req.Result != "" {
		task.Result = sql.NullString{String: req.Result, Valid: true}
	}

	err = l.svcCtx.SnatchTasksModel.Update(l.ctx, task)
	if err != nil {
		return &types.CommonResp{Code: 1, Msg: "更新失败"}, nil
	}

	return &types.CommonResp{Code: 0, Msg: "更新成功"}, nil
}
