// Package stt 识别事件定义
package stt

import "time"

// EventType 事件类型
type EventType string

const (
	// EventReady 会话就绪
	EventReady EventType = "ready"
	// EventPartial 部分识别结果
	EventPartial EventType = "partial"
	// EventFinal 最终识别结果
	EventFinal EventType = "final"
	// EventError 错误
	EventError EventType = "error"
	// EventInputDone 识别完成（服务端通知）
	EventInputDone EventType = "input_done"
	// EventProcessing 处理中心跳（服务端保活）
	EventProcessing EventType = "processing"
	// EventClosed 会话关闭
	EventClosed EventType = "closed"
)

// RecognitionEvent 识别事件
type RecognitionEvent struct {
	Type       EventType     // 事件类型
	SessionID  string        // 会话ID
	Text      string        // 识别文本
	IsFinal   bool          // 是否最终结果
	StartTime time.Duration // 开始时间
	EndTime    time.Duration // 结束时间
	Error      error         // 错误（仅EventError时有效）
}

// IsReady 是否为就绪事件
func (e *RecognitionEvent) IsReady() bool {
	return e.Type == EventReady
}

// IsPartial 是否为部分结果
func (e *RecognitionEvent) IsPartial() bool {
	return e.Type == EventPartial
}

// IsFinalResult 是否为最终结果
func (e *RecognitionEvent) IsFinalResult() bool {
	return e.Type == EventFinal
}

// IsError 是否为错误事件
func (e *RecognitionEvent) IsError() bool {
	return e.Type == EventError
}

// IsClosed 是否为关闭事件
func (e *RecognitionEvent) IsClosed() bool {
	return e.Type == EventClosed
}

// NewReadyEvent 创建就绪事件
func NewReadyEvent(sessionID string) *RecognitionEvent {
	return &RecognitionEvent{
		Type:      EventReady,
		SessionID: sessionID,
	}
}

// NewPartialEvent 创建部分识别事件
func NewPartialEvent(text string) *RecognitionEvent {
	return &RecognitionEvent{
		Type:    EventPartial,
		Text:    text,
		IsFinal: false,
	}
}

// NewFinalEvent 创建最终识别事件
func NewFinalEvent(text string, startTime, endTime time.Duration) *RecognitionEvent {
	return &RecognitionEvent{
		Type:      EventFinal,
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

// NewInputDoneEvent 创建识别完成事件
func NewInputDoneEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventInputDone,
	}
}

// NewProcessingEvent 创建处理中心跳事件
func NewProcessingEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventProcessing,
	}
}

// NewClosedEvent 创建关闭事件
func NewClosedEvent() *RecognitionEvent {
	return &RecognitionEvent{
		Type: EventClosed,
	}
}

// RecognitionResult 完整识别结果
type RecognitionResult struct {
	Text     string    // 完整文本
	Segments []Segment // 分段结果
	Duration time.Duration
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
