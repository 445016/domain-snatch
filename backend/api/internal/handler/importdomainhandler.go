package handler

import (
	"net/http"

	"domain-snatch/api/internal/logic"
	"domain-snatch/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func importDomainHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewImportDomainLogic(r.Context(), svcCtx, r)
		resp, err := l.ImportDomain()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
