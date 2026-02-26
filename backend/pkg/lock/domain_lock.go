package lock

import "sync"

// domainLockMap 用于确保同一时间只有一个任务在处理某个域名
var domainLockMap = sync.Map{} // map[string]*sync.Mutex

// GetDomainLock 获取域名的互斥锁，确保同一时间只有一个任务在处理某个域名
func GetDomainLock(domainName string) *sync.Mutex {
	lock, _ := domainLockMap.LoadOrStore(domainName, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
