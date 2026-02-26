package logic

import (
	"context"
	"errors"
	"strings"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddDomainLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddDomainLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddDomainLogic {
	return &AddDomainLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AddDomainLogic) AddDomain(req *types.AddDomainReq) (resp *types.AddDomainResp, err error) {
	domain := strings.TrimSpace(strings.ToLower(req.Domain))
	if domain == "" {
		return nil, errors.New("域名不能为空")
	}

	// 检查是否已存在
	_, err = l.svcCtx.DomainsModel.FindOneByDomain(l.ctx, domain)
	if err == nil {
		return nil, errors.New("域名已存在")
	}

	result, err := l.svcCtx.DomainsModel.Insert(l.ctx, &model.Domains{
		Domain:  domain,
		Status:  "unknown",
		Monitor: 0,
	})
	if err != nil {
		return nil, errors.New("添加域名失败")
	}

	id, _ := result.LastInsertId()
	return &types.AddDomainResp{
		Id: id,
	}, nil
}
