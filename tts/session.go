// Package tts TTS会话管理
package tts

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
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

	// 配置状态
	configDone   bool            // 配置是否完成
	configDoneCh chan struct{}   // config_done 信号
	configDoneAt time.Time       // config_done 收到时间

	// 多轮合成支持（管道化：支持快速连续提交，服务端 FIFO 按序合成）
	ctx         context.Context
	cancel      context.CancelFunc
	streamQueue []*AudioStream   // 合成流 FIFO 队列（头部为当前正在接收音频的 stream）
	streamMu    sync.Mutex
	roundCount  int              // 合成轮次计数

	// 时间记录（TTFB 已下沉到每个 AudioStream 独立追踪）
}

// newSession 创建会话
func newSession(conn *transport.Conn, config *Config, opts *SynthesisOptions) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	return &Session{
		Provider:     config.Provider,
		conn:         conn,
		config:       config,
		opts:         opts,
		closeCh:      make(chan struct{}),
		configDoneCh: make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
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

	// 启动消息处理循环（需要先启动，才能接收 config_done）
	go s.messageLoop(ctx)

	// 等待 session.config_done
	if err := s.waitConfigDone(ctx); err != nil {
		return err
	}

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

	slog.Info("Session ready", "component", "tts", "id", s.ID, "provider", s.Provider)

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

// waitConfigDone 等待配置完成
func (s *Session) waitConfigDone(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait config_done: %w", ctx.Err())
	case <-s.configDoneCh:
		return nil
	}
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
		slog.Error("Parse message error", "component", "tts", "error", err)
		return
	}

	switch msgType {
	case protocol.MessageTypeSessionConfigDone:
		s.handleConfigDone()
	case protocol.MessageTypeAudioDelta:
		s.handleAudioDelta(data)
	case protocol.MessageTypeAudioDone:
		s.handleAudioDone()
	case protocol.MessageTypeError:
		s.handleError(data)
	default:
		slog.Warn("Unknown message type", "component", "tts", "type", msgType)
	}
}

// handleConfigDone 处理配置完成消息
func (s *Session) handleConfigDone() {
	s.mu.Lock()
	if !s.configDone {
		s.configDone = true
		s.configDoneAt = time.Now()
		close(s.configDoneCh)
	}
	s.mu.Unlock()

	slog.Info("Config done", "component", "tts", "id", s.ID)
}

// handleAudioDelta 处理音频数据块
func (s *Session) handleAudioDelta(data []byte) {
	msg, err := transport.ParseMessage(data)
	if err != nil {
		slog.Error("Parse audio.delta error", "component", "tts", "error", err)
		return
	}

	delta := msg.(*protocol.AudioDelta)

	// Base64解码
	audioData, err := base64.StdEncoding.DecodeString(delta.Audio)
	if err != nil {
		slog.Error("Decode audio error", "component", "tts", "error", err)
		return
	}

	s.seqNum++

	// 推送到队列头部的 stream（FIFO：服务端按序返回，头部即当前轮）
	s.streamMu.Lock()
	var stream *AudioStream
	if len(s.streamQueue) > 0 {
		stream = s.streamQueue[0]
	}
	s.streamMu.Unlock()

	if stream != nil {
		// 记录本轮首包接收时间（每个 stream 独立追踪）
		stream.markFirstChunk()
		stream.pushData(audioData, s.seqNum)
	}
}

// handleError 处理错误消息
func (s *Session) handleError(data []byte) {
	msg, err := transport.ParseMessage(data)
	if err != nil {
		slog.Error("Parse error message error", "component", "tts", "error", err)
		return
	}

	errMsg := msg.(*protocol.ErrorMessage)
	synthErr := fmt.Errorf("[%s] %s", errMsg.Code, errMsg.Message)

	// 推送错误到队列头部的 stream，并弹出
	s.streamMu.Lock()
	var stream *AudioStream
	if len(s.streamQueue) > 0 {
		stream = s.streamQueue[0]
		s.streamQueue = s.streamQueue[1:]
	}
	s.streamMu.Unlock()

	if stream != nil {
		stream.pushError(synthErr)
	}
}

