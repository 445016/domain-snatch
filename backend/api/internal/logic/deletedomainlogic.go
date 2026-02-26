package logic

import (
	"context"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteDomainLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteDomainLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDomainLogic {
	return &DeleteDomainLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteDomainLogic) DeleteDomain(req *types.DeleteDomainReq) (resp *types.CommonResp, err error) {
	err = l.svcCtx.DomainsModel.Delete(l.ctx, uint64(req.Id))
	if err != nil {
		return &types.CommonResp{Code: 1, Msg: "删除失败"}, nil
	}

	return &types.CommonResp{Code: 0, Msg: "删除成功"}, nil
}
