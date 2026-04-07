// Package protocol 定义统一 WebSocket 消息协议
package protocol

import (
	"encoding/json"
)

// MessageType 消息类型
type MessageType string

const (
	// 阶段 1: 会话生命周期
	MessageTypeSessionReady      MessageType = "session.ready"
	MessageTypeSessionConfig     MessageType = "session.config"
	MessageTypeSessionConfigDone MessageType = "session.config_done"

	// 阶段 2: 客户端输入
	MessageTypeAudioAppend MessageType = "audio.append"
	MessageTypeTextAppend  MessageType = "text.append"
	MessageTypeInputCommit MessageType = "input.commit" // TTS 触发合成

	// 阶段 3: 服务端处理与输出
	MessageTypeSpeechStarted     MessageType = "speech.started"
	MessageTypeTranscriptPartial MessageType = "transcript.partial"
	MessageTypeTranscriptFinal   MessageType = "transcript.final"
	MessageTypeAudioDelta        MessageType = "audio.delta"
	MessageTypeAudioDone         MessageType = "audio.done"

	// 阶段 4: 会话结束
	MessageTypeSessionEnd   MessageType = "session.end"   // TTS: 关闭会话；STT: 音频发完，请完成识别
	MessageTypeSessionEnded MessageType = "session.ended"  // STT 识别完成，服务端关闭连接

	// 错误
	MessageTypeError MessageType = "error"
)

// SessionParams 会话参数
type SessionParams struct {
	Provider    string `json:"provider,omitempty"`     // azure, qwen, voxnexus
	Language    string `json:"language,omitempty"`     // zh-CN, en-US
	SampleRate  int    `json:"sample_rate,omitempty"`  // 默认 8000
	AudioFormat string `json:"audio_format,omitempty"` // pcm, wav, mp3
	// TTS 特有参数
	VoiceID string  `json:"voice_id,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
	Pitch   float64 `json:"pitch,omitempty"`
	Volume  float64 `json:"volume,omitempty"`
}

// ──────────────────────────────────────────────
// 通用消息
// ──────────────────────────────────────────────

// Message 通用消息结构
type Message struct {
	Type MessageType `json:"type"`
}

// ──────────────────────────────────────────────
// 阶段 1: 会话生命周期
// ──────────────────────────────────────────────

// SessionReady 会话就绪消息（S→C，连接建立后服务端首条消息）
type SessionReady struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id"`
}

// SessionConfig 会话配置消息（C→S）
type SessionConfig struct {
	Type    MessageType   `json:"type"`
	Session SessionParams `json:"session"`
}

// SessionConfigDone 会话配置完成消息（S→C）
type SessionConfigDone struct {
	Type MessageType `json:"type"`
}

// ──────────────────────────────────────────────
// 阶段 2: 客户端输入
// ──────────────────────────────────────────────

// AudioAppend 音频数据消息（C→S，STT）
type AudioAppend struct {
	Type  MessageType `json:"type"`
	Audio string      `json:"audio"` // base64 编码的音频数据
}

// TextAppend 文本数据消息（C→S，TTS）
type TextAppend struct {
	Type MessageType `json:"type"`
	Text string      `json:"text"`
}

// InputCommit 输入提交消息（C→S，TTS 专用）
type InputCommit struct {
	Type MessageType `json:"type"`
}

// ──────────────────────────────────────────────
// 阶段 3: 服务端处理与输出
// ──────────────────────────────────────────────

// SpeechStarted VAD 检测到用户开始说话（S→C，STT）
type SpeechStarted struct {
	Type MessageType `json:"type"`
}

// TranscriptPartial 部分识别结果（S→C，STT）
type TranscriptPartial struct {
	Type MessageType `json:"type"`
	Text string      `json:"text"`
}

// TranscriptFinal 最终识别结果（S→C，STT）
type TranscriptFinal struct {
	Type      MessageType `json:"type"`
	Text      string      `json:"text"`
	StartTime int64       `json:"start_time,omitempty"` // 毫秒
	EndTime   int64       `json:"end_time,omitempty"`
}

// AudioDelta 音频数据块（S→C，TTS）
type AudioDelta struct {
	Type  MessageType `json:"type"`
	Audio string      `json:"audio"` // base64 编码的音频数据
}

