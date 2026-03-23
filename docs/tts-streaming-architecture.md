# TTS 流式架构：文本输入与音频输出流

## 架构总览

三层架构，全双工 WebSocket 通信：

```
┌─────────────┐      ┌──────────────┐      ┌──────────────┐
│  protocol/  │      │  transport/  │      │    tts/      │
│  messages.go│ ──── │  conn.go     │ ──── │  session.go  │
│  (协议定义) │      │  message.go  │      │  stream.go   │
│             │      │  (WebSocket) │      │  (业务逻辑)  │
└─────────────┘      └──────────────┘      └──────────────┘
```

文本下行和音频上行在同一个 WebSocket 连接上同时进行。与 STT 方向相反：STT 是音频进、文本出；TTS 是文本进、音频出。

---

## 一、输出音频流

### 数据流路径

```
Gateway ──→ audio.delta JSON ──→ WebSocket Text Frame ──→ Base64 解码 ──→ AudioChunk ──→ io.Reader
```

### 核心实现

**`tts/session.go:197-231` — `handleAudioDelta(data []byte)`**

```go
func (s *Session) handleAudioDelta(data []byte) {
    // 记录首包接收时间（比应用层 stream.Read 更精确）
    s.mu.Lock()
    if s.firstChunkReceivedAt.IsZero() {
        s.firstChunkReceivedAt = time.Now()              // TTFB 终点
    }
    s.mu.Unlock()

    msg, err := transport.ParseMessage(data)             // 解析 JSON
    delta := msg.(*protocol.AudioDelta)

    audioData, err := base64.StdEncoding.DecodeString(delta.Audio)  // Base64 解码

    s.seqNum++

    s.streamMu.Lock()
    stream := s.currentStream
    s.streamMu.Unlock()

    if stream != nil {
        stream.pushData(audioData, s.seqNum)             // 推入 AudioStream
    }
}
```

**关键设计点：**

| 方面 | 实现 | 代码位置 |
|------|------|---------|
| 编码方式 | Base64（与 STT 对称，JSON 协议统一性） | `session.go:215` |
| TTFB 终点 | 首个 audio.delta 到达时立即记录 | `session.go:201-203` |
| 序列号 | 单调递增的 seqNum，用于排序验证 | `session.go:221` |
| 推送机制 | 通过 `pushData()` 写入 chunksCh channel | `session.go:229` |

### 协议消息格式

**`protocol/messages.go:104-108`**

```json
{
  "type": "audio.delta",
  "audio": "<base64 编码的音频数据>"
}
```

### 音频完成信号

**`tts/session.go:256-269` — `handleAudioDone()`**

收到 `audio.done` 表示当前轮次合成完毕：

```go
func (s *Session) handleAudioDone() {
    s.streamMu.Lock()
    stream := s.currentStream
    s.currentStream = nil
    s.synthesizing = false                              // 标记合成结束，允许下一轮
    s.streamMu.Unlock()

    if stream != nil {
        stream.pushDone()                               // 关闭 chunksCh，触发 io.EOF
    }
}
```

---

## 二、输入文本流

### 数据流路径

```
调用者文本 ──→ text.append JSON ──→ input.commit JSON ──→ WebSocket Text Frame ──→ Gateway
```

### 核心实现

**`tts/session.go:287-345` — `SynthesizeStream(ctx, text)`**

```go
func (s *Session) SynthesizeStream(ctx context.Context, text string) (*AudioStream, error) {
    // 1. 状态检查：closed? ready? synthesizing?
    // 2. 创建新的 AudioStream
    stream := newAudioStream()
    s.currentStream = stream
    s.synthesizing = true
    s.roundCount++

    // 3. 发送文本
    textMsg := transport.NewTextAppend(text)
    s.conn.SendJSON(textMsg)                            // text.append

    // 4. 触发合成
    s.commitSentAt = time.Now()                         // TTFB 起点
    commitMsg := transport.NewInputCommit()
    s.conn.SendJSON(commitMsg)                          // input.commit

    return stream, nil
}
```

**两条消息的分离设计：** `text.append` 负责传递文本，`input.commit` 负责触发合成。这种分离允许未来支持分段追加文本后一次性触发。

### 协议消息格式

**`protocol/messages.go:63-72`**

```json
{"type": "text.append", "text": "要合成的文本"}
{"type": "input.commit"}
```

---

## 三、AudioStream — io.Reader 适配层

### 结构

**`tts/stream.go:12-26`**

```go
type AudioStream struct {
    chunksCh       chan AudioChunk          // 音频块 channel（缓冲 100）
    buffer         *bytes.Buffer            // 内部缓冲区（处理部分读取）
    closed         bool
    totalSize      int64                    // 累计接收字节数
    err            error                    // 合成错误
    sessionCloser  io.Closer                // 引用 Session，stream.Close() 时关闭会话
    session        *Session                 // 访问 Session 时间信息
}
```

