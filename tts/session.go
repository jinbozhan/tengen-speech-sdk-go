// Package tts TTS会话管理
package tts

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

// Session TTS会话（支持多轮合成）
type Session struct {
	ID        string // 会话ID
	Provider  string // 提供商
	conn      *transport.Conn
	config    *Config
	opts      *SynthesisOptions
	closeCh   chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	ready     bool
	closed    bool
	seqNum    int

	// 多轮合成支持
	ctx           context.Context
	cancel        context.CancelFunc
	currentStream *AudioStream // 当前轮的流
	streamMu      sync.Mutex
	roundCount    int  // 合成轮次计数
	synthesizing  bool // 是否正在合成

	// 时间记录
	commitSentAt         time.Time // input.commit 发送时间
	firstChunkReceivedAt time.Time // 首个 audio.delta 收到时间
}

// newSession 创建会话
func newSession(conn *transport.Conn, config *Config, opts *SynthesisOptions) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		Provider: config.Provider,
		conn:     conn,
		config:   config,
		opts:     opts,
		closeCh:  make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// start 启动会话
func (s *Session) start(ctx context.Context) error {
	// 等待session.ready
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
	data, err := s.conn.Receive(ctx)
	if err != nil {
		return fmt.Errorf("wait session.ready: %w", err)
	}

	msgType, err := transport.ParseMessageType(data)
	if err != nil {
		return fmt.Errorf("parse session.ready: %w", err)
	}

	if msgType != protocol.MessageTypeSessionReady {
		return fmt.Errorf("expected session.ready, got %s", msgType)
	}

	msg, err := transport.ParseMessage(data)
	if err != nil {
		return fmt.Errorf("parse session.ready body: %w", err)
	}

	ready := msg.(*protocol.SessionReady)
	s.ID = ready.SessionID
	s.ready = true

	log.Printf("[client.tts] Session ready: id=%s, provider=%s", s.ID, s.Provider)

	return nil
}

// sendConfig 发送会话配置
func (s *Session) sendConfig() error {
	params := protocol.SessionParams{
		Provider:    s.Provider,
		VoiceID:     s.opts.VoiceID,
		Language:    s.opts.Language,
		Speed:       s.opts.Speed,
		Pitch:       s.opts.Pitch,
		Volume:      s.opts.Volume,
		SampleRate:  s.opts.SampleRate,
		AudioFormat: s.opts.AudioFormat,
	}

	msg := transport.NewSessionConfig(params)
	return s.conn.SendJSON(msg)
}

// messageLoop 消息处理循环
func (s *Session) messageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.ctx.Done():
			return
		case <-s.closeCh:
			return
		case err := <-s.conn.ErrorChan():
			s.handleStreamError(err)
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
		log.Printf("[client.tts] Parse message error: %v", err)
		return
	}

	switch msgType {
	case protocol.MessageTypeAudioDelta:
		s.handleAudioDelta(data)
	case protocol.MessageTypeAudioDone:
		s.handleAudioDone()
	case protocol.MessageTypeError:
		s.handleError(data)
	default:
		log.Printf("[client.tts] Unknown message type: %s", msgType)
	}
}

// handleAudioDelta 处理音频数据块
func (s *Session) handleAudioDelta(data []byte) {
	// 记录首包接收时间（比应用层 stream.Read 更精确）
	s.mu.Lock()
	if s.firstChunkReceivedAt.IsZero() {
		s.firstChunkReceivedAt = time.Now()
	}
	s.mu.Unlock()

	msg, err := transport.ParseMessage(data)
	if err != nil {
		log.Printf("[client.tts] Parse audio.delta error: %v", err)
		return
	}

	delta := msg.(*protocol.AudioDelta)

	// Base64解码
	audioData, err := base64.StdEncoding.DecodeString(delta.Audio)
	if err != nil {
		log.Printf("[client.tts] Decode audio error: %v", err)
		return
	}

	s.seqNum++

	// 推送到 currentStream
	s.streamMu.Lock()
	stream := s.currentStream
	s.streamMu.Unlock()

	if stream != nil {
		stream.pushData(audioData, s.seqNum)
	}
}

// handleError 处理错误消息
func (s *Session) handleError(data []byte) {
	msg, err := transport.ParseMessage(data)
	if err != nil {
		log.Printf("[client.tts] Parse error message error: %v", err)
		return
	}

	errMsg := msg.(*protocol.ErrorMessage)
	synthErr := fmt.Errorf("[%s] %s", errMsg.Code, errMsg.Message)

	// 推送错误到 currentStream
	s.streamMu.Lock()
	stream := s.currentStream
	s.currentStream = nil
	s.synthesizing = false
	s.streamMu.Unlock()

	if stream != nil {
		stream.pushError(synthErr)
	}
}

