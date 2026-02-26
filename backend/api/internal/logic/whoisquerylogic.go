package logic

import (
	"context"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/pkg/whois"

	"github.com/zeromicro/go-zero/core/logx"
)

type WhoisQueryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewWhoisQueryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WhoisQueryLogic {
	return &WhoisQueryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *WhoisQueryLogic) WhoisQuery(req *types.WhoisQueryReq) (resp *types.WhoisQueryResp, err error) {
	if req.Domain == "" {
		return nil, errors.New("域名不能为空")
	}

	l.Infof("[WhoisQuery] start, domain=%s", req.Domain)

	// 与命令行工具保持一致：优先使用伪终端(PTY)方式的本地 whois 查询
	// 优先使用快速查询，如果失败再使用完整查询逻辑
	result, err := whois.QueryFast(req.Domain)
	if err != nil || result.WhoisRaw == "" || whois.IsOnlyIANAInfo(result.WhoisRaw) {
		// 快速查询失败，使用完整查询逻辑
		result, err = whois.Query(req.Domain)
	}
	if err != nil {
		l.Errorf("[WhoisQuery] failed, domain=%s, err=%v", req.Domain, err)
		return nil, errors.New("WHOIS查询失败: " + err.Error())
	}

	resp = &types.WhoisQueryResp{
		Domain:      result.Domain,
		Status:      result.Status,
		Registrar:   result.Registrar,
		WhoisStatus: result.WhoisStatus,
		CanRegister: result.CanRegister,
		WhoisRaw:    result.WhoisRaw,
	}

	if result.ExpiryDate != nil {
		resp.ExpiryDate = result.ExpiryDate.Format("2006-01-02 15:04:05")
	}
	if result.CreationDate != nil {
		resp.CreationDate = result.CreationDate.Format("2006-01-02 15:04:05")
	}
	if result.DeleteDate != nil {
		resp.DeleteDate = result.DeleteDate.Format("2006-01-02 15:04:05")
	}

	l.Infof("[WhoisQuery] success, domain=%s, status=%s, whoisStatus=%s, registrar=%s, canRegister=%v",
		resp.Domain, resp.Status, resp.WhoisStatus, resp.Registrar, resp.CanRegister)

	return resp, nil
}