### 三种消费模式

#### 模式一：io.Reader（推荐）

**`tts/stream.go:45-78`**

```go
func (s *AudioStream) Read(p []byte) (n int, err error) {
    // 1. 先从缓冲区读取剩余数据
    if s.buffer.Len() > 0 {
        return s.buffer.Read(p)
    }
    // 2. 缓冲区空 → 阻塞等待 chunksCh
    chunk, ok := <-s.chunksCh
    if !ok || chunk.IsDone {
        return 0, io.EOF                                // channel 关闭 → EOF
    }
    // 3. 写入缓冲区再读取（处理 p 小于 chunk 的情况）
    s.buffer.Write(chunk.Data)
    return s.buffer.Read(p)
}
```

调用方式：`io.Copy(file, stream)` 或 `io.ReadAll(stream)`

#### 模式二：Channel（逐块处理）

**`tts/stream.go:80-83`**

```go
for chunk := range stream.Chunks() {
    if chunk.Error != nil { /* 处理错误 */ }
    if chunk.IsDone { break }
    // 处理 chunk.Data
}
```

#### 模式三：便捷方法

| 方法 | 位置 | 用途 |
|------|------|------|
| `ReadAll()` | `stream.go:161` | 全部加载到内存 |
| `SaveToFile(path)` | `stream.go:149` | 直接写入文件 |

### 数据推送（内部）

```
session.handleAudioDelta()
  └→ stream.pushData(audioData, seqNum)     // stream.go:103-108
       └→ stream.pushChunk(AudioChunk{...})  // stream.go:86-100
            └→ chunksCh <- chunk             // 缓冲 100，阻塞时受 closeCh 保护

session.handleAudioDone()
  └→ stream.pushDone()                      // stream.go:111-127
       └→ chunksCh <- AudioChunk{IsDone:true}
       └→ close(chunksCh)                   // 触发 Read() 返回 io.EOF

session.handleError()
  └→ stream.pushError(err)                  // stream.go:130-146
       └→ chunksCh <- AudioChunk{Error:err}
       └→ close(chunksCh)                   // 触发 Read() 返回 error
```

---

## 四、Client 层 API 设计

**`tts/client.go`**

### 三级 API

| API | 方法 | 适用场景 |
|-----|------|---------|
| 简化 API | `SynthesizeToFile()` `SynthesizeToBytes()` | 一次性合成，不关心流式细节 |
| 高级 API | `SynthesizeStream()` | 单轮流式合成，自动管理 Session 生命周期 |
| 专家 API | `CreateSession()` → 多次 `session.SynthesizeStream()` | 多轮合成，复用连接 |

### 简化 API 内部实现

**`tts/client.go:35-61` — `SynthesizeToFile()`**

```go
stream, _ := c.SynthesizeStream(ctx, text)   // 创建 Session + 合成
defer stream.Close()                          // Close 会级联关闭 Session
io.Copy(file, stream)                         // io.Reader 直接复制到文件
```

### 高级 API 的 Session 关联

**`tts/client.go:85-110` — `SynthesizeStream()`**

```go
session, _ := c.createSession(ctx, opts)      // 创建 Session
stream, _ := session.SynthesizeStream(ctx, text)
stream.setSessionCloser(session)              // stream.Close() 时自动关闭 session
return stream
```

关键：`setSessionCloser()` 让 `AudioStream.Close()` 级联关闭 Session，调用者无需手动管理。

### 多轮模式

**`tts/client.go:134-151` — `CreateSession()`**

```go
session, _ := client.CreateSession(ctx, nil)
defer session.Close()

stream1, _ := session.SynthesizeStream(ctx, "第一句话")
io.Copy(file, stream1)     // 读完第一轮

stream2, _ := session.SynthesizeStream(ctx, "第二句话")
io.Copy(file, stream2)     // 读完第二轮（复用同一连接）
```

---

## 五、完整生命周期时序

