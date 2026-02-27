// Package transport 消息处理
package transport

import (
	"encoding/json"
	"fmt"

	"github.com/jinbozhan/tengen-speech-sdk-go/protocol"
)

// Message 消息接口
type Message interface {
	GetType() protocol.MessageType
}

// RawMessage 原始消息（用于解析消息类型）
type RawMessage struct {
	Type protocol.MessageType `json:"type"`
}

// ParseMessageType 解析消息类型
func ParseMessageType(data []byte) (protocol.MessageType, error) {
	var raw RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", fmt.Errorf("parse message type: %w", err)
	}
	return raw.Type, nil
}

// ParseMessage 解析消息为具体类型
func ParseMessage(data []byte) (interface{}, error) {
	msgType, err := ParseMessageType(data)
	if err != nil {
		return nil, err
	}

	var msg interface{}
	switch msgType {
	// 服务端消息
	case protocol.MessageTypeSessionReady:
		msg = &protocol.SessionReady{}
	case protocol.MessageTypeTranscriptPartial:
		msg = &protocol.TranscriptPartial{}
	case protocol.MessageTypeTranscriptFinal:
		msg = &protocol.TranscriptFinal{}
	case protocol.MessageTypeAudioDelta:
		msg = &protocol.AudioDelta{}
	case protocol.MessageTypeAudioDone:
		msg = &protocol.AudioDone{}
	case protocol.MessageTypeInputDone:
		msg = &protocol.InputDone{}
	case protocol.MessageTypeProcessing:
		msg = &protocol.Processing{}
	case protocol.MessageTypeError:
		msg = &protocol.ErrorMessage{}

	// 客户端消息（通常不需要解析，但保留以便调试）
	case protocol.MessageTypeSessionConfig:
		msg = &protocol.SessionConfig{}
	case protocol.MessageTypeAudioAppend:
		msg = &protocol.AudioAppend{}
	case protocol.MessageTypeTextAppend:
		msg = &protocol.TextAppend{}
	case protocol.MessageTypeInputCommit:
		msg = &protocol.InputCommit{}
	case protocol.MessageTypeSessionEnd:
		msg = &protocol.SessionEnd{}

	default:
		return nil, fmt.Errorf("unknown message type: %s", msgType)
	}

	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("parse message body: %w", err)
	}

	return msg, nil
}

// EncodeMessage 编码消息为JSON
func EncodeMessage(msg interface{}) ([]byte, error) {
	return json.Marshal(msg)
}

// NewSessionConfig 创建会话配置消息
func NewSessionConfig(params protocol.SessionParams) *protocol.SessionConfig {
	return &protocol.SessionConfig{
		Type:    protocol.MessageTypeSessionConfig,
		Session: params,
	}
}

// NewAudioAppend 创建音频数据消息
func NewAudioAppend(audioBase64 string) *protocol.AudioAppend {
	return &protocol.AudioAppend{
		Type:  protocol.MessageTypeAudioAppend,
		Audio: audioBase64,
	}
}

// NewTextAppend 创建文本数据消息
func NewTextAppend(text string) *protocol.TextAppend {
	return &protocol.TextAppend{
		Type: protocol.MessageTypeTextAppend,
		Text: text,
	}
}

// NewInputCommit 创建输入提交消息
func NewInputCommit() *protocol.InputCommit {
	return &protocol.InputCommit{
		Type: protocol.MessageTypeInputCommit,
	}
}

// NewSessionEnd 创建会话结束消息
func NewSessionEnd() *protocol.SessionEnd {
	return &protocol.SessionEnd{
		Type: protocol.MessageTypeSessionEnd,
	}
}

// IsServerMessage 判断是否为服务端消息
func IsServerMessage(msgType protocol.MessageType) bool {
	switch msgType {
	case protocol.MessageTypeSessionReady,
		protocol.MessageTypeTranscriptPartial,
		protocol.MessageTypeTranscriptFinal,
		protocol.MessageTypeAudioDelta,
		protocol.MessageTypeAudioDone,
		protocol.MessageTypeInputDone,
		protocol.MessageTypeProcessing,
		protocol.MessageTypeError:
		return true
	default:
		return false
	}
}

// IsClientMessage 判断是否为客户端消息
func IsClientMessage(msgType protocol.MessageType) bool {
	switch msgType {
	case protocol.MessageTypeSessionConfig,
		protocol.MessageTypeAudioAppend,
		protocol.MessageTypeTextAppend,
		protocol.MessageTypeInputCommit,
		protocol.MessageTypeSessionEnd:
		return true
	default:
		return false
	}
}

// IsErrorMessage 判断是否为错误消息
func IsErrorMessage(msgType protocol.MessageType) bool {
	return msgType == protocol.MessageTypeError
}