// AudioDone 音频完成消息（S→C，TTS）
type AudioDone struct {
	Type MessageType `json:"type"`
}

// ──────────────────────────────────────────────
// 阶段 4: 会话结束
// ──────────────────────────────────────────────

// SessionEnd 会话结束消息（C→S，TTS: 关闭会话；STT: 音频发完，请完成识别）
type SessionEnd struct {
	Type MessageType `json:"type"`
}

// SessionEnded STT 识别完成消息（S→C，服务端发送后关闭连接）
type SessionEnded struct {
	Type MessageType `json:"type"`
}

// ──────────────────────────────────────────────
// 错误
// ──────────────────────────────────────────────

// ErrorMessage 错误消息
type ErrorMessage struct {
	Type    MessageType `json:"type"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
}

// 错误代码
const (
	ErrorCodeInvalidMessage     = "INVALID_MESSAGE"
	ErrorCodeProviderError      = "PROVIDER_ERROR"
	ErrorCodeAuthError          = "AUTH_ERROR"
	ErrorCodeRateLimitError     = "RATE_LIMIT_ERROR"
	ErrorCodeConfigError        = "CONFIG_ERROR"
	ErrorCodeInternalError      = "INTERNAL_ERROR"
	ErrorCodeUnsupported        = "UNSUPPORTED"
	ErrorCodeServiceUnavailable = "SERVICE_UNAVAILABLE" // 服务不可用（内部配置问题）
	ErrorCodeVoiceNotFound      = "VOICE_NOT_FOUND"     // 音色不存在
)

// ──────────────────────────────────────────────
// 构造函数（按协议阶段排序）
// ──────────────────────────────────────────────

// NewSessionReady 创建会话就绪消息
func NewSessionReady(sessionID string) *SessionReady {
	return &SessionReady{
		Type:      MessageTypeSessionReady,
		SessionID: sessionID,
	}
}

// NewSessionConfigDone 创建会话配置完成消息
func NewSessionConfigDone() *SessionConfigDone {
	return &SessionConfigDone{
		Type: MessageTypeSessionConfigDone,
	}
}

// NewSpeechStarted 创建语音开始消息
func NewSpeechStarted() *SpeechStarted {
	return &SpeechStarted{Type: MessageTypeSpeechStarted}
}

// NewTranscriptPartial 创建部分识别结果
func NewTranscriptPartial(text string) *TranscriptPartial {
	return &TranscriptPartial{
		Type: MessageTypeTranscriptPartial,
		Text: text,
	}
}

// NewTranscriptFinal 创建最终识别结果
func NewTranscriptFinal(text string, startTime, endTime int64) *TranscriptFinal {
	return &TranscriptFinal{
		Type:      MessageTypeTranscriptFinal,
		Text:      text,
		StartTime: startTime,
		EndTime:   endTime,
	}
}

// NewAudioDelta 创建音频数据块
func NewAudioDelta(audioBase64 string) *AudioDelta {
	return &AudioDelta{
		Type:  MessageTypeAudioDelta,
		Audio: audioBase64,
	}
}

// NewAudioDone 创建音频完成消息
func NewAudioDone() *AudioDone {
	return &AudioDone{
		Type: MessageTypeAudioDone,
	}
}

// NewSessionEnded 创建 STT 识别完成消息
func NewSessionEnded() *SessionEnded {
	return &SessionEnded{
		Type: MessageTypeSessionEnded,
	}
}

// NewError 创建错误消息
func NewError(code, message string) *ErrorMessage {
	return &ErrorMessage{
		Type:    MessageTypeError,
		Code:    code,
		Message: message,
	}
}

// ──────────────────────────────────────────────
// 解析函数
// ──────────────────────────────────────────────

// ParseMessage 解析消息类型
func ParseMessage(data []byte) (MessageType, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return "", err
	}
	return msg.Type, nil
}

// ParseSessionConfig 解析会话配置
func ParseSessionConfig(data []byte) (*SessionConfig, error) {
	var msg SessionConfig
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ParseAudioAppend 解析音频数据
func ParseAudioAppend(data []byte) (*AudioAppend, error) {
	var msg AudioAppend
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ParseTextAppend 解析文本数据
func ParseTextAppend(data []byte) (*TextAppend, error) {
	var msg TextAppend
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
