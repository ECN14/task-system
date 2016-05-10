package queue

import (
	"errors"
)

// Queue 队列接口
type Queue interface {
	Put(channel string, key string, val []byte) error
	End(key string) error
	Get(channel string) (key string, val []byte, err error)
	Restore(key string) error
	User1(channel string) error
	Wait()
}

// ErrChannelIsNull 为空 
var ErrChannelIsNull = errors.New("-102 channel is null")
// ErrKeyNotExist Key不存在
var ErrKeyNotExist = errors.New("-204 key not exist")
// ErrMaxMemoryError 内存不够
var ErrMaxMemoryError = errors.New("-104 beyond memory")

// READY 等待状态 RESERVED 进行中状态 DELAYED 可以删除状态 BURIED 等待删除
const (
	_ uint8 = iota
	READY
	RESERVED
	DELAYED
)