package queue

import (
	"sync"
)

// List 链表接口
type List interface {
	Put(x string)
	Get() (x string)
	Len() int
}

// list 链表结构
type list struct {
	sync.RWMutex
	items map[uint64]string
	nn    uint64
}

// NewList 新建链表
func NewList() List {
	l := &list{items: make(map[uint64]string)}

	return l
}

// Put 添加一个值
func (l *list) Put(x string) {
	l.Lock()
	defer l.Unlock()
	l.items[l.nn] = x
	l.nn++
}

// Get 获取一个值
func (l *list) Get() (x string) {
	l.Lock()
	defer l.Unlock()

	d := l.nn - uint64(len(l.items))

	if x, ok := l.items[d]; ok {
		delete(l.items, d)
		return x
	}

	return
}

// Len 获取长度
func (l *list) Len() int {
	l.RLock()
	defer l.RUnlock()

	return len(l.items)
}

// Init 初始化
func (l *list) Init() {
	l.Lock()
	defer l.Unlock()

	l.items = make(map[uint64]string)
	l.nn = 0
}
