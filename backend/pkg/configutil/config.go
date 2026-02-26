// Package configutil 提供项目统一的配置加载，数据库连接仅从配置文件读取，不使用 dsn 参数。
package configutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeromicro/go-zero/core/conf"
)

// ResolveConfigPath 支持多环境：当 explicitPath 等于 defaultPath 时，若设置了 APP_ENV 则优先使用同目录下的 {name}-{APP_ENV}.yaml。
// 例如 defaultPath=etc/domain.yaml、APP_ENV=prod 且存在 etc/domain-prod.yaml 时返回 etc/domain-prod.yaml。
func ResolveConfigPath(explicitPath, defaultPath string) string {
	if explicitPath != defaultPath {
		return explicitPath
	}
	env := strings.TrimSpace(os.Getenv("APP_ENV"))
	if env == "" {
		return explicitPath
	}
	dir := filepath.Dir(explicitPath)
	base := filepath.Base(explicitPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	envPath := filepath.Join(dir, name+"-"+env+ext)
	if _, err := os.Stat(envPath); err == nil {
		return envPath
	}
	return explicitPath
}

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
