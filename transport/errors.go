// Package transport 错误定义
package transport

import "errors"

// 预定义错误
var (
	// ErrNotConnected 未连接错误
	ErrNotConnected = errors.New("websocket not connected")

	// ErrConnectionClosed 连接已关闭错误
	ErrConnectionClosed = errors.New("websocket connection closed")

	// ErrConnectTimeout 连接超时错误
	ErrConnectTimeout = errors.New("websocket connect timeout")

	// ErrWriteTimeout 写入超时错误
	ErrWriteTimeout = errors.New("websocket write timeout")

	// ErrReadTimeout 读取超时错误
	ErrReadTimeout = errors.New("websocket read timeout")

	// ErrInvalidMessage 无效消息错误
	ErrInvalidMessage = errors.New("invalid websocket message")

	// ErrBufferFull 缓冲区满错误
	ErrBufferFull = errors.New("message buffer full")
)

// ConnectionError 连接错误
type ConnectionError struct {
	URL     string
	Op      string
	Err     error
	Retries int
}

func (e *ConnectionError) Error() string {
	if e.Retries > 0 {
		return e.Op + " " + e.URL + " after " + string(rune(e.Retries+'0')) + " retries: " + e.Err.Error()
	}
	return e.Op + " " + e.URL + ": " + e.Err.Error()
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// NewConnectionError 创建连接错误
func NewConnectionError(url, op string, err error, retries int) *ConnectionError {
	return &ConnectionError{
		URL:     url,
		Op:      op,
		Err:     err,
		Retries: retries,
	}
}
