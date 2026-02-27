// Package protocol 定义统一 WebSocket 消息协议
package protocol

import (
	"encoding/json"
)

// MessageType 消息类型
type MessageType string

const (
	// 客户端消息类型
	MessageTypeSessionConfig MessageType = "session.config"
	MessageTypeAudioAppend   MessageType = "audio.append"
	MessageTypeTextAppend    MessageType = "text.append"
	MessageTypeInputCommit   MessageType = "input.commit"
	MessageTypeSessionEnd    MessageType = "session.end"

	// 服务端消息类型
	MessageTypeSessionReady      MessageType = "session.ready"
	MessageTypeSessionConfigDone MessageType = "session.config_done"
	MessageTypeTranscriptPartial MessageType = "transcript.partial"
	MessageTypeTranscriptFinal   MessageType = "transcript.final"
	MessageTypeAudioDelta        MessageType = "audio.delta"
	MessageTypeAudioDone         MessageType = "audio.done"
	MessageTypeInputDone         MessageType = "input.done"
	MessageTypeProcessing        MessageType = "processing"
	MessageTypeError             MessageType = "error"
)

// Message 通用消息结构
type Message struct {
	Type MessageType `json:"type"`
}

// SessionConfig 会话配置消息
type SessionConfig struct {
	Type    MessageType   `json:"type"`
	Session SessionParams `json:"session"`
}

// SessionParams 会话参数
type SessionParams struct {
	Provider    string `json:"provider,omitempty"`     // azure, qwen, voxnexus
	Language    string `json:"language,omitempty"`     // zh-CN, en-US
	SampleRate  int    `json:"sample_rate,omitempty"`  // 16000
	AudioFormat string `json:"audio_format,omitempty"` // pcm, wav, mp3

	// TTS 特有参数
	VoiceID string  `json:"voice_id,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
	Pitch   float64 `json:"pitch,omitempty"`
	Volume  float64 `json:"volume,omitempty"`

}

// AudioAppend 音频数据消息（STT）
type AudioAppend struct {
	Type  MessageType `json:"type"`
	Audio string      `json:"audio"` // base64 编码的音频数据
}

// TextAppend 文本数据消息（TTS）
type TextAppend struct {
	Type MessageType `json:"type"`
	Text string      `json:"text"`
}

// InputCommit 输入提交消息
type InputCommit struct {
	Type MessageType `json:"type"`
}

// SessionEnd 会话结束消息
type SessionEnd struct {
	Type MessageType `json:"type"`
}

// SessionReady 会话就绪消息
type SessionReady struct {
	Type      MessageType `json:"type"`
	SessionID string      `json:"session_id"`
}

// SessionConfigDone 会话配置完成消息
type SessionConfigDone struct {
	Type MessageType `json:"type"`
}

// TranscriptPartial 部分识别结果（STT）
type TranscriptPartial struct {
	Type MessageType `json:"type"`
	Text string      `json:"text"`
}

// TranscriptFinal 最终识别结果（STT）
type TranscriptFinal struct {
	Type       MessageType `json:"type"`
	Text      string      `json:"text"`
	StartTime int64       `json:"start_time,omitempty"` // 毫秒
	EndTime   int64       `json:"end_time,omitempty"`
}

// AudioDelta 音频数据块（TTS）
type AudioDelta struct {
	Type  MessageType `json:"type"`
	Audio string      `json:"audio"` // base64 编码的音频数据
}

// AudioDone 音频完成消息（TTS）
type AudioDone struct {
	Type MessageType `json:"type"`
}

// InputDone 识别完成消息（STT）
type InputDone struct {
	Type MessageType `json:"type"`
}

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
	ErrorCodeVoiceNotFound      = "VOICE_NOT_FOUND"      // 音色不存在
)

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

// Processing 处理中心跳消息（STT）
type Processing struct {
	Type MessageType `json:"type"`
}

// NewError 创建错误消息
func NewError(code, message string) *ErrorMessage {
	return &ErrorMessage{
		Type:    MessageTypeError,
		Code:    code,
		Message: message,
	}
}

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
