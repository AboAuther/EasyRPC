package EasyRPC

import (
	"EasyRPC/codec"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type Call struct {
	Seq           uint64
	ServiceMethod string      // format "<service>.<method>"
	Args          interface{} // RPC调用时参数
	Reply         interface{} // RPC调用返回的参数
	Error         error
	Done          chan *Call // Strobes when call is complete.
}

type clientResult struct {
	client *Client
	err    error
}

func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc       codec.Codec
	opt      *Option
	sending  sync.Mutex // 保证消息有序发送，防止报文混淆
	header   codec.Header
	mu       sync.Mutex
	seq      uint64           // 给发送的请求编号，每个请求拥有唯一编号
	pending  map[uint64]*Call // pending 存储未处理完的请求，键是编号，值是 Call 实例
	closing  bool             // 置true代表用户已主动关闭
	shutdown bool             // 置true代表服务端告知关闭
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

type newClientFunc func(conn net.Conn, opt *Option) (client *Client, err error)

// Close 关闭连接
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrShutdown
	}
	client.closing = true
	return client.cc.Close()
}

// IsAvailable 客户端正常工作时返回true
func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.shutdown && !client.closing
}

//registerCall 将参数 call 添加到 client.pending 中，并更新 client.seq
func (client *Client) registerCall(call *Call) (uint64, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing || client.shutdown {
		return 0, ErrShutdown
	}
	call.Seq = client.seq
	client.pending[call.Seq] = call
	client.seq++
	return call.Seq, nil
}

//removeCall 根据 seq，从 client.pending 中移除对应的 call，并返回。
func (client *Client) removeCall(seq uint64) *Call {
	client.mu.Lock()
	defer client.mu.Unlock()
	call := client.pending[seq]
	delete(client.pending, seq)
	return call
}

//terminateCalls 服务端或客户端发生错误时调用，将 shutdown 设置为 true，且将错误信息通知所有 pending 状态的 call
func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	for _, call := range client.pending {
		call.Error = err
		call.done()
	}
}

//receive
func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			// 写入部分失败，call被删
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadBody(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body " + err.Error())
			}
			call.done()
		}
	}
	// 发生错误，因此 terminateCalls 挂起的调用
	client.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error: ", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func newClientCodec(cc codec.Codec, opt *Option) *Client {
	client := &Client{
		seq:     1, // seq 以 1 开头，0 表示调用无效
		cc:      cc,
		opt:     opt,
		pending: make(map[uint64]*Call),
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*Option) (*Option, error) {
	// 如果 opts 为 nil 或将 nil 作为参数传递
	if len(opts) == 0 || opts[0] == nil {
		return DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("number of options is more than 1")
	}
	opt := opts[0]
	opt.MagicNumber = DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = DefaultOption.CodecType
	}
	return opt, nil
}

func dialTimeout(f newClientFunc, network, address string, opts ...*Option) (client *Client, err error) {
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, opt.ConnectTimeOut)
	if err != nil {
		return nil, err
	}
	//如果Client 为空则关闭连接
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	ch := make(chan clientResult)
	go func() {
		client, err := f(conn, opt)
		ch <- clientResult{
			client: client,
			err:    err,
		}
	}()
	if opt.ConnectTimeOut == 0 {
		result := <-ch
		return result.client, result.err
	}
	select {
	case <-time.After(opt.ConnectTimeOut):
		return nil, fmt.Errorf("rpc client: connect timeout:except within %s", opt.ConnectTimeOut)
	case result := <-ch:
		return result.client, result.err
	}
}

// Dial 连接到指定网络地址的 RPC 服务器
func Dial(network, address string, opts ...*Option) (client *Client, err error) {
	return dialTimeout(NewClient, network, address, opts...)
}

func (client *Client) send(call *Call) {
	// 保证客户端将发送完整的请求
	client.sending.Lock()
	defer client.sending.Unlock()

	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	// 准备请求头
	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	// 加密，发送请求
	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call := client.removeCall(seq)
		// call可能为空，通常为写入失败
		// 客户端收到相应并处理
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

// Go 异步调用函数，返回表示调用的 Call 结构。
func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 10)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	client.send(call)
	return call
}

// Call 调用命名函数，等待它完成，并返回其错误状态
func (client *Client) Call(ctx context.Context, serviceMethod string, args, reply interface{}) error {
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return errors.New("rpc client:call failed: " + ctx.Err().Error())
	case call := <-call.Done:
		return call.Error
	}
}
