package logic

import (
	"context"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SnatchTaskListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSnatchTaskListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SnatchTaskListLogic {
	return &SnatchTaskListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SnatchTaskListLogic) SnatchTaskList(req *types.SnatchTaskListReq) (resp *types.SnatchTaskListResp, err error) {
	total, err := l.svcCtx.SnatchTasksModel.Count(l.ctx, req.Status)
	if err != nil {
		return nil, err
	}

	list, err := l.svcCtx.SnatchTasksModel.FindList(l.ctx, req.Page, req.PageSize, req.Status)
	if err != nil {
		return nil, err
	}

	items := make([]types.SnatchTaskItem, 0, len(list))
	for _, t := range list {
		item := types.SnatchTaskItem{
			Id:              int64(t.Id),
			DomainId:        int64(t.DomainId),
			Domain:          t.Domain,
			Status:          t.Status,
			Priority:        t.Priority,
			TargetRegistrar: t.TargetRegistrar,
			AutoRegister:    t.AutoRegister,
			RetryCount:      t.RetryCount,
			CreatedAt:       t.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt:       t.UpdatedAt.Format("2006-01-02 15:04:05"),
		}
		if t.Result.Valid {
			item.Result = t.Result.String
		}
		if t.LastError.Valid {
			item.LastError = t.LastError.String
		}
		items = append(items, item)
	}

	return &types.SnatchTaskListResp{
		Total: total,
		List:  items,
	}, nil
}
