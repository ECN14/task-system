package main

import (
	"./socketio"
	"./cache"
	"./queue"
	"fmt"
	"net"
	"log"
	"os"
	"bytes"
	"time"
	"strconv"
	"sync"
	"runtime"
	"errors"
	"regexp"
) 
// Cache 缓存对象
var Cache = cache.NewCache()
// Queue 队列对象
var Queue = queue.NewQueue()
// DefaultPool 池
var DefaultPool = NewPool()

// ErrKeyPool 连接对象Key池过多  
var ErrKeyPool = errors.New("-205 beyond max")
// ErrChannel 请求队列通道存在问题
var ErrChannel = errors.New("-206 channe is null or format error")
// ErrKeyNull 请问Key存在问题 
var ErrKeyNull = errors.New("-207 key is null")
// ErrTimeOut 请求时间错误
var ErrTimeOut = errors.New("-208 request time out")
// ChannelPattern 队列channel验证
var ChannelPattern = `^[a-zA-Z_]+[a-zA-Z0-9_]*$`
// KeyPattern 队列Key验证
var KeyPattern = `^[0-9a-zA-Z]+$`

// Pool 池结构
type Pool struct {
	sync.RWMutex
	items map[net.Conn]map[string]bool
}

// NewPool 新建池
func NewPool() *Pool {
	return &Pool{
        items:make(map[net.Conn]map[string]bool),
    }
}
 // Put 向连接KEY池添加一个KEY
func (p *Pool) Put(conn net.Conn, key string) error {
	itm := p.GetItm(conn)
	if len(itm) >= 1000 {
		return ErrKeyPool
	}
	itm[key] = true
	return nil
}

// GetItm  获取连接池KEY对象
func (p *Pool) GetItm(conn net.Conn) map[string]bool {
	p.Lock()
	defer p.Unlock()

	itm, ok := p.items[conn]
	if !ok {
		itm = make(map[string]bool)
		p.items[conn] = make(map[string]bool)
	}

	return itm
}

// Out 移出链接池KEY
func (p *Pool) Out(conn net.Conn, key string) error {
	itm := p.GetItm(conn)
	delete(itm, key)

	return nil
}

// GetList 获取所以连接池KEY
func (p *Pool) GetList(conn net.Conn) map[string]bool {
	p.RLock()
	defer p.RUnlock()

	if itm, ok := p.items[conn]; ok {
		return itm
	}

	return nil
}

// Clear 清空一个连接KEY池
func (p *Pool) Clear(conn net.Conn) error {
	p.Lock()
	defer p.Unlock()

	delete(p.items, conn)

	return nil
}

func main() {
	socketio.HandleFunc("addTask", addTask)
	socketio.HandleFunc("getReturn", getReturn)
	socketio.HandleFunc("getTask", getTask)
	socketio.HandleFunc("setReturn", setReturn)
	socketio.HandleFunc("usr1", usr1)
	srv, err := socketio.ListenAndServe(":7788", connErr)
	if err != nil {
		log.Fatal("启动失败")
	} else {
		// 程序进入退出阶段
		socketio.OutHandleFunc("getTask")
		fmt.Println("服务进入退出")
		fmt.Println("等待队列无RESERVED状态")
		Queue.Wait()
		fmt.Println("队列无RESERVED状态")
		fmt.Println("等待所有连接关闭")
		srv.Wait()
		fmt.Println("所有连接关闭")
		fmt.Println("退出")
		os.Exit(0)
	}

}

func addTask(rep socketio.ResponseWriter, req *socketio.Request) {
	n := bytes.IndexByte(req.Data, ' ')
	if n < 1 {
		rep.Write([]byte(ErrChannel.Error()))
		return
	}

	var data []byte
	var channel = req.Data[0:n]

	ma, err := regexp.Match(ChannelPattern, channel)
	if err != nil || ma == false {
		rep.Write([]byte(ErrChannel.Error()))
		return
	}

	if len(req.Data) > n + 1 {
		data = req.Data[n + 1:]
	} else {
		data = []byte("")
	}

	key := Cache.InitNil()
	err = Queue.Put(string(channel), key, data)
	if err != nil {
		Cache.Delete(key)
		rep.Write([]byte(err.Error()))
		return
	}

	rep.Write([]byte(fmt.Sprintf("+%s", key)))
}

func getReturn(rep socketio.ResponseWriter, req *socketio.Request) {
	n := bytes.IndexByte(req.Data, ' ')
	if n < 1 {
		rep.Write([]byte(ErrKeyNull.Error()))
		return
	}
	var key = req.Data[0:n]
	var tm int
	var err error

	ma, err := regexp.Match(KeyPattern, key)
	if err != nil || ma == false {
		rep.Write([]byte(ErrKeyNull.Error()))
		return
	}

	if len(req.Data) > n + 1 {
		tm = 1000
	} else {
		tm, err = strconv.Atoi(string(req.Data[n + 1:]))
		if err != nil {
			rep.Write([]byte(ErrTimeOut.Error()))
			return
		}
	}


	data, err := Cache.GetAndTimeOut(string(key), time.Duration(tm) * time.Millisecond)
	if err != nil {
		rep.Write([]byte(err.Error()))
	} else {
		rep.Write(data)
	}

}

func getTask(rep socketio.ResponseWriter, req *socketio.Request) {
	ma, err := regexp.Match(ChannelPattern, req.Data)
	if err != nil || ma == false {
		rep.Write([]byte(ErrChannel.Error()))
		return
	}

	key, val, err := Queue.Get(string(req.Data))
	if err != nil {
		rep.Write([]byte(err.Error()))
		return
	}

	err = DefaultPool.Put(req.Conn, key)
	if err != nil {
		rep.Write([]byte(err.Error()))
		return
	}
	rep.Write([]byte(fmt.Sprintf("+%s %s", key, val)))
}

func setReturn(rep socketio.ResponseWriter, req *socketio.Request) {
	n := bytes.IndexByte(req.Data, ' ')
	if n < 1 {
		rep.Write([]byte(ErrKeyNull.Error()))
		return
	}

	var data []byte
	var key = string(req.Data[:n])

	ma, err := regexp.Match(KeyPattern, req.Data[:n])
	if err != nil || ma == false {
		rep.Write([]byte(ErrKeyNull.Error()))
		return
	}

	if len(req.Data) > n + 1 {
		data = req.Data[n + 1:]
	} else {
		data = []byte("")
	}

	DefaultPool.Out(req.Conn, key)
	err = Queue.End(key)
	if err != nil {
		rep.Write([]byte(err.Error()))
		return
	}
	Cache.Cover(key, data)
	rep.Write([]byte("+1"))

}

func usr1(rep socketio.ResponseWriter, req *socketio.Request) {
	ma, err := regexp.Match(ChannelPattern, req.Data)
	if err != nil || ma == false {
		rep.Write([]byte(ErrChannel.Error()))
		return
	}
	Queue.User1(string(req.Data))
	rep.Write([]byte("+n"))
}

func connErr(conn net.Conn, err interface{}) {
	list := DefaultPool.GetList(conn)
	for key := range list {
		Queue.Restore(key)
	}
	DefaultPool.Clear(conn)
}

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}