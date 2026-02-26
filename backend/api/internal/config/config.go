package config

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf
	Auth struct {
		AccessSecret string
		AccessExpire int64
	}
	Mysql struct {
		DataSource string
	}
	Cache cache.CacheConf
	Aliyun struct {
		AccessKeyID     string
		AccessKeySecret string
		Enabled         bool
	}
	GoDaddy struct {
		APIKey    string
		APISecret string
		Sandbox   bool
		Enabled   bool
	}
	AutoSnatch struct {
		Enabled        bool
		MaxRetries     int
		CheckIntervals struct {
			Registered    int // 秒
			Expired       int
			GracePeriod   int
			Redemption    int
			PendingDelete int
		}
		Contact struct {
			FirstName    string
			LastName     string
			Email        string
			Phone        string
			Organization string
			Address1     string
			City         string
			State        string
			PostalCode   string
			Country      string
		}
	}
}
