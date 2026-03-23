# STT 流式架构：输入音频流与输出事件流

## 架构总览

三层架构，全双工 WebSocket 通信：

```
┌─────────────┐      ┌──────────────┐      ┌──────────────┐
│  protocol/  │      │  transport/  │      │    stt/      │
│  messages.go│ ──── │  conn.go     │ ──── │  session.go  │
│  (协议定义) │      │  message.go  │      │  events.go   │
│             │      │  (WebSocket) │      │  (业务逻辑)  │
└─────────────┘      └──────────────┘      └──────────────┘
```

音频上行和事件下行在同一个 WebSocket 连接上同时进行。

---

## 一、输入音频流

### 数据流路径

```
原始 PCM 字节 ──→ Base64 编码 ──→ audio.append JSON ──→ WebSocket Text Frame ──→ Gateway
```

### 核心实现

**`stt/session.go:231-252` — `Send(audio []byte)`**

```go
func (s *Session) Send(audio []byte) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.closed { return fmt.Errorf("session closed") }
    if !s.ready { return fmt.Errorf("session not ready") }

    encoded := base64.StdEncoding.EncodeToString(audio)   // Base64 编码
    msg := transport.NewAudioAppend(encoded)              // 构造 audio.append 消息
    if err := s.conn.SendJSON(msg); err != nil {          // WebSocket 发送
        return err
    }
    if s.firstSendTime.IsZero() {
        s.firstSendTime = time.Now()                      // 记录首包时间（用于 TTFB）
    }
    return nil
}
```

**关键设计点：**

| 方面 | 实现 | 代码位置 |
|------|------|---------|
| 编码方式 | Base64（膨胀约 33%，换取 JSON 协议统一性） | `session.go:243` |
| 并发安全 | `sync.Mutex` 保护 Send/EndInput | `session.go:232-233` |
| 状态检查 | 发送前检查 `closed` 和 `ready` | `session.go:235-240` |
| TTFB 起点 | 首次 Send 时记录 `firstSendTime` | `session.go:248-250` |

### 协议消息格式

**`protocol/messages.go:58-61`**

```json
{
  "type": "audio.append",
  "audio": "<base64 编码的音频数据>"
}
```

### 音频结束信号

**`stt/session.go:254-265` — `EndInput()`**

发送 `session.end` 告知 Gateway 音频流已全部发完，请完成剩余识别：

```json
{ "type": "session.end" }
```

### 典型调用模式（来自 `examples/stt_stream/main.go`）

```go
// 后台 goroutine 按 100ms 分片发送
chunkSize := sampleRate * 2 / 10   // 100ms 的 PCM 数据量
for {
    n, err := file.Read(buf[:chunkSize])
    if n > 0 {
        session.Send(buf[:n])
    }
    if err == io.EOF { break }
    time.Sleep(100 * time.Millisecond)  // 模拟实时流
}
session.EndInput()  // 标记输入完成
```

---

## 二、输出事件流

### 7 种事件类型

**`stt/events.go:9-24`**

| 事件类型 | 含义 | 携带数据 |
|---------|------|---------|
| `session.ready` | 会话就绪，可以开始发送音频 | SessionID |
| `transcript.partial` | 中间识别结果（会被后续结果覆盖） | Text |
| `transcript.final` | 最终识别结果（不可变） | Text, StartTime, EndTime |
| `speech.started` | VAD 检测到用户开始说话 | 无 |
| `processing` | 服务端处理中心跳 | 无 |
| `session.ended` | 识别完成，服务端即将关闭连接 | 无 |
| `error` | 错误 | Error |

### 事件结构体

**`stt/events.go:27-35`**

```go
type RecognitionEvent struct {
    Type      EventType     // 事件类型
    SessionID string        // 会话ID
    Text      string        // 识别文本
    IsFinal   bool          // 是否最终结果
    StartTime time.Duration // 音频时间轴起点
    EndTime   time.Duration // 音频时间轴终点
    Error     error         // 仅 EventError 时有效
}
```

### 事件传递管道

```
WebSocket ──→ readLoop() ──→ readCh(100) ──→ messageLoop() ──→ handleMessage()
                                                                     │
                                              eventsCh(100) ←── sendEvent()
                                                   │
                                              Events() <-chan ──→ 调用者 for-range
```

#### 第一级：WebSocket → readCh

**`transport/conn.go:148-195` — `readLoop()`**

独立 goroutine，从 WebSocket 读取原始字节，写入 `readCh`（缓冲 100）：

```go
_, message, err := c.ws.ReadMessage()
// ...
c.readCh <- message
```

#### 第二级：readCh → eventsCh

**`stt/session.go:128-146` — `messageLoop()`**

独立 goroutine，用 `select` 多路复用：

```go
for {
    select {
    case <-ctx.Done():     return
    case <-s.closeCh:      return
    case err := <-s.conn.ErrorChan():
        s.sendEvent(NewErrorEvent(err))
        return
    case data := <-s.conn.ReceiveChan():
        s.handleMessage(data)          // 解析 + 路由
    }
}
```

#### 第三级：消息路由

