// Package tts 音频流处理
package tts

import (
	"bytes"
	"io"
	"os"
	"sync"
	"time"
)

// AudioStream 音频流
type AudioStream struct {
	chunksCh       chan AudioChunk
	buffer         *bytes.Buffer
	mu             sync.Mutex
	closed         bool
	closeCh        chan struct{}
	closeOnce      sync.Once
	chunkChClosed  bool
	chunkChMu      sync.Mutex
	totalSize      int64
	err            error
	sessionCloser  io.Closer  // 用于关闭底层 session
	session        *Session   // 用于访问 session 时间信息
}

// AudioChunk 音频数据块
type AudioChunk struct {
	Data      []byte // 音频数据
	Sequence  int    // 序列号
	IsDone    bool   // 是否完成
	Error     error  // 错误
}

// newAudioStream 创建音频流
func newAudioStream() *AudioStream {
	return &AudioStream{
		chunksCh: make(chan AudioChunk, 100),
		buffer:   new(bytes.Buffer),
		closeCh:  make(chan struct{}),
	}
}

// Read 实现io.Reader接口
func (s *AudioStream) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 先从缓冲区读取
	if s.buffer.Len() > 0 {
		return s.buffer.Read(p)
	}

	// 检查是否已关闭
	if s.closed {
		return 0, io.EOF
	}

	// 等待新的数据块
	s.mu.Unlock()
	chunk, ok := <-s.chunksCh
	s.mu.Lock()

	if !ok || chunk.IsDone {
		s.closed = true
		return 0, io.EOF
	}

	if chunk.Error != nil {
		return 0, chunk.Error
	}

	// 写入缓冲区并读取
	s.buffer.Write(chunk.Data)
	s.totalSize += int64(len(chunk.Data))
	return s.buffer.Read(p)
}

// Chunks 返回音频块channel（逐块接收）
func (s *AudioStream) Chunks() <-chan AudioChunk {
	return s.chunksCh
}

// pushChunk 推送音频块（内部使用）
func (s *AudioStream) pushChunk(chunk AudioChunk) bool {
	s.chunkChMu.Lock()
	if s.chunkChClosed {
		s.chunkChMu.Unlock()
		return false
	}
	s.chunkChMu.Unlock()

	select {
	case s.chunksCh <- chunk:
		return true
	case <-s.closeCh:
		return false
	}
}

// pushData 推送音频数据（内部使用）
func (s *AudioStream) pushData(data []byte, seq int) {
	s.pushChunk(AudioChunk{
		Data:     data,
		Sequence: seq,
	})
}

// pushDone 推送完成信号（内部使用）
func (s *AudioStream) pushDone() {
	s.closeOnce.Do(func() {
		// 先标记channel为已关闭，再发送最后的消息
		s.chunkChMu.Lock()
		if !s.chunkChClosed {
			// 尝试发送完成信号（非阻塞）
			select {
			case s.chunksCh <- AudioChunk{IsDone: true}:
			default:
			}
			s.chunkChClosed = true
			close(s.chunksCh)
		}
		s.chunkChMu.Unlock()
		close(s.closeCh)
	})
}

// pushError 推送错误（内部使用）
func (s *AudioStream) pushError(err error) {
	s.err = err
	s.closeOnce.Do(func() {
		s.chunkChMu.Lock()
		if !s.chunkChClosed {
			// 尝试发送错误信号（非阻塞）
			select {
			case s.chunksCh <- AudioChunk{Error: err}:
			default:
			}
			s.chunkChClosed = true
			close(s.chunksCh)
		}
		s.chunkChMu.Unlock()
		close(s.closeCh)
	})
}

// SaveToFile 保存到文件
func (s *AudioStream) SaveToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, s)
	return err
}

// ReadAll 读取所有数据
func (s *AudioStream) ReadAll() ([]byte, error) {
	return io.ReadAll(s)
}

// TotalSize 返回已接收的总大小
func (s *AudioStream) TotalSize() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.totalSize
}

// Error 返回错误
func (s *AudioStream) Error() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// setSessionCloser 设置 session closer（内部使用）
func (s *AudioStream) setSessionCloser(closer io.Closer) {
	s.sessionCloser = closer
	// 如果是 *Session 类型，同时保存引用以便访问时间信息
	if session, ok := closer.(*Session); ok {
		s.session = session
	}
}

// Close 关闭流
func (s *AudioStream) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()

		s.chunkChMu.Lock()
		if !s.chunkChClosed {
			s.chunkChClosed = true
			close(s.chunksCh)
		}
		s.chunkChMu.Unlock()

		close(s.closeCh)

		// 关闭底层 session（会关闭 WebSocket 连接）
		if s.sessionCloser != nil {
			s.sessionCloser.Close()
		}
	})
	return nil
}

// IsClosed 检查是否已关闭
func (s *AudioStream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// CommitSentAt 返回 input.commit 发送时间
func (s *AudioStream) CommitSentAt() time.Time {
	if s.session == nil {
		return time.Time{}
	}
	return s.session.CommitSentAt()
}

// FirstChunkReceivedAt 返回首个 audio.delta 收到时间
// 这个时间在 WebSocket 层接收到消息时立即记录，比应用层 Read() 更精确
func (s *AudioStream) FirstChunkReceivedAt() time.Time {
	if s.session == nil {
		return time.Time{}
	}
	return s.session.FirstChunkReceivedAt()
}

// TTFB 返回从 commit 发送到首包收到的时间（毫秒）
// 这是真正的 Time To First Byte，不包含建连和配置时间
func (s *AudioStream) TTFB() int64 {
	if s.session == nil {
		return 0
	}
	return s.session.TTFB()
}

// ConnectDuration 返回建连耗时（TCP+TLS+WS握手）
func (s *AudioStream) ConnectDuration() time.Duration {
	if s.session == nil {
		return 0
	}
	return s.session.ConnectDuration()
}

// ConnectedAt 返回建连完成时间
func (s *AudioStream) ConnectedAt() time.Time {
	if s.session == nil {
		return time.Time{}
	}
	return s.session.ConnectedAt()
}
