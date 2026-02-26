package logic

import (
	"context"
	"errors"
	"net/http"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"domain-snatch/model"
	"domain-snatch/pkg/excel"

	"github.com/zeromicro/go-zero/core/logx"
)

type ImportDomainLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
	r      *http.Request
}

func NewImportDomainLogic(ctx context.Context, svcCtx *svc.ServiceContext, r *http.Request) *ImportDomainLogic {
	return &ImportDomainLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
		r:      r,
	}
}

func (l *ImportDomainLogic) ImportDomain() (resp *types.ImportDomainResp, err error) {
	file, _, err := l.r.FormFile("file")
	if err != nil {
		return nil, errors.New("请上传Excel文件")
	}
	defer file.Close()

	domains, err := excel.ParseDomainsFromReader(file)
	if err != nil {
		return nil, errors.New("解析Excel文件失败: " + err.Error())
	}

	var success, failed int64
	for _, domain := range domains {
		// 检查是否已存在
		_, err := l.svcCtx.DomainsModel.FindOneByDomain(l.ctx, domain)
		if err == nil {
			failed++
			continue
		}

		_, err = l.svcCtx.DomainsModel.Insert(l.ctx, &model.Domains{
			Domain:  domain,
			Status:  "unknown",
			Monitor: 0,
		})
		if err != nil {
			failed++
			continue
		}
		success++
	}

	return &types.ImportDomainResp{
		Total:   int64(len(domains)),
		Success: success,
		Failed:  failed,
	}, nil
}
