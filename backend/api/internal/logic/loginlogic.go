package logic

import (
	"context"
	"errors"
	"time"

	"domain-snatch/api/internal/svc"
	"domain-snatch/api/internal/types"

	"github.com/golang-jwt/jwt/v4"
	"github.com/zeromicro/go-zero/core/logx"
	"golang.org/x/crypto/bcrypt"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LoginLogic) Login(req *types.LoginReq) (resp *types.LoginResp, err error) {
	user, err := l.svcCtx.UsersModel.FindOneByUsername(l.ctx, req.Username)
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	now := time.Now().Unix()
	accessExpire := l.svcCtx.Config.Auth.AccessExpire
	accessToken, err := l.getJwtToken(
		l.svcCtx.Config.Auth.AccessSecret,
		now,
		accessExpire,
		int64(user.Id),
		user.Username,
		user.Role,
	)
	if err != nil {
		return nil, errors.New("生成token失败")
	}

	return &types.LoginResp{
		Token:    accessToken,
		ExpireAt: now + accessExpire,
	}, nil
}

func (l *LoginLogic) getJwtToken(secretKey string, iat, seconds int64, userId int64, username, role string) (string, error) {
	claims := make(jwt.MapClaims)
	claims["exp"] = iat + seconds
	claims["iat"] = iat
	claims["userId"] = userId
	claims["username"] = username
	claims["role"] = role
	token := jwt.New(jwt.SigningMethodHS256)
	token.Claims = claims
	return token.SignedString([]byte(secretKey))
}
