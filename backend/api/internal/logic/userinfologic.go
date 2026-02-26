package logic

import (
	"context"
	"encoding/json"
	"errors"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserInfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUserInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserInfoLogic {
	return &UserInfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UserInfoLogic) UserInfo() (resp *types.UserInfoResp, err error) {
	userId, err := l.ctx.Value("userId").(json.Number).Int64()
	if err != nil {
		return nil, errors.New("获取用户信息失败")
	}

	user, err := l.svcCtx.UsersModel.FindOne(l.ctx, uint64(userId))
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	return &types.UserInfoResp{
		Id:       int64(user.Id),
		Username: user.Username,
		Role:     user.Role,
	}, nil
}
