package cache

import (
	"time"
	"sync"
	"../config"
	"../handlerTable"
	"strconv"
	"unsafe"
)

// Item 存储单元结构
type Item struct {
	val          []byte
	createdTime  time.Time
	lifespan     time.Duration
	nilLifespan  time.Duration
	handlerTable handlerTable.Table
}

// Bucket 内存缓存存储结构
type Bucket struct {
	sync.RWMutex
	dur           time.Duration
	items         map[string]*Item
	size          int64
	maxMemorySize int64
}

// NewCache 新建一个存储
func NewCache() Cache {
	cache := &Bucket{
		dur: config.CacheDur,
		items:make(map[string]*Item),
		size:0,
		maxMemorySize:config.CacheMaxMemory,
	}
	cache.StartAndGC()

	return cache
}

// Set 设置数据
func (bc *Bucket) Set(key string, val []byte) error {
	bc.Lock()
	defer bc.Unlock()
	var size = int64(unsafe.Sizeof(val)) + bc.size
	if size > bc.maxMemorySize {
		return ErrMaxMemoryError
	}

	bc.items[key] = &Item{
		val: val,
		createdTime : time.Now(),
		lifespan: config.CahceLifespan,
		nilLifespan: config.CahceNilLifespan,
		handlerTable: handlerTable.NewTable(),
	}
	bc.size = size
	return nil
}

// Cover 覆盖设置一个字
func (bc *Bucket) Cover(key string, val []byte) error {
	bc.Lock()
	defer bc.Unlock()

	var size = int64(unsafe.Sizeof(val)) + bc.size
	if size > bc.maxMemorySize {
		return ErrMaxMemoryError
	}
	if v, ok := bc.items[key]; ok {
		v.val = val
		v.createdTime = time.Now()
		bc.size = size
		go v.handlerTable.CallBack()

		return nil
	}

	return ErrKeyNotExist
}

// Get 获取数据
func (bc *Bucket) Get(key string) ([]byte, bool) {
	bc.RLock()
	defer bc.RUnlock()

	if itm, ok := bc.items[key]; ok {
		if !itm.isExpire() {
			return itm.val, ok
		}
	}

	return nil, false
}

// InitNil 添加一个nil
func (bc *Bucket) InitNil() (key string) {
	bc.Lock()
	defer bc.Unlock()
	dur := time.Now().UnixNano()
	for {
		key = strconv.FormatInt(int64(dur), 32)
		if _, ok := bc.items[key]; ok {
			dur++
			continue
		}
		bc.items[key] = &Item{
			val: nil,
			createdTime : time.Now(),
			lifespan: config.CahceLifespan,
			nilLifespan: config.CahceNilLifespan,
			handlerTable: handlerTable.NewTable(),
		}

		return key
	}

}

// Delete 删除数据
func (bc *Bucket) Delete(key string) error {
	bc.Lock()
	defer bc.Unlock()

	itm, ok := bc.items[key];
	if !ok {
		return ErrKeyNotExist
	}
	delete(bc.items, key)
	bc.size -= int64(unsafe.Sizeof(itm.val))

	return nil
}

// IsExist 判定数据是否存在
func (bc *Bucket) IsExist(key string) bool {
	bc.RLock()
	defer bc.RUnlock()

	if v, ok := bc.items[key]; ok {
		return !v.isExpire()
	}

	return false
}

// isExpire 判定数据是否过期
func (mi *Item) isExpire() bool {
	if mi.val == nil {
		if mi.nilLifespan == 0 {
			return false
		}
		return time.Now().Sub(mi.createdTime) > mi.nilLifespan
	}
	if mi.lifespan == 0 {
		return false
	}
	return time.Now().Sub(mi.createdTime) > mi.lifespan
}

// StartAndGC 开启周期删除数据
func (bc *Bucket) StartAndGC() error {
	go bc.vaccuum()

	return nil
}

// vaccuum 周期性删除过期数据
func (bc *Bucket) vaccuum() {
	ticker := time.Tick(bc.dur)
	for {
		<-ticker
		if bc.items == nil {
			continue
		}

		for name := range bc.items {
			bc.itemExpired(name)
		}
	}
}

// itemExpired 验证数据是否过期，如果过期则删除
func (bc *Bucket) itemExpired(name string) bool {
	bc.Lock()
	defer bc.Unlock()

	itm, ok := bc.items[name]
	if !ok {
		return true
	}
	if itm.isExpire() {
		delete(bc.items, name)
		if _, ok = bc.items[name]; ok {
			return true
		}
		bc.size -= int64(unsafe.Sizeof(itm.val))
		return true
	}
	return false
}

// ClearAll 清除所有数据
func (bc *Bucket) ClearAll() error {
	bc.Lock()
	defer bc.Unlock()

	bc.items = make(map[string]*Item)
	bc.size = 0

	return nil
}

// GetAndTimeOut 设置超时获取数据
func (bc *Bucket) GetAndTimeOut(key string, timeout time.Duration) ([]byte, error) {
	ch := bc.handleFunc(key)
	if ch == nil {
		return nil, ErrKeyNotExist
	}
	select {
	case <-time.After(timeout):
		return nil, ErrTimeOut
	case <-ch:
		b, ok := bc.Get(key)
		if ok {
			return b, nil
		}
		return nil, ErrKeyNotExist
	}

}

// handleFunc 注册func
func (bc *Bucket) handleFunc(key string) chan bool {

	bc.RLock()
	defer bc.RUnlock()
	var ch chan bool
	if v, ok := bc.items[key]; ok {
		ch = make(chan bool, 1)
		f := func() {
			select {
			case <-time.After(1 * time.Minute):
				return
			case ch <- true:
				return
			}
		}
		if v.val == nil {
			v.handlerTable.HandlerFunc(f)
		} else {
			f()
		}

	}

	return ch
}
