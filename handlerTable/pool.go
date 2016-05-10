package handlerTable

import (
	"sync"
)

// Pool 缓冲接口
type Pool interface {
	Get() Handler
	Put(x Handler)
}

// pool 缓冲结构
type pool struct {
	sync.Mutex
	items map[int]Handler
}

// NewPool 新建一个缓冲池
func NewPool() Pool {
	p := &pool{
		items: make(map[int]Handler),
	}
	return p
}

// Get 获取一个缓冲
func (p *pool) Get() (Handler) {
	p.Lock()
	defer p.Unlock()
	l := len(p.items)
	if l > 0 {
		if ff, ok := p.items[l]; ok {
			delete(p.items, l)
			return ff
		}
	}

	return nil
}

// Put 添加一个缓冲
func (p *pool) Put(ff Handler) {
	p.Lock()
	defer p.Unlock()
	if p.items == nil {
		p.items = make(map[int]Handler)
	}
	l := len(p.items)
	p.items[l + 1] = ff
}