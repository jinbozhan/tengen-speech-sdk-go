// Package tts 提供TTS客户端SDK
package tts

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/transport"
)

// Client TTS客户端
type Client struct {
	config *Config
}

// NewClient 创建TTS客户端
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Client{config: config}, nil
}

// SynthesizeToFile 合成到文件（简化API）
func (c *Client) SynthesizeToFile(ctx context.Context, text, outputPath string) error {
	start := time.Now()

	// 创建流式会话
	stream, err := c.SynthesizeStream(ctx, text)
	if err != nil {
		return err
	}
	defer stream.Close()

	// 创建输出文件
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	// 复制音频数据到文件
	written, err := io.Copy(file, stream)
	if err != nil {
		return fmt.Errorf("write audio: %w", err)
	}

	log.Printf("[client.tts] SynthesizeToFile completed: output=%s, size=%d bytes, duration=%dms",
		outputPath, written, time.Since(start).Milliseconds())

	return stream.Error()
}

// SynthesizeToBytes 合成到内存（简化API）
func (c *Client) SynthesizeToBytes(ctx context.Context, text string) ([]byte, error) {
	stream, err := c.SynthesizeStream(ctx, text)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	data, err := stream.ReadAll()
	if err != nil {
		return nil, err
	}

	if streamErr := stream.Error(); streamErr != nil {
		return nil, streamErr
	}

	return data, nil
}

// SynthesizeStream 流式合成（高级API）
// 返回音频流，可逐块接收或作为io.Reader使用
func (c *Client) SynthesizeStream(ctx context.Context, text string) (*AudioStream, error) {
	opts := &SynthesisOptions{
		VoiceID:     c.config.VoiceID,
		Speed:       c.config.Speed,
		Pitch:       c.config.Pitch,
		Volume:      c.config.Volume,
		SampleRate:  c.config.SampleRate,
		AudioFormat: c.config.AudioFormat,
	}

	session, err := c.createSession(ctx, opts)
	if err != nil {
		return nil, err
	}

	// 使用 session.SynthesizeStream 进行合成
	stream, err := session.SynthesizeStream(ctx, text)
	if err != nil {
		session.Close()
		return nil, err
	}

	stream.setSessionCloser(session) // 让 stream.Close() 能关闭 session
	return stream, nil
}

// SynthesizeStreamWithOptions 带选项的流式合成
func (c *Client) SynthesizeStreamWithOptions(ctx context.Context, text string, opts *SynthesisOptions) (*AudioStream, error) {
	if opts == nil {
		opts = DefaultSynthesisOptions()
	}

	session, err := c.createSession(ctx, opts)
	if err != nil {
		return nil, err
	}

	// 使用 session.SynthesizeStream 进行合成
	stream, err := session.SynthesizeStream(ctx, text)
	if err != nil {
		session.Close()
		return nil, err
	}

	stream.setSessionCloser(session) // 让 stream.Close() 能关闭 session
	return stream, nil
}

// CreateSession 创建TTS会话（支持多轮合成）
// 会话建立后可重复调用 SynthesizeStream() 进行多次合成
// 适用于对话TTS、长文本分段合成等场景
func (c *Client) CreateSession(ctx context.Context, opts *SynthesisOptions) (*Session, error) {
	if opts == nil {
		opts = DefaultSynthesisOptions()
	}
	return c.createSession(ctx, opts)
}

// createSession 内部创建会话
func (c *Client) createSession(ctx context.Context, opts *SynthesisOptions) (*Session, error) {
	// 构建WebSocket URL
	wsURL := fmt.Sprintf("%s/ws/tts?provider=%s", c.config.GatewayURL, c.config.Provider)
	// 添加 voice_id 到 URL 以便 Gateway 精准预热
	if c.config.VoiceID != "" {
		wsURL += "&voice_id=" + url.QueryEscape(c.config.VoiceID)
	}
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

// Close 关闭客户端
func (c *Client) Close() error {
	return nil
}

// Config 返回当前配置
func (c *Client) Config() *Config {
	return c.config
}
