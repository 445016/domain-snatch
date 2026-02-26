package svc

import (
	"domain-snatch/api/internal/config"
	"domain-snatch/model"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config             config.Config
	UsersModel         model.UsersModel
	DomainsModel       model.DomainsModel
	SnatchTasksModel   model.SnatchTasksModel
	NotifyLogsModel    model.NotifyLogsModel
	NotifySettingsModel model.NotifySettingsModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.Mysql.DataSource)
	return &ServiceContext{
		Config:             c,
		UsersModel:         model.NewUsersModel(conn),
		DomainsModel:       model.NewDomainsModel(conn),
		SnatchTasksModel:   model.NewSnatchTasksModel(conn),
		NotifyLogsModel:    model.NewNotifyLogsModel(conn),
		NotifySettingsModel: model.NewNotifySettingsModel(conn),
	}
}