**`stt/session.go:149-172` — `handleMessage()`**

先解析 JSON 中的 `type` 字段，再按类型分发：

```go
switch msgType {
case protocol.MessageTypeTranscriptPartial:  s.handlePartial(data)
case protocol.MessageTypeTranscriptFinal:    s.handleFinal(data)
case protocol.MessageTypeSessionEnded:       s.sendEvent(NewSessionEndedEvent())
case protocol.MessageTypeProcessing:         s.sendEvent(NewProcessingEvent())
case protocol.MessageTypeSpeechStarted:      s.sendEvent(NewSpeechStartedEvent())
case protocol.MessageTypeError:              s.handleError(data)
}
```

#### 第四级：写入 eventsCh

**`stt/session.go:220-228` — `sendEvent()`**

非阻塞写入，缓冲区满时丢弃事件：

```go
select {
case s.eventsCh <- event:       // 成功写入
case <-s.closeCh:               // 会话已关闭
default:                        // 缓冲区满，丢弃
    slog.Error("Event buffer full, dropping event", ...)
}
```

### 调用者消费事件

```go
for event := range session.Events() {
    switch event.Type {
    case stt.EventTranscriptPartial:
        fmt.Printf("[partial] %s\n", event.Text)
    case stt.EventTranscriptFinal:
        fmt.Printf("[final] %s (%v-%v)\n", event.Text, event.StartTime, event.EndTime)
    case stt.EventError:
        fmt.Printf("[error] %v\n", event.Error)
    case stt.EventSessionEnded:
        break
    }
}
```

`Events()` 返回 `<-chan *RecognitionEvent`，channel 在 `messageLoop` 退出时 close，因此 `for-range` 会自然结束。

---

## 三、完整生命周期时序

```
 调用者                     Session                  Transport/WebSocket           Gateway
   │                          │                           │                          │
   │── NewClient() ──────────▶│                           │                          │
   │── CreateSession() ──────▶│                           │                          │
   │                          │── Connect() ─────────────▶│── TCP+TLS+WS 握手 ──────▶│
   │                          │                           │◀── 握手完成 ──────────────│
   │                          │                           │◀── session.ready ────────│
   │                          │◀─ waitReady() ────────────│                          │
   │                          │── sendConfig() ──────────▶│── session.config ───────▶│
   │                          │── go messageLoop() ──────▶│  (后台监听)               │
   │                          │                           │                          │
   │── Send(audio1) ─────────▶│── audio.append ─────────▶│─────────────────────────▶│
   │── Send(audio2) ─────────▶│── audio.append ─────────▶│─────────────────────────▶│
   │   ...                    │                           │                          │
   │                          │                           │◀── transcript.partial ───│
   │◀── Events() partial ────│◀─ handlePartial() ────────│                          │
   │                          │                           │◀── transcript.final ─────│
   │◀── Events() final ──────│◀─ handleFinal() ──────────│                          │
   │                          │                           │                          │
   │── EndInput() ───────────▶│── session.end ──────────▶│─────────────────────────▶│
   │                          │                           │                          │
   │                          │                           │◀── processing (心跳) ────│
   │                          │                           │◀── transcript.final ×N ──│
   │                          │                           │◀── session.ended ────────│
   │◀── Events() ended ──────│◀─ sendEvent() ────────────│                          │
   │◀── Events() channel关闭 │── close(eventsCh) ────────│                          │
   │── Close() ──────────────▶│── close(closeCh) ────────▶│── CloseMessage ─────────▶│
   │                          │                           │── ws.Close() ────────────│
```

---

## 四、Goroutine 模型

一个 STT 会话运行时共 3 个后台 goroutine：

| Goroutine | 位置 | 职责 | 生命周期 |
|-----------|------|------|---------|
| `readLoop` | `transport/conn.go:149` | 从 WebSocket 读消息 → `readCh` | Connect 时启动，连接关闭时退出 |
| `pingLoop` | `transport/conn.go:198` | 定时发送 Ping 保活 | Connect 时启动（可选），连接关闭时退出 |
| `messageLoop` | `stt/session.go:128` | 从 `readCh` 路由消息 → `eventsCh` | start() 时启动，ctx取消/closeCh关闭时退出 |

调用者通常额外起一个 goroutine 用于发送音频（见 example），形成 **发送-接收分离** 的全双工模式。

---

## 五、关键文件索引

| 文件 | 职责 |
|------|------|
| `stt/session.go` | 会话管理：音频发送、事件循环、TTFB、生命周期 |
| `stt/events.go` | 事件类型定义、RecognitionEvent 结构体、工厂函数 |
| `stt/client.go` | 客户端入口：CreateSession、RecognizeFile 高级API |
| `stt/options.go` | 流式配置选项 |
| `transport/conn.go` | WebSocket 连接：readLoop、pingLoop、SendJSON、重试 |
| `transport/message.go` | 消息编解码：ParseMessage、NewAudioAppend 等工厂函数 |
| `protocol/messages.go` | 协议定义：所有消息类型常量和结构体 |
| `examples/stt_stream/main.go` | 完整使用示例 |
