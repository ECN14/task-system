package handlerTable

// Handler 注册函数
type Handler interface {
	CallFunc()
}
// HandlerFunc 注册函数类型
type HandlerFunc func()

// CallFunc 实现注册函数
func (f HandlerFunc) CallFunc() {
	f()
}

// Table 函数注册表
type Table interface {
	HandlerFunc(f func()) error
	CallBack()
}

// 函数注册表结构体
type handerTable struct {
	Pool
}

// NewTable 新建一个函数注册表
func NewTable() Table {
	ht := &handerTable{Pool:NewPool(), }
	return ht
}

// HandlerFunc 注册一个函数
func (ht *handerTable) HandlerFunc(f func()) error {
	ht.Put(HandlerFunc(f))

	return nil
}

// put 注册函数
func (ht *handerTable) put(h HandlerFunc) {
	ht.Put(Handler(h))
}

// 取出所有函数并回调
func (ht *handerTable) CallBack() {
	var h Handler
	for {
		h = ht.Get()
		if h == nil {
			return
		}
		h.CallFunc()
	}
}


