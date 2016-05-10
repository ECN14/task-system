package queue

import (
	"sync"
	"../handlerTable"
	"../config"
	"time"
	"unsafe"
)

// beanstalk 队列Job存储体
type beanstalk struct {
	sync.RWMutex
	sync.WaitGroup
	tube          map[string]*tb
	db            map[string]*job
	dur           time.Duration
	size          int64
	maxMemorySize int64
}

// 一个Job任务
type job struct {
	channel string
	key     string
	data    []byte
	status  uint8
}

// tb 存储Tube 一个Tube存储N个Job
type tb struct {
	l            List
	handlerTable handlerTable.Table
	updateTime   time.Time
}

// NewQueue 新建一个队列
func NewQueue() Queue {
	mg := &beanstalk{
		tube: make(map[string]*tb),
		db: make(map[string]*job),
		dur: config.QueueDur,
		size: 0,
		maxMemorySize:config.QueueMaxMemory,
	}

	return mg
}

// Put 添加一个任务
func (q *beanstalk) Put(channel string, key string, val []byte) error {
	j := &job{
		channel:channel,
		key:key,
		data: val,
		status: READY,

	}

	q.Lock()
	defer q.Unlock()

	size := q.size + int64(unsafe.Sizeof(val))
	if size > q.maxMemorySize {
		return ErrMaxMemoryError
	}

	g, ok := q.tube[channel]
	if !ok {
		g = &tb{
			l: NewList(),
			handlerTable: handlerTable.NewTable(),
		}
		q.tube[channel] = g
	}
	g.put(key)
	q.db[key] = j
	q.size = size

	return nil
}

func (g *tb) put(key string) {
	g.l.Put(key)
	go g.handlerTable.CallBack()
	g.updateTime = time.Now()
}

// Done 完成一个任务
func (q *beanstalk) End(key string) error {
	q.Lock()
	defer q.Unlock()

	if j, ok := q.db[key]; ok {
		if j.status == READY {
			q.Done()
			j.status = DELAYED
		}

		return nil
	}
	return ErrKeyNotExist
}

// Get 获取一个任务
func (q *beanstalk) Get(channel string) (key string, val []byte, err error) {
	q.Lock()
	defer q.Unlock()

	g, ok := q.tube[channel];
	if !ok {
		g = &tb{
			l: NewList(),
			handlerTable: handlerTable.NewTable(),
		}
		q.tube[channel] = g
	}

	for {
		key = g.l.Get()
		if key != "" {
			if j, ok := q.db[key]; ok {
				if j.status == READY {
					j.status = RESERVED
					q.Add(1)
					return j.key, j.data, nil
				}
				continue
			}
		}

		return key, nil, ErrChannelIsNull
	}
}

// Restore 还原一个任务
func (q *beanstalk) Restore(key string) error {
	q.Lock()
	defer q.Unlock()

	if j, ok := q.db[key]; ok && j.status == RESERVED {
		if g, ok := q.tube[j.channel]; ok {
			g.l.Put(key)
			j.status = READY
		}
	}

	return nil
}

// User1 添加一个通知
func (q *beanstalk) User1(channel string) error {
	<-q.handleFunc(channel)
	return nil
}

// handleFunc 注册func
func (q *beanstalk) handleFunc(channel string) chan bool {

	q.RLock()
	defer q.RUnlock()

	ch := make(chan bool, 1)
	f := func() {
		ch <- true
	}
	g, ok := q.tube[channel]
	if !ok {
		g = &tb{
			l: NewList(),
			handlerTable: handlerTable.NewTable(),
		}
		q.tube[channel] = g
	}

	if g.l.Len() > 0 {
		f()
	} else {
		g.handlerTable.HandlerFunc(f)
	}

	return ch
}

// startAndGC 开启清理数据
func (q *beanstalk) startAndGC() {
	go q.vaccuumDB()
	go q.vaccuumTube()
}

// vaccuumDB 检查
func (q *beanstalk) vaccuumDB() {
	ticker := time.Tick(q.dur)
	for {
		<-ticker
		if q.db == nil {
			continue
		}

		for key := range q.db {
			q.itemExpiredDB(key)
		}
	}
}

// AWeek 一周
const AWeek = time.Hour * 24 * 7

// vaccuumTube 检查
func (q *beanstalk) vaccuumTube() {
	ticker := time.Tick(AWeek)
	for {
		<-ticker
		if q.tube == nil {
			continue
		}

		for channel := range q.tube {
			q.itemExpiredDB(channel)
		}
	}
}

// itemExpired 判定并删除
func (q *beanstalk) itemExpiredDB(key string) bool {
	q.Lock()
	defer q.Unlock()

	j, ok := q.db[key]
	if !ok {
		return true
	}
	if j.isExpire(key) {
		delete(q.db, key)
		q.size -= int64(unsafe.Sizeof(j.data))
		return true
	}
	return false
}

// itemExpiredTube 判定并删除
func (q *beanstalk) itemExpiredTube(channel string) bool {
	q.Lock()
	defer q.Unlock()

	g, ok := q.tube[channel]
	if !ok {
		return true
	}

	if g.isExpire(channel) {
		delete(q.tube, channel)
	}

	return false
}

// isExpire 判定
func (j *job) isExpire(key string) bool {
	if j.status == DELAYED {
		return true
	}

	return false
}

// isExpire 判定
func (g *tb) isExpire(key string) bool {
	if g.l.Len() > 0 {
		return false
	}

	return time.Now().Sub(g.updateTime) > AWeek
}