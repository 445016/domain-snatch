package model

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ SnatchTasksModel = (*customSnatchTasksModel)(nil)

type (
	SnatchTasksModel interface {
		snatchTasksModel
		withSession(session sqlx.Session) SnatchTasksModel
		FindList(ctx context.Context, page, pageSize int64, status string) ([]*SnatchTasks, error)
		Count(ctx context.Context, status string) (int64, error)
		CountByStatus(ctx context.Context, status string) (int64, error)
		FindPending(ctx context.Context) ([]*SnatchTasks, error)
	}

	customSnatchTasksModel struct {
		*defaultSnatchTasksModel
	}
)

func NewSnatchTasksModel(conn sqlx.SqlConn) SnatchTasksModel {
	return &customSnatchTasksModel{
		defaultSnatchTasksModel: newSnatchTasksModel(conn),
	}
}

func (m *customSnatchTasksModel) withSession(session sqlx.Session) SnatchTasksModel {
	return NewSnatchTasksModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customSnatchTasksModel) FindList(ctx context.Context, page, pageSize int64, status string) ([]*SnatchTasks, error) {
	where := "1=1"
	args := make([]interface{}, 0)
	if status != "" {
		where += " AND `status` = ?"
		args = append(args, status)
	}
	offset := (page - 1) * pageSize
	args = append(args, offset, pageSize)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY `priority` DESC, `id` DESC LIMIT ?, ?", snatchTasksRows, m.table, where)
	var resp []*SnatchTasks
	err := m.conn.QueryRowsCtx(ctx, &resp, query, args...)
	return resp, err
}

func (m *customSnatchTasksModel) Count(ctx context.Context, status string) (int64, error) {
	where := "1=1"
	args := make([]interface{}, 0)
	if status != "" {
		where += " AND `status` = ?"
		args = append(args, status)
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", m.table, where)
	var count int64
	err := m.conn.QueryRowCtx(ctx, &count, query, args...)
	return count, err
}

func (m *customSnatchTasksModel) CountByStatus(ctx context.Context, status string) (int64, error) {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE `status` = ?", m.table)
	var count int64
	err := m.conn.QueryRowCtx(ctx, &count, query, status)
	return count, err
}

func (m *customSnatchTasksModel) FindPending(ctx context.Context) ([]*SnatchTasks, error) {
	query := fmt.Sprintf("SELECT %s FROM %s WHERE `status` = 'pending' ORDER BY `priority` DESC, `id` ASC", snatchTasksRows, m.table)
	var resp []*SnatchTasks
	err := m.conn.QueryRowsCtx(ctx, &resp, query)
	return resp, err
}
