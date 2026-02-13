// Package tts 提供TTS客户端
package tts

import "time"

// Config TTS客户端配置
type Config struct {
	// Gateway配置
	GatewayURL string // Gateway WebSocket URL
	Provider   string // 提供商: tengen (默认), azure, qwen_realtime, voxnexus
	APIKey     string // API Key 认证（可选，通过URL参数传递）

	// 合成参数
	VoiceID     string  // 语音ID（克隆声音）
	Language    string  // 语言代码: en-NG, sw-TZ 等（用于文本归一化）
	Speed       float64 // 语速 0.5-2.0
	Pitch       float64 // 音调 -10 to 10
	Volume      float64 // 音量 0.0-1.0
	SampleRate  int     // 采样率 (Hz)
	AudioFormat string  // 音频格式: pcm, wav, mp3

	// 连接配置
	ConnectTimeout   time.Duration
	ReadTimeout      time.Duration
	WriteTimeout     time.Duration
	ReconnectBackoff time.Duration
	MaxReconnects    int
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		GatewayURL:       "ws://localhost:8080",
		Provider:         "tengen",
		VoiceID:          "",
		Speed:            1.0,
		Pitch:            1.0,
		Volume:           1.0,
		SampleRate:       8000,
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
	if c.Speed <= 0 {
		c.Speed = 1.0
	}
	if c.Volume <= 0 {
		c.Volume = 1.0
	}
	return nil
}

// WithProvider 设置提供商
func (c *Config) WithProvider(provider string) *Config {
	c.Provider = provider
	return c
}

// WithVoice 设置语音ID
func (c *Config) WithVoice(voiceID string) *Config {
	c.VoiceID = voiceID
	return c
}

// WithLanguage 设置语言代码（用于文本归一化）
func (c *Config) WithLanguage(language string) *Config {
	c.Language = language
	return c
}

// WithSpeed 设置语速
func (c *Config) WithSpeed(speed float64) *Config {
	c.Speed = speed
	return c
}

// WithPitch 设置音调
func (c *Config) WithPitch(pitch float64) *Config {
	c.Pitch = pitch
	return c
}

// WithVolume 设置音量
func (c *Config) WithVolume(volume float64) *Config {
	c.Volume = volume
	return c
}

// WithAPIKey 设置API Key
func (c *Config) WithAPIKey(apiKey string) *Config {
	c.APIKey = apiKey
	return c
}

// WithSampleRate 设置采样率
func (c *Config) WithSampleRate(sampleRate int) *Config {
	c.SampleRate = sampleRate
	return c
}

// WithAudioFormat 设置音频格式
func (c *Config) WithAudioFormat(audioFormat string) *Config {
	c.AudioFormat = audioFormat
	return c
}

// SynthesisOptions 合成选项
type SynthesisOptions struct {
	VoiceID     string  // 语音ID
	Language    string  // 语言代码
	Speed       float64 // 语速
	Pitch       float64 // 音调
	Volume      float64 // 音量
	SampleRate  int     // 采样率 (Hz)
	AudioFormat string  // 音频格式: pcm, wav, mp3
}

// DefaultSynthesisOptions 返回默认合成选项
func DefaultSynthesisOptions() *SynthesisOptions {
	return &SynthesisOptions{
		Speed:       1.0,
		Pitch:       1.0,
		Volume:      1.0,
		SampleRate:  8000,
		AudioFormat: "pcm",
	}
}

// ConfigError 配置错误
type configError struct {
	message string
}

func (e *configError) Error() string {
	return "tts config: " + e.message
}

// ErrInvalidConfig 创建配置错误
func ErrInvalidConfig(message string) error {
	return &configError{message: message}
}
