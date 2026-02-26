package whois

import (
	"fmt"
	"sync"
	"time"
)

var (
	aliyunClient *AliyunClient
	aliyunMutex  sync.RWMutex

	// 速率限制器：控制查询频率，防止被限流
	rateLimiter = NewRateLimiter()
)

// RateLimiter 速率限制器
type RateLimiter struct {
	mu           sync.Mutex
	lastQuery    time.Time
	minInterval  time.Duration // 最小查询间隔
	failureCount int           // 连续失败次数
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		minInterval:  500 * time.Millisecond, // 默认最小间隔500毫秒（优化速度）
		failureCount: 0,
	}
}

// Wait 等待直到可以发起下一次查询
func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 根据失败次数计算指数退避间隔
	interval := r.minInterval
	if r.failureCount > 0 {
		// 指数退避：2s, 4s, 8s, 16s... 最大60s
		backoff := r.minInterval * time.Duration(1<<uint(r.failureCount))
		if backoff > 60*time.Second {
			backoff = 60 * time.Second
		}
		interval = backoff
	}

	elapsed := time.Since(r.lastQuery)
	if elapsed < interval {
		time.Sleep(interval - elapsed)
	}

	r.lastQuery = time.Now()
}

// RecordSuccess 记录成功查询，重置失败计数
func (r *RateLimiter) RecordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCount = 0
}

// RecordFailure 记录失败查询，增加失败计数
func (r *RateLimiter) RecordFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failureCount < 5 { // 最多退避到 2^5 * 2s = 64s
		r.failureCount++
	}
}

// SetAliyunCredentials 设置阿里云凭证
func SetAliyunCredentials(accessKeyID, accessKeySecret string) error {
	client, err := NewAliyunClient(accessKeyID, accessKeySecret)
	if err != nil {
		return err
	}

	aliyunMutex.Lock()
	aliyunClient = client
	aliyunMutex.Unlock()

	return nil
}

// QueryWithRateLimit 带速率限制的查询（用于批量操作）
// 注意：单个查询应该使用 QueryFast 或 Query，不需要速率限制
// 优化：直接使用 Query 函数，避免重复调用 QueryFast
func QueryWithRateLimit(domain string) (*Result, error) {
	rateLimiter.Wait()
	// 直接使用 Query 函数，它内部会尝试多种方式（QueryFast -> QueryWithPTY -> QueryWithTimeout -> QueryHTTP）
	// 避免在 QueryWithRateLimit 中先调用 QueryFast，然后在 Query 中又调用一次，造成重复查询
	result, err := Query(domain)
	if err != nil {
		rateLimiter.RecordFailure()
	} else {
		rateLimiter.RecordSuccess()
	}
	return result, err
}

// QueryUnified 统一查询接口，自动选择最佳方式
// 优先级：阿里云API > likexian库 > whois命令
func QueryUnified(domain string) (*Result, error) {
	aliyunMutex.RLock()
	hasAliyun := aliyunClient != nil
	client := aliyunClient
	aliyunMutex.RUnlock()

	// 如果配置了阿里云，优先使用
	if hasAliyun {
		result, err := client.QueryDomain(domain)
		if err == nil {
			return result, nil
		}
		// 阿里云查询失败，fallback到其他方式
		fmt.Printf("阿里云查询失败: %v, 使用备用方式...\n", err)
	}

	// 尝试使用Go whois库
	result, err := QueryWithLib(domain)
	if err == nil {
		return result, nil
	}

	// 最后尝试whois命令
	return QueryWithTimeout(domain, 10*time.Second)
}
