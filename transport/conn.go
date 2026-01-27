// Package transport 提供WebSocket连接管理
package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 全局 TLS Session Cache（所有连接共享）
// 启用 TLS Session Resumption，减少 TLS 握手从 2 RTT 到 1 RTT
var globalTLSSessionCache = tls.NewLRUClientSessionCache(100)

// Config WebSocket连接配置
type Config struct {
	URL              string        // WebSocket URL
	ConnectTimeout   time.Duration // 连接超时
	ReadTimeout      time.Duration // 读超时
	WriteTimeout     time.Duration // 写超时
	PingInterval     time.Duration // 心跳间隔
	ReconnectBackoff time.Duration // 重连退避基数
	MaxReconnects    int           // 最大重连次数
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ConnectTimeout:   10 * time.Second,
		ReadTimeout:      60 * time.Second,
		WriteTimeout:     10 * time.Second,
		PingInterval:     30 * time.Second,
		ReconnectBackoff: 1 * time.Second,
		MaxReconnects:    3,
	}
}

// Conn WebSocket连接封装
type Conn struct {
	config    *Config
	ws        *websocket.Conn
	mu        sync.Mutex
	readCh    chan []byte
	errorCh   chan error
	closeCh   chan struct{}
	closeOnce sync.Once
	connected bool

	// 时间记录
	connectStartAt time.Time // 建连开始时间（TCP+TLS+WS握手）
	connectedAt    time.Time // 建连完成时间
}

// NewConn 创建新的WebSocket连接
func NewConn(config *Config) *Conn {
	if config == nil {
		config = DefaultConfig()
	}
	return &Conn{
		config:  config,
		readCh:  make(chan []byte, 100),
		errorCh: make(chan error, 10),
		closeCh: make(chan struct{}),
	}
}

// Connect 建立WebSocket连接
func (c *Conn) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// 记录建连开始时间
	c.connectStartAt = time.Now()

	dialer := websocket.Dialer{
		HandshakeTimeout: c.config.ConnectTimeout,
		TLSClientConfig: &tls.Config{
			ClientSessionCache: globalTLSSessionCache, // 启用 TLS session 复用
		},
	}

	// 创建带超时的context
	connectCtx, cancel := context.WithTimeout(ctx, c.config.ConnectTimeout)
	defer cancel()

	ws, resp, err := dialer.DialContext(connectCtx, c.config.URL, nil)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("websocket connect failed: %w, status: %d", err, resp.StatusCode)
		}
		return fmt.Errorf("websocket connect failed: %w", err)
	}

	c.ws = ws
	c.connected = true
	c.connectedAt = time.Now() // 记录建连完成时间

	// 启动读取goroutine
	go c.readLoop()

	// 启动心跳goroutine（可选）
	if c.config.PingInterval > 0 {
		go c.pingLoop()
	}

	log.Printf("[client.transport] WebSocket connected: url=%s, connect_duration=%dms",
		c.config.URL, c.connectedAt.Sub(c.connectStartAt).Milliseconds())
	return nil
}

// ConnectWithRetry 带重试的连接
func (c *Conn) ConnectWithRetry(ctx context.Context) error {
	var lastErr error
	backoff := c.config.ReconnectBackoff

	for i := 0; i <= c.config.MaxReconnects; i++ {
		if i > 0 {
			log.Printf("[client.transport] Reconnecting attempt %d/%d, backoff=%v",
				i, c.config.MaxReconnects, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			// 指数退避
			backoff *= 2
		}

		err := c.Connect(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		log.Printf("[client.transport] Connect failed: %v", err)
	}

	return fmt.Errorf("connect failed after %d retries: %w", c.config.MaxReconnects, lastErr)
}

// readLoop 读取消息循环
func (c *Conn) readLoop() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.closeCh:
			return
		default:
		}

		// 设置读取超时
		if c.config.ReadTimeout > 0 {
			c.ws.SetReadDeadline(time.Now().Add(c.config.ReadTimeout))
		}

		_, message, err := c.ws.ReadMessage()
		if err != nil {
			select {
			case <-c.closeCh:
				return
			default:
				// 区分正常关闭和异常关闭
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("[client.transport] WebSocket closed normally")
					// 通过 closeOnce 安全关闭 closeCh，通知 session
					c.closeOnce.Do(func() {
						close(c.closeCh)
					})
				} else {
					log.Printf("[client.transport] WebSocket read error: %v", err)
					c.errorCh <- err
				}
				return
			}
		}

		select {
		case c.readCh <- message:
		case <-c.closeCh:
			return
		}
	}
}

