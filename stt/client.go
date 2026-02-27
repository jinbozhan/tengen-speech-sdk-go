// Package stt 提供STT客户端SDK
package stt

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/transport"
)

const (
	// commit 后无新事件的空闲超时，超时即认为识别完成
	recognizeIdleTimeout = 10 * time.Second
)

// Client STT客户端
type Client struct {
	config *Config
}

// NewClient 创建STT客户端
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Client{config: config}, nil
}

// RecognizeFile 识别音频文件（简化API）
// 自动处理连接、会话、文件读取和关闭
func (c *Client) RecognizeFile(ctx context.Context, audioPath string) (*RecognitionResult, error) {
	start := time.Now()

	// 检查文件是否存在
	if _, err := os.Stat(audioPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("audio file not found: %s", audioPath)
	}

	// 打开音频文件
	file, err := os.Open(audioPath)
	if err != nil {
		return nil, fmt.Errorf("open audio file: %w", err)
	}
	defer file.Close()

	// 创建流式会话
	opts := &StreamOptions{
		Language:    c.config.Language,
		SampleRate:  c.config.SampleRate,
		AudioFormat: c.config.AudioFormat,
	}

	session, err := c.RecognizeStream(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	// 跳过WAV头（如果是WAV文件）
	if strings.HasSuffix(strings.ToLower(audioPath), ".wav") {
		skipWavHeader(file)
	}

	// 发送音频数据
	result := &RecognitionResult{
		Segments: make([]Segment, 0),
	}
	var texts []string

	// goroutine: 发送音频 + commit
	sendDoneCh := make(chan error, 1)
	go func() {
		if err := c.sendAudioFromReader(session, file); err != nil {
			sendDoneCh <- err
			return
		}
		sendDoneCh <- session.Commit()
	}()

	// 事件循环：commit 后启动空闲计时器，每收到事件重置
	// 如果 idleTimeout 内无新事件到达，认为识别完成
	committed := false
	idleTimer := time.NewTimer(0)
	if !idleTimer.Stop() {
		<-idleTimer.C
	}
	defer idleTimer.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		case err := <-sendDoneCh:
			if err != nil {
				return nil, fmt.Errorf("send audio: %w", err)
			}
			committed = true
			sendDoneCh = nil // 防止重复读取
			idleTimer.Reset(recognizeIdleTimeout)

		case event, ok := <-session.Events():
			if !ok {
				break loop
			}
			if committed {
				idleTimer.Reset(recognizeIdleTimeout)
			}
			switch event.Type {
			case EventPartial:
				// 部分结果，可以用于实时显示
			case EventFinal:
				texts = append(texts, event.Text)
				result.Segments = append(result.Segments, Segment{
					Text:      event.Text,
					IsFinal:   true,
					StartTime: event.StartTime,
					EndTime:   event.EndTime,
				})
			case EventError:
				result.Error = event.Error
			case EventInputDone:
				break loop
			case EventClosed:
				break loop
			}

		case <-idleTimer.C:
			log.Printf("[client.stt] Idle timeout %v after last event, closing (no input.done received)", recognizeIdleTimeout)
			break loop
		}
	}

	// 合并文本
	result.Text = strings.Join(texts, "")
	result.Duration = time.Since(start)

	log.Printf("[client.stt] RecognizeFile completed: file=%s, text=%s, duration=%dms",
		audioPath, truncateText(result.Text, 50), result.Duration.Milliseconds())

	return result, result.Error
}

