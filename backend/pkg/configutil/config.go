// Package configutil 提供项目统一的配置加载，数据库连接仅从配置文件读取，不使用 dsn 参数。
package configutil

import (
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
)

// DBConfig 仅包含数据库连接配置，与 api/etc/domain.yaml 中 Mysql 段结构一致。
type DBConfig struct {
	Mysql struct {
		DataSource string
	}
}

// LoadDataSource 从指定配置文件路径加载并返回 Mysql.DataSource。
// 若 DataSource 为空则返回错误。供命令行工具与需要读库的组件统一使用。
func LoadDataSource(configPath string) (string, error) {
	var cfg DBConfig
	if err := conf.Load(configPath, &cfg); err != nil {
		return "", fmt.Errorf("加载配置 %s: %w", configPath, err)
	}
	if cfg.Mysql.DataSource == "" {
		return "", fmt.Errorf("配置文件中 Mysql.DataSource 为空")
	}
	return cfg.Mysql.DataSource, nil
}