// pingLoop 心跳循环
func (c *Conn) pingLoop() {
	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
			c.mu.Lock()
			if c.ws != nil && c.connected {
				if err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.config.WriteTimeout)); err != nil {
					log.Printf("[client.transport] Ping failed: %v", err)
				}
			}
			c.mu.Unlock()
		}
	}
}

// SendJSON 发送JSON消息
func (c *Conn) SendJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.ws == nil {
		return ErrNotConnected
	}

	// 设置写入超时
	if c.config.WriteTimeout > 0 {
		c.ws.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
	}

	return c.ws.WriteJSON(v)
}

// SendBytes 发送二进制消息
func (c *Conn) SendBytes(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.ws == nil {
		return ErrNotConnected
	}

	if c.config.WriteTimeout > 0 {
		c.ws.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
	}

	return c.ws.WriteMessage(websocket.BinaryMessage, data)
}

// SendText 发送文本消息
func (c *Conn) SendText(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.ws == nil {
		return ErrNotConnected
	}

	if c.config.WriteTimeout > 0 {
		c.ws.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout))
	}

	return c.ws.WriteMessage(websocket.TextMessage, []byte(text))
}

// Receive 阻塞接收一条消息
func (c *Conn) Receive(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeCh:
		return nil, ErrConnectionClosed
	case err := <-c.errorCh:
		return nil, err
	case msg := <-c.readCh:
		return msg, nil
	}
}

// ReceiveJSON 接收并解析JSON消息
func (c *Conn) ReceiveJSON(ctx context.Context, v interface{}) error {
	data, err := c.Receive(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// ReceiveChan 返回消息接收channel
func (c *Conn) ReceiveChan() <-chan []byte {
	return c.readCh
}

// ErrorChan 返回错误channel
func (c *Conn) ErrorChan() <-chan error {
	return c.errorCh
}

// CloseChan 返回关闭信号channel
func (c *Conn) CloseChan() <-chan struct{} {
	return c.closeCh
}

// Close 关闭连接
func (c *Conn) Close() error {
	// 关闭 closeCh 通知其他 goroutine
	// 注意：closeOnce 可能已被 readLoop 执行（正常关闭场景）
	c.closeOnce.Do(func() {
		close(c.closeCh)
	})

	// 关闭实际的 ws 连接（总是需要执行）
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ws != nil {
		// 发送关闭消息
		c.ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second))
		c.ws.Close()
		c.ws = nil
		c.connected = false
		log.Printf("[client.transport] WebSocket closed")
	}
	return nil
}

// IsConnected 检查连接状态
func (c *Conn) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// URL 返回连接URL
func (c *Conn) URL() string {
	return c.config.URL
}

// SetReadDeadline 设置读取超时
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws == nil {
		return ErrNotConnected
	}
	return c.ws.SetReadDeadline(t)
}

// SetWriteDeadline 设置写入超时
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ws == nil {
		return ErrNotConnected
	}
	return c.ws.SetWriteDeadline(t)
}

// Response 返回WebSocket握手响应（仅在连接时有效）
func (c *Conn) Response() *http.Response {
	// 注意：gorilla/websocket不保存响应
	return nil
}

// ConnectDuration 返回建连耗时（TCP+TLS+WS握手）
func (c *Conn) ConnectDuration() time.Duration {
	if c.connectedAt.IsZero() || c.connectStartAt.IsZero() {
		return 0
	}
	return c.connectedAt.Sub(c.connectStartAt)
}

// ConnectedAt 返回建连完成时间
func (c *Conn) ConnectedAt() time.Time {
	return c.connectedAt
}
