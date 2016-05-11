package cache

import (
	"time"
	"errors"
)
// ErrTimeOut 获取数据超时
var ErrTimeOut = errors.New("-101 time out")
// ErrKeyNotExist Key不存在
var ErrKeyNotExist = errors.New("-203 key not exist")
// ErrMaxMemoryError 内存不够
var ErrMaxMemoryError = errors.New("-103 beyond memory")
// Cache 缓存接口
type Cache interface {
	Set(key string, val []byte) error
	Get(key string) ([]byte, bool)
	Cover(key string, val []byte) error
	InitNil() (key string)
	GetAndTimeOut(key string, timeout time.Duration) ([]byte, error)
	Delete(key string) error
	IsExist(key string) bool
	ClearAll() error
	StartAndGC() error
}
