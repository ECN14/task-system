package socketio

import (
	"net"
	"sync"
	"time"
	"bytes"
	"bufio"
	"regexp"
	"encoding/binary"
	"log"
	"io"
	"os"
	"fmt"
	"os/signal"
)

// Handler 是业务动作接口
type Handler interface {
	ServeTCP(ResponseWriter, *Request)
} 

const (
	startHead byte = '/'
	cmdCrlf byte = ' '
)
// ResponseWriter 数据返回接口
type ResponseWriter interface {
	Write(b[]byte)
}
// Request 一根据协议分割的一次请求
type Request struct {
	Conn    net.Conn
	Data    []byte
	Command string
}

// ServeMux 存储根据不同的命令，调用不同的函数
type ServeMux struct {
	sync.RWMutex
	m map[string]muxEntry
}

// muxEntry 命令调用函数
type muxEntry struct {
	h   Handler
	cmd string
}

// NewServeMux 新建一个存储命令函数
func NewServeMux() *ServeMux {
	return &ServeMux{
		m: make(map[string]muxEntry),
	}
}

// DefaultServeMux 默认配置
var DefaultServeMux = NewServeMux()

// NotFound 没有找到该命令
func NotFound(w ResponseWriter, r *Request) {
	w.Write([]byte("-205 command not found"))
}
// Out 服务进入退出阶段
func Out(w ResponseWriter, r *Request) {
	w.Write([]byte("-301 service out"))
}

// NotFoundHandler 返回一个简单的请求处理程序
func NotFoundHandler() Handler {
	return HandlerFunc(NotFound)
}

// HandleFunc 添加命令函数
func HandleFunc(name string, handler func(ResponseWriter, *Request)) {
	DefaultServeMux.HandleFunc(name, handler)
}

// OutHandleFunc 退出程序注册事件
func OutHandleFunc(name string) {
	DefaultServeMux.HandleFunc(fmt.Sprintf("_%s", name), Out)
}

// HandleFunc 注册命令函数
func (mux *ServeMux) HandleFunc(name string, handler func(ResponseWriter, *Request)) {
	mux.Handle(name, HandlerFunc(handler))
}

// HandlerFunc 业务动作
type HandlerFunc func(ResponseWriter, *Request)

// ServeTCP 调用 f(w, r).
func (f HandlerFunc) ServeTCP(w ResponseWriter, r *Request) {
	f(w, r)
}
// Handle 注册命令函数
func (mux *ServeMux) Handle(name string, handler Handler) {
	mux.Lock()
	defer mux.Unlock()

	ma, err := regexp.Match(`^[a-zA-Z_]+[a-zA-Z0-9_]*$`, []byte(name))
	if err != nil {
		panic(err.Error())
	} else if ma == false {
		panic("命令格式错误")
	}

	mux.m[name] = muxEntry{cmd:name, h:handler}

}

// Handler 取出命令函数
func (mux *ServeMux) Handler(name string) (Handler, string) {
	mux.RLock()
	defer mux.RUnlock()


	entry, ok := mux.m[fmt.Sprintf("_%s", name)]
	if ok {
		return entry.h, name
	}

	entry, ok = mux.m[name]
	if ok {
		return entry.h, name
	}
	return NotFoundHandler(), name
}

// ServeTCP 业务动作
func (mux *ServeMux) ServeTCP(w ResponseWriter, r *Request) {
	h, _ := mux.Handler(r.Command)
	h.ServeTCP(w, r)
}

// Server 服务
type Server struct {
	Addr     string
	sync.WaitGroup
	efunc    func(conn net.Conn, err interface{})
	ErrorLog *log.Logger
}

// ListenAndServe 监听端口和建立连接
func ListenAndServe(addr string, efunc func(conn net.Conn, err interface{})) (srv *Server, e error) {
	srv = &Server{Addr:addr, efunc: efunc}
	e = srv.ListenAndServe()
	return srv, e
}

// ListenAndServe 监听端口和建立连接
func (srv *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	return srv.Serve(ln)
}

// Error 链接异常处理函数
func (srv *Server) Error(conn net.Conn, err interface{}) {
	if srv.efunc != nil {
		srv.efunc(conn, err)
	}

}

var testHookServerServe func(*Server, net.Listener) // used if non-nil

