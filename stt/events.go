// Package stt 识别事件定义
package stt

import "time"

// EventType 事件类型
type EventType string

const (
	// EventSessionReady 会话就绪（对应 MessageType "session.ready"）
	EventSessionReady EventType = "session.ready"
	// EventTranscriptPartial 部分识别结果（对应 MessageType "transcript.partial"）
	EventTranscriptPartial EventType = "transcript.partial"
	// EventTranscriptFinal 最终识别结果（对应 MessageType "transcript.final"）
	EventTranscriptFinal EventType = "transcript.final"
	// EventError 错误（对应 MessageType "error"）
	EventError EventType = "error"
	// EventSessionEnded 识别完成（对应 MessageType "session.ended"）
	EventSessionEnded EventType = "session.ended"
	// EventProcessing 处理中心跳（对应 MessageType "processing"）
	EventProcessing EventType = "processing"
	// EventSpeechStarted 用户开始说话（对应 MessageType "speech.started"）
	EventSpeechStarted EventType = "speech.started"
	// EventSessionClosed 会话关闭（客户端专属事件）
	EventSessionClosed EventType = "session.closed"
)

// RecognitionEvent 识别事件
type RecognitionEvent struct {
	Type      EventType     // 事件类型
	SessionID string        // 会话ID
	Text      string        // 识别文本
	IsFinal   bool          // 是否最终结果
	StartTime time.Duration // 开始时间
	EndTime   time.Duration // 结束时间
	Error     error         // 错误（仅EventError时有效）
}

// IsSessionReady 是否为就绪事件
func (e *RecognitionEvent) IsSessionReady() bool {
	return e.Type == EventSessionReady
}

// IsTranscriptPartial 是否为部分结果
func (e *RecognitionEvent) IsTranscriptPartial() bool {
	return e.Type == EventTranscriptPartial
}

// IsTranscriptFinal 是否为最终结果
func (e *RecognitionEvent) IsTranscriptFinal() bool {
	return e.Type == EventTranscriptFinal
}

// IsError 是否为错误事件
func (e *RecognitionEvent) IsError() bool {
	return e.Type == EventError
}

// IsSessionClosed 是否为关闭事件
func (e *RecognitionEvent) IsSessionClosed() bool {
	return e.Type == EventSessionClosed
}

// NewSessionReadyEvent 创建就绪事件
func NewSessionReadyEvent(sessionID string) *RecognitionEvent {
	return &RecognitionEvent{
		Type:      EventSessionReady,
		SessionID: sessionID,
	}
}

// NewTranscriptPartialEvent 创建部分识别事件
func NewTranscriptPartialEvent(text string) *RecognitionEvent {
	return &RecognitionEvent{
		Type:    EventTranscriptPartial,
		Text:    text,
		IsFinal: false,
	}
}

// NewTranscriptFinalEvent 创建最终识别事件
func NewTranscriptFinalEvent(text string, startTime, endTime time.Duration) *RecognitionEvent {
	return &RecognitionEvent{
		Type:      EventTranscriptFinal,
		Text:      text,
		IsFinal:   true,
		StartTime: startTime,
		EndTime:   endTime,
	}
}

// NewErrorEvent 创建错误事件
func NewErrorEvent(err error) *RecognitionEvent {
	return &RecognitionEvent{
		Type:  EventError,
		Error: err,
	}
}

// NewSessionEndedEvent 创建识别完成事件
func NewSessionEndedEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventSessionEnded,
	}
}

// NewProcessingEvent 创建处理中心跳事件
func NewProcessingEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventProcessing,
	}
}

// NewSpeechStartedEvent 创建语音开始事件
func NewSpeechStartedEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventSpeechStarted,
	}
}

// NewSessionClosedEvent 创建关闭事件
func NewSessionClosedEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventSessionClosed,
	}
}

// RecognitionResult 完整识别结果
type RecognitionResult struct {
	Text     string         // 完整文本
	Segments []Segment      // 分段结果
	Duration time.Duration  //
	TTFB     time.Duration  // 首个识别结果延迟
	Error    error
}

// Segment 识别分段
type Segment struct {
	Text      string
	IsFinal   bool
	StartTime time.Duration
	EndTime   time.Duration
}

// EventHandler 事件处理回调
type EventHandler func(event *RecognitionEvent)