```
 调用者                     Session                  Transport/WebSocket           Gateway
   │                          │                           │                          │
   │── NewClient() ──────────▶│                           │                          │
   │── CreateSession() ──────▶│                           │                          │
   │                          │── Connect() ─────────────▶│── TCP+TLS+WS 握手 ──────▶│
   │                          │                           │◀── 握手完成 ─────────────│
   │                          │                           │◀── session.ready ────────│
   │                          │◀─ waitReady() ────────────│                          │
   │                          │── sendConfig() ──────────▶│── session.config ───────▶│
   │                          │── go messageLoop() ──────▶│  (后台监听)              │
   │                          │                           │◀── session.config_done ──│
   │                          │◀─ waitConfigDone() ───────│                          │
   │                          │                           │                          │
   │── SynthesizeStream()  ──▶│── text.append ───────────▶│─────────────────────────▶│
   │                          │── input.commit ──────────▶│─────────────────────────▶│
   │◀── AudioStream ──────────│                           │                          │
   │                          │                           │◀── audio.delta ──────────│
   │◀── Read()/Chunks() ─────│◀─ handleAudioDelta() ──────│                          │
   │                          │                           │◀── audio.delta ×N ───────│
   │◀── Read()/Chunks() ─────│◀─ handleAudioDelta() ──────│                          │
   │                          │                           │◀── audio.done ───────────│
   │◀── io.EOF ──────────────│◀─ handleAudioDone() ───────│                          │
   │                          │                           │                          │
   │  （多轮模式：可重复上述 SynthesizeStream 过程）                                 │
   │                          │                           │                          │
   │── Close() ──────────────▶│── session.end ───────────▶│─────────────────────────▶│
   │                          │                           │◀── Close Frame ──────────│
   │                          │── close(closeCh) ────────▶│── CloseMessage ─────────▶│
   │                          │                           │── ws.Close() ────────────│
```

---

## 六、多轮合成状态机

```
                   ┌──────────────────┐
         start()   │                  │  SynthesizeStream()
     ─────────────▶│  IDLE            │────────────────────▶┌──────────────────┐
                   │  synthesizing=F  │                     │  SYNTHESIZING    │
                   │  currentStream=∅ │◀────────────────────│  synthesizing=T  │
                   │                  │   audio.done /      │  currentStream=S │
                   └──────────────────┘   error             └──────────────────┘
                           │
                     Close()
                           │
                   ┌──────────────────┐
                   │  CLOSED          │
                   │  closed=T        │
                   └──────────────────┘
```

**关键约束（`session.go:300-303`）：** `synthesizing=true` 时调用 `SynthesizeStream()` 会返回错误。一个 Session 同一时刻只能有一轮合成在进行。

**轮次计数（`session.go:314`）：** 每次 `SynthesizeStream()` 递增 `roundCount`，`handleAudioDone()` 打印日志时报告当前轮次。

---

## 七、Goroutine 模型

一个 TTS 会话运行时共 3 个后台 goroutine：

| Goroutine | 位置 | 职责 | 生命周期 |
|-----------|------|------|---------|
| `readLoop` | `transport/conn.go:149` | 从 WebSocket 读消息 → `readCh` | Connect 时启动，连接关闭时退出 |
| `pingLoop` | `transport/conn.go:198` | 定时发送 Ping 保活（30s 间隔） | Connect 时启动（可选），连接关闭时退出 |
| `messageLoop` | `tts/session.go:144` | 从 `readCh` 路由消息 → `AudioStream` | start() 时启动，ctx取消/closeCh关闭时退出 |

与 STT 的区别：STT 的 `messageLoop` 输出到 `eventsCh`（事件 channel），TTS 的 `messageLoop` 输出到 `AudioStream.chunksCh`（音频 channel）。

### Channel 缓冲

| Channel | 容量 | 位置 | 用途 |
|---------|------|------|------|
| `readCh` | 100 | `transport/conn.go:67` | WebSocket 原始消息缓冲 |
| `chunksCh` | 100 | `tts/stream.go:39` | 音频块缓冲 |
| `errorCh` | 10 | `transport/conn.go:68` | 错误缓冲 |
| `configDoneCh` | 0 | `tts/session.go:57` | 配置完成信号（一次性） |
| `closeCh` | 0 | `tts/session.go:56` | 关闭信号（一次性） |

### 锁保护

| Mutex | 位置 | 保护范围 |
|-------|------|---------|
| `Session.mu` | `session.go:25` | ready, closed, 时间戳, configDone |
| `Session.streamMu` | `session.go:39` | currentStream, synthesizing, roundCount |
| `AudioStream.mu` | `stream.go:16` | buffer, closed, totalSize, err |
| `AudioStream.chunkChMu` | `stream.go:21` | chunkChClosed（防止 double close） |
| `Conn.mu` | `conn.go:48` | ws, connected |

---

## 八、时间度量

### 测量点

```
                ConnectWithRetry()
                    │
 connectStartAt ───▶├── TCP + TLS + WS 握手
                    │
 connectedAt ──────▶├── session.ready
                    ├── session.config →
                    ├── go messageLoop()
 configDoneAt ─────▶├── session.config_done
                    │
                    ├── text.append →
 commitSentAt ─────▶├── input.commit →
                    │
                    │   ... Gateway 合成中 ...
                    │
 firstChunkReceivedAt ▶├── audio.delta (第一个)
                    ├── audio.delta ×N
                    ├── audio.done
```

### 指标计算

