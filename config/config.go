package config

import (
	"time"
)

// 字节单位
const (
	_ = iota
	KB int64 = 1 << (10 * iota)
	MB
	GB
	TB
)
// 缓存配置
const (
	// CacheDur GC心态周期
	CacheDur time.Duration = 1 * time.Minute
	// CahceLifespan cache生命周期
	CahceLifespan time.Duration = 1 * time.Minute
	// CahceNilLifespan 值为nil 生命周期
	CahceNilLifespan time.Duration = 10 * time.Minute
	// CacheMaxMemory 现在内存使用
	CacheMaxMemory = 1 * GB
)

// 队列配置
const (
	QueueDur time.Duration = 1 * time.Minute
	// QueueMaxMemory 内存限制
	QueueMaxMemory = 1 * GB
)
