// Package stt STT会话管理
package stt

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/protocol"
	"github.com/jinbozhan/tengen-speech-sdk-go/transport"
)

// Session STT会话
type Session struct {
	ID        string         // 会话ID
	Provider  string         // 提供商
	conn      *transport.Conn
	config    *Config
	opts      *StreamOptions
	eventsCh  chan *RecognitionEvent
	closeCh   chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	ready     bool
	closed    bool
}

// newSession 创建会话
func newSession(conn *transport.Conn, config *Config, opts *StreamOptions) *Session {
	return &Session{
		Provider: config.Provider,
		conn:     conn,
		config:   config,
		opts:     opts,
		eventsCh: make(chan *RecognitionEvent, 100),
		closeCh:  make(chan struct{}),
	}
}

// start 启动会话
func (s *Session) start(ctx context.Context) error {
	// 等待session.ready消息
	if err := s.waitReady(ctx); err != nil {
		return err
	}

	// 发送session.config
	if err := s.sendConfig(); err != nil {
		return err
	}

	// 启动消息处理循环
	go s.messageLoop(ctx)

	return nil
}

// waitReady 等待会话就绪
func (s *Session) waitReady(ctx context.Context) error {
	// 设置超时
	timeout := s.config.ConnectTimeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 接收第一条消息，应该是session.ready
	data, err := s.conn.Receive(ctx)
	if err != nil {
		return fmt.Errorf("wait session.ready: %w", err)
	}

	// 解析消息类型
	msgType, err := transport.ParseMessageType(data)
	if err != nil {
		return fmt.Errorf("parse session.ready: %w", err)
	}

	if msgType != protocol.MessageTypeSessionReady {
		return fmt.Errorf("expected session.ready, got %s", msgType)
	}

	// 解析会话ID
	msg, err := transport.ParseMessage(data)
	if err != nil {
		return fmt.Errorf("parse session.ready body: %w", err)
	}

	ready := msg.(*protocol.SessionReady)
	s.ID = ready.SessionID
	s.ready = true

	log.Printf("[client.stt] Session ready: id=%s, provider=%s", s.ID, s.Provider)

	// 发送就绪事件
	s.sendEvent(NewReadyEvent(s.ID))

	return nil
}

// sendConfig 发送会话配置
func (s *Session) sendConfig() error {
	params := protocol.SessionParams{
		Provider:    s.Provider,
		Language:    s.opts.Language,
		SampleRate:  s.opts.SampleRate,
		AudioFormat: s.opts.AudioFormat,
	}

	msg := transport.NewSessionConfig(params)
	return s.conn.SendJSON(msg)
}

// messageLoop 消息处理循环
func (s *Session) messageLoop(ctx context.Context) {
	defer func() {
		s.sendEvent(NewClosedEvent())
		close(s.eventsCh)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.closeCh:
			return
		case err := <-s.conn.ErrorChan():
			s.sendEvent(NewErrorEvent(err))
			return
		case data := <-s.conn.ReceiveChan():
			s.handleMessage(data)
		}
	}
}

// handleMessage 处理消息
func (s *Session) handleMessage(data []byte) {
	msgType, err := transport.ParseMessageType(data)
	if err != nil {
		log.Printf("[client.stt] Parse message error: %v", err)
		return
	}

	switch msgType {
	case protocol.MessageTypeTranscriptPartial:
		s.handlePartial(data)
	case protocol.MessageTypeTranscriptFinal:
		s.handleFinal(data)
	case protocol.MessageTypeInputDone:
		s.sendEvent(NewInputDoneEvent())
	case protocol.MessageTypeProcessing:
		s.sendEvent(NewProcessingEvent())
	case protocol.MessageTypeError:
		s.handleError(data)
	default:
		log.Printf("[client.stt] Unknown message type: %s", msgType)
	}
}

// handlePartial 处理部分识别结果
func (s *Session) handlePartial(data []byte) {
	msg, err := transport.ParseMessage(data)
	if err != nil {
		log.Printf("[client.stt] Parse partial error: %v", err)
		return
	}

	partial := msg.(*protocol.TranscriptPartial)
	event := NewPartialEvent(partial.Text)
	s.sendEvent(event)
}

// handleFinal 处理最终识别结果
func (s *Session) handleFinal(data []byte) {
	msg, err := transport.ParseMessage(data)
	if err != nil {
		log.Printf("[client.stt] Parse final error: %v", err)
		return
	}

	final := msg.(*protocol.TranscriptFinal)
	// Gateway 发送的时间戳单位为毫秒
	startTime := time.Duration(final.StartTime) * time.Millisecond
	endTime := time.Duration(final.EndTime) * time.Millisecond

	event := NewFinalEvent(final.Text, startTime, endTime)
	s.sendEvent(event)
}

// handleError 处理错误消息
func (s *Session) handleError(data []byte) {
	msg, err := transport.ParseMessage(data)
	if err != nil {
		log.Printf("[client.stt] Parse error message error: %v", err)
		return
	}

	errMsg := msg.(*protocol.ErrorMessage)
	event := NewErrorEvent(fmt.Errorf("[%s] %s", errMsg.Code, errMsg.Message))
	s.sendEvent(event)
}

// sendEvent 发送事件到channel
func (s *Session) sendEvent(event *RecognitionEvent) {
	select {
	case s.eventsCh <- event:
	case <-s.closeCh:
	default:
		// 缓冲区满，丢弃事件
		log.Printf("[client.stt] Event buffer full, dropping event: %s", event.Type)
	}
}

// Send 发送音频数据
func (s *Session) Send(audio []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session closed")
	}
	if !s.ready {
		return fmt.Errorf("session not ready")
	}

	// Base64编码
	encoded := base64.StdEncoding.EncodeToString(audio)
	msg := transport.NewAudioAppend(encoded)
	return s.conn.SendJSON(msg)
}

// SendPCM 发送PCM音频数据（16位有符号整数，小端序）
func (s *Session) SendPCM(pcm []byte) error {
	return s.Send(pcm)
}

// Commit 提交当前输入
func (s *Session) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session closed")
	}

	msg := transport.NewInputCommit()
	return s.conn.SendJSON(msg)
}

// Events 返回事件channel
func (s *Session) Events() <-chan *RecognitionEvent {
	return s.eventsCh
}

// Close 关闭会话
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()

		// 发送 session.end 消息
		msg := transport.NewSessionEnd()
		s.conn.SendJSON(msg)

		// 等待 Gateway 的 Close Frame（最多 2 秒）
		// Gateway 会在处理完 session.end 后主动发送 Close Frame
		// 这是正确的 WebSocket 关闭握手流程（RFC 6455）
		select {
		case <-s.conn.CloseChan():
			// Gateway 已正常关闭连接
		case <-time.After(2 * time.Second):
			// 超时，强制关闭
			log.Printf("[client.stt] Close timeout, forcing close: id=%s", s.ID)
		}

		// 关闭 session 自己的 closeCh（通知 messageLoop 退出）
		close(s.closeCh)
		s.conn.Close()

		log.Printf("[client.stt] Session closed: id=%s", s.ID)
	})
	return nil
}

// IsReady 检查会话是否就绪
func (s *Session) IsReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready && !s.closed
}

// IsClosed 检查会话是否已关闭
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