// handleAudioDone 处理合成完成（多轮模式）
func (s *Session) handleAudioDone() {
	s.streamMu.Lock()
	stream := s.currentStream
	s.currentStream = nil
	s.synthesizing = false
	s.streamMu.Unlock()

	if stream != nil {
		stream.pushDone()
	}

	log.Printf("[client.tts] Round %d completed: id=%s", s.roundCount, s.ID)
}

// handleStreamError 处理流错误
func (s *Session) handleStreamError(err error) {
	// 推送错误到 currentStream
	s.streamMu.Lock()
	stream := s.currentStream
	s.currentStream = nil
	s.synthesizing = false
	s.streamMu.Unlock()

	if stream != nil {
		stream.pushError(err)
	}
}

// SynthesizeStream 合成下一段文本（多轮模式）
// 返回音频流，可重复调用多次
func (s *Session) SynthesizeStream(ctx context.Context, text string) (*AudioStream, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, fmt.Errorf("session closed")
	}
	if !s.ready {
		s.mu.Unlock()
		return nil, fmt.Errorf("session not ready")
	}
	s.mu.Unlock()

	s.streamMu.Lock()
	if s.synthesizing {
		s.streamMu.Unlock()
		return nil, fmt.Errorf("synthesis in progress, wait for current round to complete")
	}

	// 重置每轮的时间记录
	s.mu.Lock()
	s.firstChunkReceivedAt = time.Time{}
	s.mu.Unlock()

	// 创建新的音频流
	stream := newAudioStream()
	s.currentStream = stream
	s.synthesizing = true
	s.roundCount++
	round := s.roundCount
	s.streamMu.Unlock()

	// 发送文本
	textMsg := transport.NewTextAppend(text)
	if err := s.conn.SendJSON(textMsg); err != nil {
		s.streamMu.Lock()
		s.currentStream = nil
		s.synthesizing = false
		s.streamMu.Unlock()
		return nil, fmt.Errorf("send text: %w", err)
	}

	// 发送提交
	s.mu.Lock()
	s.commitSentAt = time.Now()
	s.mu.Unlock()

	commitMsg := transport.NewInputCommit()
	if err := s.conn.SendJSON(commitMsg); err != nil {
		s.streamMu.Lock()
		s.currentStream = nil
		s.synthesizing = false
		s.streamMu.Unlock()
		return nil, fmt.Errorf("commit: %w", err)
	}

	log.Printf("[client.tts] Round %d started: text_len=%d, id=%s", round, len(text), s.ID)

	return stream, nil
}

// RoundCount 返回已完成的轮次数
func (s *Session) RoundCount() int {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	return s.roundCount
}

// IsSynthesizing 返回是否正在合成中
func (s *Session) IsSynthesizing() bool {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	return s.synthesizing
}

// SendText 发送要合成的文本
func (s *Session) SendText(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session closed")
	}
	if !s.ready {
		return fmt.Errorf("session not ready")
	}

	msg := transport.NewTextAppend(text)
	return s.conn.SendJSON(msg)
}

// Commit 提交文本，触发合成
func (s *Session) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session closed")
	}

	msg := transport.NewInputCommit()
	s.commitSentAt = time.Now() // 记录 commit 发送时间
	return s.conn.SendJSON(msg)
}

// Close 关闭会话
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()

		// 发送 session.end
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
			log.Printf("[client.tts] Close timeout, forcing close: id=%s", s.ID)
		}

		// 取消 context（通知 messageLoop 退出）
		s.cancel()
		// 关闭 session 自己的 closeCh
		close(s.closeCh)
		s.conn.Close()

		// 清理 currentStream
		s.streamMu.Lock()
		if s.currentStream != nil {
			s.currentStream.pushDone()
			s.currentStream = nil
		}
		s.streamMu.Unlock()
		log.Printf("[client.tts] Session closed: id=%s, rounds=%d", s.ID, s.roundCount)
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

// CommitSentAt 返回 input.commit 发送时间
func (s *Session) CommitSentAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitSentAt
}

// FirstChunkReceivedAt 返回首个 audio.delta 收到时间
func (s *Session) FirstChunkReceivedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.firstChunkReceivedAt
}

// TTFB 返回从 commit 发送到首包收到的时间（毫秒）
// 这是真正的 Time To First Byte，不包含建连和配置时间
func (s *Session) TTFB() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.commitSentAt.IsZero() || s.firstChunkReceivedAt.IsZero() {
		return 0
	}
	return s.firstChunkReceivedAt.Sub(s.commitSentAt).Milliseconds()
}

// ConnectDuration 返回建连耗时（从 Conn 获取）
func (s *Session) ConnectDuration() time.Duration {
	return s.conn.ConnectDuration()
}

// ConnectedAt 返回建连完成时间
func (s *Session) ConnectedAt() time.Time {
	return s.conn.ConnectedAt()
}