// handleAudioDone 处理合成完成（管道化：弹出队列头部 stream）
func (s *Session) handleAudioDone() {
	s.streamMu.Lock()
	var stream *AudioStream
	if len(s.streamQueue) > 0 {
		stream = s.streamQueue[0]
		s.streamQueue = s.streamQueue[1:]
	}
	pending := len(s.streamQueue)
	s.streamMu.Unlock()

	if stream != nil {
		stream.pushDone()
	}

	slog.Info("Round completed", "component", "tts", "round", s.roundCount, "pending", pending, "id", s.ID)
}

// handleStreamError 处理流错误（连接级错误：所有排队中的 stream 都收到错误）
func (s *Session) handleStreamError(err error) {
	s.streamMu.Lock()
	queue := s.streamQueue
	s.streamQueue = nil
	s.streamMu.Unlock()

	for _, stream := range queue {
		stream.pushError(err)
	}
}

// SynthesizeStream 合成下一段文本（管道化模式）
// 可连续快速调用多次，不需要等待上一轮完成。服务端按 FIFO 顺序合成。
// 每次调用返回独立的 AudioStream，调用方通过 stream.Read 获取对应轮次的音频。
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

	// 创建新的音频流并推入队列
	stream := newAudioStream()
	s.streamMu.Lock()
	s.streamQueue = append(s.streamQueue, stream)
	s.roundCount++
	round := s.roundCount
	s.streamMu.Unlock()

	// 发送文本
	textMsg := transport.NewTextAppend(text)
	if err := s.conn.SendJSON(textMsg); err != nil {
		s.streamMu.Lock()
		// 从队列中移除刚加入的 stream
		for i, st := range s.streamQueue {
			if st == stream {
				s.streamQueue = append(s.streamQueue[:i], s.streamQueue[i+1:]...)
				break
			}
		}
		s.streamMu.Unlock()
		return nil, fmt.Errorf("send text: %w", err)
	}

	// 发送提交（记录 commit 时间到 stream 级别，管道化下每轮独立追踪）
	commitTime := time.Now()
	stream.setCommitSentAt(commitTime)

	commitMsg := transport.NewInputCommit()
	if err := s.conn.SendJSON(commitMsg); err != nil {
		s.streamMu.Lock()
		for i, st := range s.streamQueue {
			if st == stream {
				s.streamQueue = append(s.streamQueue[:i], s.streamQueue[i+1:]...)
				break
			}
		}
		s.streamMu.Unlock()
		return nil, fmt.Errorf("commit: %w", err)
	}

	slog.Info("Round started", "component", "tts", "round", round, "text_len", len(text), "id", s.ID)

	return stream, nil
}

// RoundCount 返回已完成的轮次数
func (s *Session) RoundCount() int {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	return s.roundCount
}

// IsSynthesizing 返回是否有正在进行或排队中的合成
func (s *Session) IsSynthesizing() bool {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	return len(s.streamQueue) > 0
}

// PendingRounds 返回队列中待合成的轮次数
func (s *Session) PendingRounds() int {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	return len(s.streamQueue)
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
			slog.Warn("Close timeout, forcing close", "component", "tts", "id", s.ID)
		}

		// 取消 context（通知 messageLoop 退出）
		s.cancel()
		// 关闭 session 自己的 closeCh
		close(s.closeCh)
		s.conn.Close()

		// 清理所有排队中的 stream
		s.streamMu.Lock()
		for _, stream := range s.streamQueue {
			stream.pushDone()
		}
		s.streamQueue = nil
		s.streamMu.Unlock()
		slog.Info("Session closed", "component", "tts", "id", s.ID, "rounds", s.roundCount)
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

// ConnectDuration 返回建连耗时（从 Conn 获取）
func (s *Session) ConnectDuration() time.Duration {
	return s.conn.ConnectDuration()
}

// ConnectedAt 返回建连完成时间
func (s *Session) ConnectedAt() time.Time {
	return s.conn.ConnectedAt()
}

// ConfigDoneAt 返回 config_done 收到时间
func (s *Session) ConfigDoneAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configDoneAt
}

// IsConfigDone 检查配置是否完成
func (s *Session) IsConfigDone() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configDone
}
