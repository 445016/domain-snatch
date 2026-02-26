// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package handler

import (
	"net/http"

	"domain-snatch/api/internal/logic"
	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func toggleMonitorHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ToggleMonitorReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewToggleMonitorLogic(r.Context(), svcCtx)
		resp, err := l.ToggleMonitor(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
