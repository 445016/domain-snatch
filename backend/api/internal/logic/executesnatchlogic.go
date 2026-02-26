package logic

import (
	"context"
	"database/sql"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/model"
	"domain-snatch/pkg/snatch"

	"github.com/zeromicro/go-zero/core/logx"
)

type ExecuteSnatchLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExecuteSnatchLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExecuteSnatchLogic {
	return &ExecuteSnatchLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ExecuteSnatchLogic) ExecuteSnatch(req *types.ExecuteSnatchReq) (resp *types.ExecuteSnatchResp, err error) {
	var task *model.SnatchTasks

	if req.TaskId > 0 {
		task, err = l.svcCtx.SnatchTasksModel.FindOne(l.ctx, uint64(req.TaskId))
		if err != nil {
			return nil, errors.New("抢注任务不存在")
		}
		if task.Status != "pending" && task.Status != "processing" {
			return nil, errors.New("任务状态不允许执行，仅 pending/processing 可触发")
		}
	} else if req.Domain != "" {
		// 按域名：先查是否已有 pending 任务
		pending, err := l.svcCtx.SnatchTasksModel.FindPending(l.ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range pending {
			if t.Domain == req.Domain {
				task = t
				break
			}
		}
		if task == nil {
			// 创建一条新任务再执行
			res, err := l.svcCtx.SnatchTasksModel.Insert(l.ctx, &model.SnatchTasks{
				DomainId:        0,
				Domain:          req.Domain,
				Status:          "pending",
				Priority:        0,
				TargetRegistrar: "",
				AutoRegister:    1,
				RetryCount:      0,
				LastError:       sql.NullString{},
				Result:          sql.NullString{},
			})
			if err != nil {
				return nil, errors.New("创建抢注任务失败")
			}
			id, _ := res.LastInsertId()
			task, _ = l.svcCtx.SnatchTasksModel.FindOne(l.ctx, uint64(id))
			if task == nil {
				return nil, errors.New("创建任务后查询失败")
			}
		}
	} else {
		return nil, errors.New("请指定 domain 或 taskId")
	}

	settings, _ := l.svcCtx.NotifySettingsModel.FindFirst(l.ctx)
	exec := l.buildExecutor(settings)
	if err := exec.Execute(l.ctx, task); err != nil {
		l.Errorf("[ExecuteSnatch] execute failed, domain=%s, err=%v", task.Domain, err)
		return &types.ExecuteSnatchResp{Msg: "抢注已触发，执行过程中出错: " + err.Error()}, nil
	}
	return &types.ExecuteSnatchResp{Msg: "抢注已触发，请关注飞书通知"}, nil
}

func (l *ExecuteSnatchLogic) buildExecutor(settings *model.NotifySettings) *snatch.Executor {
	exec := &snatch.Executor{
		GodaddyClient: l.svcCtx.GodaddyClient,
		SnatchTasks:   l.svcCtx.SnatchTasksModel,
		NotifyLogs:    l.svcCtx.NotifyLogsModel,
		MaxRetries:    3,
	}
	if settings != nil && settings.Enabled == 1 && settings.WebhookUrl != "" {
		exec.WebhookURL = settings.WebhookUrl
	}
	c := l.svcCtx.Config.AutoSnatch
	if c.MaxRetries > 0 {
		exec.MaxRetries = c.MaxRetries
	}
	exec.Contact = snatch.Contact{
		FirstName:    c.Contact.FirstName,
		LastName:     c.Contact.LastName,
		Email:        c.Contact.Email,
		Phone:        c.Contact.Phone,
		Organization: c.Contact.Organization,
		Address1:     c.Contact.Address1,
		City:         c.Contact.City,
		State:        c.Contact.State,
		PostalCode:   c.Contact.PostalCode,
		Country:      c.Contact.Country,
	}
	return exec
}
