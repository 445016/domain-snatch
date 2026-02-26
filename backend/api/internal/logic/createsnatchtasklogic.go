package logic

import (
	"context"
	"database/sql"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateSnatchTaskLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateSnatchTaskLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateSnatchTaskLogic {
	return &CreateSnatchTaskLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateSnatchTaskLogic) CreateSnatchTask(req *types.CreateSnatchTaskReq) (resp *types.CreateSnatchTaskResp, err error) {
	if req.Domain == "" {
		return nil, errors.New("域名不能为空")
	}

	result, err := l.svcCtx.SnatchTasksModel.Insert(l.ctx, &model.SnatchTasks{
		DomainId:        uint64(req.DomainId),
		Domain:          req.Domain,
		Status:          "pending",
		Priority:        req.Priority,
		TargetRegistrar: req.TargetRegistrar,
		AutoRegister:    req.AutoRegister,
		RetryCount:      0,
		LastError:       sql.NullString{},
		Result:          sql.NullString{},
	})
	if err != nil {
		return nil, errors.New("创建抢注任务失败")
	}

	id, _ := result.LastInsertId()
	return &types.CreateSnatchTaskResp{
		Id: id,
	}, nil
}
