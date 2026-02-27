// Package configutil 提供项目统一的配置加载，数据库连接仅从配置文件读取，不使用 dsn 参数。
package configutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeromicro/go-zero/core/conf"
)

// DefaultConfigSearchPaths 默认配置搜索顺序：从 backend 或 backend/api 执行都能找到 backend/etc/。
var DefaultConfigSearchPaths = []string{"etc/domain.yaml", "../etc/domain.yaml"}

// ResolveConfigPath 支持多环境：当 explicitPath 等于 defaultPath 时，若设置了 APP_ENV 则优先使用同目录下的 {name}-{APP_ENV}.yaml。
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

// ResolveConfigPathWithSearch 当使用默认配置时，按 searchPaths 顺序找第一个存在的文件并应用 APP_ENV，支持从 backend 或 backend/api 执行。
func ResolveConfigPathWithSearch(explicitPath string, searchPaths []string) string {
	var isDefault bool
	for _, p := range searchPaths {
		if explicitPath == p {
			isDefault = true
			break
		}
	}
	if !isDefault {
		return ResolveConfigPath(explicitPath, explicitPath)
	}
	for _, p := range searchPaths {
		resolved := ResolveConfigPath(p, p)
		if _, err := os.Stat(resolved); err == nil {
			return resolved
		}
	}
	return ResolveConfigPath(searchPaths[0], searchPaths[0])
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
