package model

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ NotifySettingsModel = (*customNotifySettingsModel)(nil)

type (
	NotifySettingsModel interface {
		notifySettingsModel
		withSession(session sqlx.Session) NotifySettingsModel
		FindFirst(ctx context.Context) (*NotifySettings, error)
	}

	customNotifySettingsModel struct {
		*defaultNotifySettingsModel
	}
)

func NewNotifySettingsModel(conn sqlx.SqlConn) NotifySettingsModel {
	return &customNotifySettingsModel{
		defaultNotifySettingsModel: newNotifySettingsModel(conn),
	}
}

func (m *customNotifySettingsModel) withSession(session sqlx.Session) NotifySettingsModel {
	return NewNotifySettingsModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customNotifySettingsModel) FindFirst(ctx context.Context) (*NotifySettings, error) {
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY `id` ASC LIMIT 1", notifySettingsRows, m.table)
	var resp NotifySettings
	err := m.conn.QueryRowCtx(ctx, &resp, query)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}
