// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package handler

import (
	"net/http"

	"domain-snatch/api/internal/logic"
	"domain-snatch/api/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func getNotifySettingsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewGetNotifySettingsLogic(r.Context(), svcCtx)
		resp, err := l.GetNotifySettings()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
