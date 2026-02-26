package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var _ NotifyLogsModel = (*customNotifyLogsModel)(nil)

type (
	// NotifyLogsModel is an interface to be customized, add more methods here,
	// and implement the added methods in customNotifyLogsModel.
	NotifyLogsModel interface {
		notifyLogsModel
		withSession(session sqlx.Session) NotifyLogsModel
	}

	customNotifyLogsModel struct {
		*defaultNotifyLogsModel
	}
)

// NewNotifyLogsModel returns a model for the database table.
func NewNotifyLogsModel(conn sqlx.SqlConn) NotifyLogsModel {
	return &customNotifyLogsModel{
		defaultNotifyLogsModel: newNotifyLogsModel(conn),
	}
}

func (m *customNotifyLogsModel) withSession(session sqlx.Session) NotifyLogsModel {
	return NewNotifyLogsModel(sqlx.NewSqlConnFromSession(session))
}
