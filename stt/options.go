// Package stt 提供STT客户端
package stt

import "time"

// Config STT客户端配置
type Config struct {
	// Gateway配置
	GatewayURL string // Gateway WebSocket URL，如 ws://localhost:8080
	Provider   string // 提供商: tengen (默认), azure, qwen_realtime, voxnexus
	APIKey     string // API Key 认证（可选，通过URL参数传递）

	// 识别参数
	Language     string // 识别语言: zh-CN, en-US
	SampleRate   int    // 采样率: 16000, 8000
	AudioFormat  string // 音频格式: pcm, wav
	// 连接配置
	ConnectTimeout   time.Duration // 连接超时
	ReadTimeout      time.Duration // 读超时
	WriteTimeout     time.Duration // 写超时
	ReconnectBackoff time.Duration // 重连退避基数
	MaxReconnects    int           // 最大重连次数
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		GatewayURL:       "ws://localhost:8080",
		Provider:         "tengen",
		Language:         "zh-CN",
		SampleRate:       16000,
		AudioFormat:      "pcm",
		ConnectTimeout:   10 * time.Second,
		ReadTimeout:      60 * time.Second,
		WriteTimeout:     10 * time.Second,
		ReconnectBackoff: 1 * time.Second,
		MaxReconnects:    3,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.GatewayURL == "" {
		return ErrInvalidConfig("GatewayURL is required")
	}
	if c.Provider == "" {
		return ErrInvalidConfig("Provider is required")
	}
	if c.SampleRate <= 0 {
		c.SampleRate = 16000
	}
	if c.Language == "" {
		c.Language = "zh-CN"
	}
	if c.AudioFormat == "" {
		c.AudioFormat = "pcm"
	}
	return nil
}

// WithProvider 设置提供商
func (c *Config) WithProvider(provider string) *Config {
	c.Provider = provider
	return c
}

// WithLanguage 设置识别语言
func (c *Config) WithLanguage(language string) *Config {
	c.Language = language
	return c
}

// WithSampleRate 设置采样率
func (c *Config) WithSampleRate(sampleRate int) *Config {
	c.SampleRate = sampleRate
	return c
}

// WithAPIKey 设置API Key
func (c *Config) WithAPIKey(apiKey string) *Config {
	c.APIKey = apiKey
	return c
}

// StreamOptions 流式识别选项
type StreamOptions struct {
	Language    string // 识别语言
	SampleRate  int    // 采样率
	AudioFormat string // 音频格式
}

// DefaultStreamOptions 返回默认流式选项
func DefaultStreamOptions() *StreamOptions {
	return &StreamOptions{
		Language:    "zh-CN",
		SampleRate:  16000,
		AudioFormat: "pcm",
	}
}

// ConfigError 配置错误
type configError struct {
	message string
}

func (e *configError) Error() string {
	return "stt config: " + e.message
}

// ErrInvalidConfig 创建配置错误
func ErrInvalidConfig(message string) error {
	return &configError{message: message}
}