// sendAudioFromReader 从Reader发送音频
func (c *Client) sendAudioFromReader(session *Session, reader io.Reader) error {
	// 100ms音频块 @ 16kHz, 16-bit = 3200字节
	chunkSize := c.config.SampleRate * 2 / 10 // 100ms
	buf := make([]byte, chunkSize)

	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if n > 0 {
			if err := session.Send(buf[:n]); err != nil {
				return err
			}
			// 模拟实时发送（可选）
			// time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}

// RecognizeStream 流式识别（高级API）
// 返回会话对象，用户手动发送音频和接收结果
func (c *Client) RecognizeStream(ctx context.Context, opts *StreamOptions) (*Session, error) {
	if opts == nil {
		opts = DefaultStreamOptions()
	}

	// 构建WebSocket URL
	wsURL := fmt.Sprintf("%s/ws/stt?provider=%s", c.config.GatewayURL, c.config.Provider)
	if c.config.APIKey != "" {
		wsURL += "&api_key=" + url.QueryEscape(c.config.APIKey)
	}

	// 创建连接
	connConfig := &transport.Config{
		URL:              wsURL,
		ConnectTimeout:   c.config.ConnectTimeout,
		ReadTimeout:      c.config.ReadTimeout,
		WriteTimeout:     c.config.WriteTimeout,
		ReconnectBackoff: c.config.ReconnectBackoff,
		MaxReconnects:    c.config.MaxReconnects,
	}

	conn := transport.NewConn(connConfig)
	if err := conn.ConnectWithRetry(ctx); err != nil {
		return nil, fmt.Errorf("connect to gateway: %w", err)
	}

	// 创建会话
	session := newSession(conn, c.config, opts)

	// 启动会话
	if err := session.start(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("start session: %w", err)
	}

	return session, nil
}

// RecognizeBytes 识别音频字节（简化API）
func (c *Client) RecognizeBytes(ctx context.Context, audio []byte) (*RecognitionResult, error) {
	start := time.Now()

	// 创建流式会话
	opts := &StreamOptions{
		Language:    c.config.Language,
		SampleRate:  c.config.SampleRate,
		AudioFormat: c.config.AudioFormat,
	}

	session, err := c.RecognizeStream(ctx, opts)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	// 分块发送
	result := &RecognitionResult{
		Segments: make([]Segment, 0),
	}
	var texts []string

	// goroutine: 发送音频 + commit
	sendDoneCh := make(chan error, 1)
	go func() {
		chunkSize := c.config.SampleRate * 2 / 10 // 100ms
		for i := 0; i < len(audio); i += chunkSize {
			end := i + chunkSize
			if end > len(audio) {
				end = len(audio)
			}
			if err := session.Send(audio[i:end]); err != nil {
				sendDoneCh <- err
				return
			}
		}
		sendDoneCh <- session.Commit()
	}()

	// 事件循环：commit 后启动空闲计时器
	committed := false
	idleTimer := time.NewTimer(0)
	if !idleTimer.Stop() {
		<-idleTimer.C
	}
	defer idleTimer.Stop()

loop:
	for {
		select {
		case <-ctx.Done():
			break loop

		case err := <-sendDoneCh:
			if err != nil {
				return nil, fmt.Errorf("send audio: %w", err)
			}
			committed = true
			sendDoneCh = nil
			idleTimer.Reset(recognizeIdleTimeout)

		case event, ok := <-session.Events():
			if !ok {
				break loop
			}
			if committed {
				idleTimer.Reset(recognizeIdleTimeout)
			}
			switch event.Type {
			case EventFinal:
				texts = append(texts, event.Text)
				result.Segments = append(result.Segments, Segment{
					Text:      event.Text,
					IsFinal:   true,
					StartTime: event.StartTime,
					EndTime:   event.EndTime,
				})
			case EventError:
				result.Error = event.Error
			case EventInputDone:
				break loop
			case EventClosed:
				break loop
			}

		case <-idleTimer.C:
			log.Printf("[client.stt] Idle timeout %v after last event, closing (no input.done received)", recognizeIdleTimeout)
			break loop
		}
	}

	// 合并文本
	result.Text = strings.Join(texts, "")
	result.Duration = time.Since(start)

	return result, result.Error
}

// Close 关闭客户端
func (c *Client) Close() error {
	// 客户端本身无需关闭资源，每次识别创建独立会话
	return nil
}

// Config 返回当前配置
func (c *Client) Config() *Config {
	return c.config
}

// skipWavHeader 跳过WAV文件头（44字节）
func skipWavHeader(file *os.File) {
	file.Seek(44, io.SeekStart)
}

// truncateText 截断文本用于日志
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
