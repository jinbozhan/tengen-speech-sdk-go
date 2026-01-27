// Package client 客户端错误定义
package client

import (
	"errors"
	"fmt"
)

// 预定义错误
var (
	// ErrSessionNotReady 会话未就绪
	ErrSessionNotReady = errors.New("session not ready")

	// ErrSessionClosed 会话已关闭
	ErrSessionClosed = errors.New("session closed")

	// ErrProviderNotSupported 不支持的提供商
	ErrProviderNotSupported = errors.New("provider not supported")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrAudioFormatNotSupported 不支持的音频格式
	ErrAudioFormatNotSupported = errors.New("audio format not supported")

	// ErrFileNotFound 文件未找到
	ErrFileNotFound = errors.New("file not found")

	// ErrTimeout 操作超时
	ErrTimeout = errors.New("operation timeout")
)

// ClientError 客户端错误
type ClientError struct {
	Op       string // 操作名称
	Provider string // 提供商
	Code     string // 错误代码
	Message  string // 错误信息
	Err      error  // 底层错误
}

func (e *ClientError) Error() string {
	msg := e.Op
	if e.Provider != "" {
		msg += " [provider=" + e.Provider + "]"
	}
	if e.Code != "" {
		msg += " [code=" + e.Code + "]"
	}
	msg += ": " + e.Message
	if e.Err != nil {
		msg += " (" + e.Err.Error() + ")"
	}
	return msg
}

func (e *ClientError) Unwrap() error {
	return e.Err
}

// NewClientError 创建客户端错误
func NewClientError(op, provider, code, message string, err error) *ClientError {
	return &ClientError{
		Op:       op,
		Provider: provider,
		Code:     code,
		Message:  message,
		Err:      err,
	}
}

// NewConnectionError 创建连接错误
func NewConnectionError(op, message string, err error) *ClientError {
	return &ClientError{
		Op:      op,
		Code:    "CONNECTION_ERROR",
		Message: message,
		Err:     err,
	}
}

// NewConfigError 创建配置错误
func NewConfigError(op, message string) *ClientError {
	return &ClientError{
		Op:      op,
		Code:    "CONFIG_ERROR",
		Message: message,
	}
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(op, message string) *ClientError {
	return &ClientError{
		Op:      op,
		Code:    "TIMEOUT",
		Message: message,
	}
}

// NewProtocolError 创建协议错误
func NewProtocolError(op, message string, err error) *ClientError {
	return &ClientError{
		Op:      op,
		Code:    "PROTOCOL_ERROR",
		Message: message,
		Err:     err,
	}
}

// NewProviderError 创建提供商错误
func NewProviderError(op, provider, code, message string) *ClientError {
	return &ClientError{
		Op:       op,
		Provider: provider,
		Code:     code,
		Message:  message,
	}
}

// IsConnectionError 判断是否为连接错误
func IsConnectionError(err error) bool {
	var ce *ClientError
	if errors.As(err, &ce) {
		return ce.Code == "CONNECTION_ERROR"
	}
	return false
}

// IsTimeoutError 判断是否为超时错误
func IsTimeoutError(err error) bool {
	var ce *ClientError
	if errors.As(err, &ce) {
		return ce.Code == "TIMEOUT"
	}
	return errors.Is(err, ErrTimeout)
}

// IsRetryable 判断错误是否可重试
func IsRetryable(err error) bool {
	var ce *ClientError
	if errors.As(err, &ce) {
		switch ce.Code {
		case "CONNECTION_ERROR", "TIMEOUT":
			return true
		}
	}
	return false
}

// WrapError 包装错误
func WrapError(op string, err error) error {
	if err == nil {
		return nil
	}
	var ce *ClientError
	if errors.As(err, &ce) {
		// 已经是ClientError，添加操作上下文
		return fmt.Errorf("%s: %w", op, err)
	}
	return &ClientError{
		Op:      op,
		Message: err.Error(),
		Err:     err,
	}
}