// Serve 建立连接
func (srv *Server) Serve(l net.Listener) error {
	defer l.Close()
	if fn := testHookServerServe; fn != nil {
		fn(srv, l)
	}
	var tempDelay time.Duration
	var c = make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	for {
		select {
		case <-c:
			return nil
		default:
			rw, e := l.Accept()
			if e != nil {
				if ne, ok := e.(net.Error); ok && ne.Temporary() {
					if tempDelay == 0 {
						tempDelay = 5 * time.Millisecond
					} else {
						tempDelay *= 2
					}
					if max := 1 * time.Second; tempDelay > max {
						tempDelay = max
					}
					srv.logf("http: Accept error: %v; retrying in %v", e, tempDelay)
					time.Sleep(tempDelay)
					continue
				}
				return e
			}
			tempDelay = 0
			c := srv.newConn(rw)
			srv.Add(1)
			go c.serve()
		}

	}
}

// logf 记录错误日志
func (srv *Server) logf(format string, args ...interface{}) {
	if srv.ErrorLog != nil {
		srv.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

type conn struct {
	server     *Server
	remoteAddr string
	rwc        net.Conn
	reader     *bufio.Reader
	buf        *bytes.Buffer
}

// newConn 建立了一个新连接
func (srv *Server) newConn(rwc net.Conn) *conn {
	return &conn{server:srv,
		rwc: rwc,
		reader: bufio.NewReader(rwc),
		buf: bytes.NewBuffer(nil),
	}
}

// serve 一个新连接
func (c *conn) serve() {
	c.remoteAddr = c.rwc.RemoteAddr().String()
	defer func() {
		c.close()
		c.server.Done()
		if err := recover(); err != nil {
			switch e := err.(type) {
			case error:
				if e != io.EOF {
					c.server.logf("tcp: panic serving %v: %v\n", c.remoteAddr, err)
				}
			default:
				c.server.logf("tcp: panic serving %v: %v\n", c.remoteAddr, err)
			}

			c.server.Error(c.rwc, err)
		}
	}()
	for {
		c.header()
		var _l int32
		err := binary.Read(c.reader, binary.BigEndian, &_l)
		if err != nil {
			panic(err)
		}
		data := c.readDataSize(_l)
		req := c.newRequest(data)

		DefaultServeMux.ServeTCP(ResponseWriter(c), req)
	}
}

// newRequset 创建一个读取数据
func (c *conn) newRequest(b []byte) *Request {
	req := &Request{Conn: c.rwc}
	n := bytes.IndexByte(b, cmdCrlf)
	if n != -1 {
		req.Command = string(b[:n])
		req.Data = b[n + 1:]
	}

	return req
}

// read 读取数据
func (c *conn) read(b []byte) (n int, err error) {
	n, err = c.reader.Read(b)
	return
}

// Read 读取完整数据
func (c *conn) Read(b []byte) (err error) {
	var n, rn, l int = 0, 0, len(b)

	for l > rn {
		n, err = c.read(b[rn:])
		if err != nil {
			return
		}

		rn += n
	}

	return
}

// Write 写入数据
func (c *conn) Write(b []byte) {
	e := c.write(b)
	defer c.buf.Reset()
	if e != nil {
		panic(e)
	}
}
func (c *conn) write(b []byte) (err error) {
	var nn, wn, l int64 = 0, 0, int64(len(b) + 4)
	err = c.buf.WriteByte(startHead)
	if err != nil {
		return
	}
	err = binary.Write(c.buf, binary.BigEndian, int32(l))
	if err != nil {
		return
	}

	c.buf.Write(b)

	for l > wn {
		nn, err = c.buf.WriteTo(c.rwc)
		if err != nil {
			return err
		}
		wn += nn
	}

	return nil
}

// header 寻找数据头位置
func (c *conn) header() {
	for {
		_, err := c.reader.ReadSlice(startHead)
		if err != nil {
			panic(err)
		}

		return
	}
}


// 关闭当前连接
func (c *conn) close() {
	c.rwc.Close()
}

// readDataSize 读取请求数据大小
func (c *conn) readDataSize(size int32) []byte {
	if size < 1 {
		return nil
	}
	data := make([]byte, size)
	err := c.Read(data)
	if err != nil {
		panic(err)
	}

	return data
} 