| 指标 | 计算方式 | 代码位置 |
|------|---------|---------|
| 建连耗时 | `connectedAt - connectStartAt` | `transport/conn.go:369-374` |
| TTFB | `firstChunkReceivedAt - commitSentAt` | `tts/session.go:461-468` |
| 配置耗时 | `configDoneAt - connectedAt` | 可从两个时间戳相减得到 |

**TTFB 的精确性（`session.go:199-203`）：** 首包时间在 `handleAudioDelta()` 内立即记录，这是 WebSocket 消息到达的时刻，比应用层 `stream.Read()` 更精确（排除了 channel 传递和 Read 调度延迟）。

**每轮重置（`session.go:305-308`）：** 多轮模式下，`firstChunkReceivedAt` 在每次 `SynthesizeStream()` 时重置为零值，确保 TTFB 是当前轮的测量。

### 时间信息访问

`AudioStream` 委托 `Session` 暴露时间信息（`stream.go:219-259`），调用者无需直接操作 Session：

```go
stream.TTFB()                  // 毫秒
stream.ConnectDuration()       // time.Duration
stream.CommitSentAt()          // time.Time
stream.FirstChunkReceivedAt()  // time.Time
```

---

## 九、错误处理

### 两级错误体系

| 层级 | 位置 | 错误类型 |
|------|------|---------|
| 传输层 | `transport/errors.go` | `ErrNotConnected` `ErrConnectionClosed` |
| 客户端层 | `errors.go` | `ClientError{Op, Provider, Code, Message, Err}` |

### 错误传播路径

```
Gateway error 消息 ──→ handleError() ──→ stream.pushError() ──→ chunksCh
                                                                    │
WebSocket 异常 ──→ errorCh ──→ handleStreamError() ──→ stream.pushError()
                                                                    │
                                                          Read() 返回 error
                                                          或 Chunks() 返回 {Error: err}
```

### 服务端错误码

**`protocol/messages.go:128-138`**

| 错误码 | 含义 |
|--------|------|
| `PROVIDER_ERROR` | 提供商服务异常 |
| `VOICE_NOT_FOUND` | 音色不存在 |
| `AUTH_ERROR` | 认证失败 |
| `RATE_LIMIT_ERROR` | 限流 |
| `CONFIG_ERROR` | 配置错误 |
| `SERVICE_UNAVAILABLE` | 服务不可用 |

---

## 十、配置

### Config（客户端级别）

**`tts/options.go:7-28`**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| GatewayURL | string | `ws://localhost:8080` | Gateway WebSocket URL |
| Provider | string | `tengen` | 提供商: tengen, azure, qwen, voxnexus |
| APIKey | string | — | API Key（通过 URL 参数传递） |
| VoiceID | string | — | 语音 ID |
| Language | string | — | 语言代码（用于文本归一化） |
| Speed | float64 | 1.0 | 语速 0.5-2.0 |
| Pitch | float64 | 1.0 | 音调 -10 to 10 |
| Volume | float64 | 1.0 | 音量 0.0-1.0 |
| SampleRate | int | 8000 | 采样率 Hz |
| AudioFormat | string | `pcm` | 音频格式: pcm, wav, mp3 |
| ConnectTimeout | Duration | 10s | 连接超时 |
| ReadTimeout | Duration | 60s | 读超时 |
| WriteTimeout | Duration | 10s | 写超时 |
| ReconnectBackoff | Duration | 1s | 重连退避基数 |
| MaxReconnects | int | 3 | 最大重连次数 |

### SynthesisOptions（每轮级别）

**`tts/options.go:129-138`**

可在每轮合成时覆盖：VoiceID, Language, Speed, Pitch, Volume, SampleRate, AudioFormat。

### Builder 模式

```go
config := tts.DefaultConfig().
    WithProvider("qwen").
    WithVoice("loongstella").
    WithSpeed(1.2).
    WithSampleRate(24000).
    WithAPIKey("sk_xxx")
```

---

## 十一、关键文件索引

| 文件 | 职责 |
|------|------|
| `tts/client.go` | 客户端入口：SynthesizeStream、CreateSession、便捷 API |
| `tts/session.go` | 会话管理：消息路由、多轮合成、TTFB、生命周期 |
| `tts/stream.go` | 音频流：io.Reader 适配、channel 消费、时间信息委托 |
| `tts/options.go` | 配置：Config、SynthesisOptions、Builder、验证 |
| `transport/conn.go` | WebSocket 连接：readLoop、pingLoop、SendJSON、重试 |
| `transport/message.go` | 消息编解码：ParseMessage、NewTextAppend 等工厂函数 |
| `protocol/messages.go` | 协议定义：所有消息类型常量和结构体 |
| `errors.go` | 错误处理：ClientError、分类判断函数 |
| `examples/tts_stream/main.go` | 完整使用示例（多轮合成 + WAV 输出） |
