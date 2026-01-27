# Tengen Speech SDK for Go

Go 语言实现的语音服务客户端 SDK，用于连接 Speech Arena Gateway，提供 TTS（文本转语音）和 STT（语音转文本）功能。

## 特性

- **统一接口**：通过 WebSocket 协议连接 Gateway，支持多个后端 Provider
- **API Key 认证**：支持 Gateway 认证机制
- **简化 + 高级 API**：提供一行代码完成任务的简化 API，也支持流式控制的高级 API
- **流式处理**：支持音频流式发送和接收，适用于实时场景
- **自动重连**：内置指数退避重连机制

## 安装

```bash
go get github.com/jinbozhan/tengen-speech-sdk-go
```

## 目录结构

```
tengen-speech-sdk-go/
├── bin/                   # 编译输出目录
├── tts/                   # TTS 客户端
│   ├── client.go          # 主接口
│   ├── options.go         # 配置选项
│   ├── session.go         # 会话管理
│   └── stream.go          # 音频流处理
├── stt/                   # STT 客户端
│   ├── client.go          # 主接口
│   ├── options.go         # 配置选项
│   ├── session.go         # 会话管理
│   └── events.go          # 识别事件
├── transport/             # 传输层
│   └── conn.go            # WebSocket 连接管理
├── protocol/              # 协议定义
│   └── messages.go        # 消息结构体
├── audio/                 # 音频工具
│   └── wav.go             # WAV 文件读写
├── examples-sdk/          # SDK 简化 API 示例
│   ├── tts_stream/        # TTS 示例
│   └── stt_stream/        # STT 示例
├── examples-ws/           # WebSocket 完整流程示例
│   ├── tts_stream/        # TTS 示例
│   └── stt_stream/        # STT 示例
├── errors.go              # 统一错误类型
└── build.sh               # 构建脚本
```

## 快速开始

### 构建

```bash
cd tengen-speech-sdk-go
./build.sh
```

编译产物输出到 `bin/` 目录：
- `bin/tts_stream_sdk` - TTS SDK 示例
- `bin/stt_stream_sdk` - STT SDK 示例
- `bin/tts_stream_ws` - TTS WebSocket 完整流程示例
- `bin/stt_stream_ws` - STT WebSocket 完整流程示例

### TTS 示例

```bash
# 基本用法
./bin/tts_stream_sdk "你好，世界"

# 带认证
./bin/tts_stream_sdk -apikey "sk_xxx" "你好，世界"

# 指定 Provider 和语音（voice_id 通过 URL 参数传递，Gateway 精准路由）
./bin/tts_stream_sdk -apikey "sk_xxx" -provider tengen -voice en-NG-OkunNeutral "测试"

# 查看所有选项
./bin/tts_stream_sdk -h
```

### STT 示例

```bash
# 基本用法
./bin/stt_stream_sdk audio.wav

# 带认证
./bin/stt_stream_sdk -apikey "sk_xxx" audio.wav

# 流式模式（显示部分识别结果）
./bin/stt_stream_sdk -apikey "sk_xxx" -stream audio.wav

# 查看所有选项
./bin/stt_stream_sdk -h
```

## 认证

Gateway 支持 API Key 认证。通过 `-apikey` 参数传递：

```bash
./bin/tts_stream_sdk -apikey "sk_free_xxx" "测试"
./bin/stt_stream_sdk -apikey "sk_free_xxx" audio.wav
```

在代码中配置：

```go
config := &tts.Config{
    GatewayURL: "ws://localhost:8080",
    Provider:   "tengen",                  // 品牌抽象层，根据 voice_id 自动路由
    APIKey:     "sk_free_xxx",             // API Key 认证
    VoiceID:    "en-NG-OkunNeutral",       // 通过 URL 参数传递，实现精准路由
}
```

### 用户等级和限制

| 等级 | 并发连接 | 每日请求 |
|------|----------|----------|
| Free | 1 | 5 |
| Lite | 20 | 400 |
| Plus | 40 | 1000 |
| Pro | 100 | 5000 |
| Max | 400 | 无限制 |

## SDK 使用

### TTS 客户端

```go
import "github.com/jinbozhan/tengen-speech-sdk-go/tts"

// 创建客户端
config := &tts.Config{
    GatewayURL: "ws://localhost:8080",
    Provider:   "tengen",              // 品牌抽象层
    APIKey:     "sk_xxx",
    VoiceID:    "en-NG-OkunNeutral",   // 通过 URL 参数传递，Gateway 精准路由
    Speed:      1.0,
}
client, err := tts.NewClient(config)
defer client.Close()

// 简化 API - 合成到文件
err = client.SynthesizeToFile(ctx, "你好，世界", "output.wav")

// 简化 API - 合成到内存
audio, err := client.SynthesizeToBytes(ctx, "你好，世界")

// 高级 API - 流式合成
stream, err := client.SynthesizeStream(ctx, "你好，世界")
defer stream.Close()
io.Copy(file, stream)
```

### STT 客户端

```go
import "github.com/jinbozhan/tengen-speech-sdk-go/stt"

// 创建客户端
config := &stt.Config{
    GatewayURL: "ws://localhost:8080",
    Provider:   "azure",
    APIKey:     "sk_xxx",
    Language:   "zh-CN",
    SampleRate: 16000,
    EnableVAD:  true,
}
client, err := stt.NewClient(config)
defer client.Close()

// 简化 API - 识别文件
result, err := client.RecognizeFile(ctx, "audio.wav")
fmt.Println(result.Text)

// 高级 API - 流式识别
session, err := client.RecognizeStream(ctx, opts)
defer session.Close()

// 发送音频
session.Send(audioChunk)
session.Commit()

// 接收识别事件
for event := range session.Events() {
    switch event.Type {
    case stt.EventPartial:
        fmt.Printf("部分: %s\n", event.Text)
    case stt.EventFinal:
        fmt.Printf("最终: %s\n", event.Text)
    }
}
```

## 配置说明

### TTS 配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `GatewayURL` | string | `ws://localhost:8080` | Gateway 地址 |
| `Provider` | string | `tengen` | 服务提供商（tengen 为品牌抽象层） |
| `APIKey` | string | - | API Key 认证 |
| `VoiceID` | string | - | 语音 ID（**通过 URL 参数传递，实现精准路由**） |
| `Speed` | float64 | `1.0` | 语速 (0.5-2.0) |
| `Pitch` | float64 | `0` | 音调 (-10 to 10) |
| `Volume` | float64 | `100` | 音量 (0-100) |

> **注意**：`VoiceID` 会自动添加到 WebSocket URL 参数中（如 `?voice_id=xxx`），Gateway 在连接阶段即可进行精准路由和 Provider 预热，无需等到 `session.config` 阶段。

### STT 配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `GatewayURL` | string | `ws://localhost:8080` | Gateway 地址 |
| `Provider` | string | `tengen` | 服务提供商 |
| `APIKey` | string | - | API Key 认证 |
| `Language` | string | `zh-CN` | 识别语言 |
| `SampleRate` | int | `16000` | 采样率 |
| `AudioFormat` | string | `pcm` | 音频格式 |
| `EnableVAD` | bool | `true` | 启用语音活动检测 |

## WebSocket 协议

### TTS 流程

```
1. 建立连接 → ws://host/ws/tts?provider=tengen&voice_id=xxx&api_key=xxx
   ↑ voice_id 在 URL 中传递，Gateway 精准路由并预热正确的 Provider
2. 接收 session.ready（Provider 已预热就绪）
3. 发送 session.config (语速等参数，voice_id 已在 URL 中可省略)
4. 发送 text.append (合成文本)
5. 发送 input.commit (触发合成)
6. 循环接收 audio.delta (音频数据块)
7. 接收 audio.done (合成完成)
8. 发送 session.end (关闭会话)
```

> **精准路由优化**：将 `voice_id` 放在 URL 参数中，Gateway 在 WebSocket 连接阶段就能根据 voice_id 路由到正确的底层 Provider 并预热，避免在 `session.config` 阶段重新获取 Provider，从而降低 TTFB。

### STT 流程

```
1. 建立连接 → ws://host/ws/stt?provider=xxx&api_key=xxx
2. 接收 session.ready
3. 发送 session.config (语言、采样率、VAD)
4. 循环发送 audio.append (音频分块)
5. 实时接收 transcript.partial (部分结果)
6. 发送 input.commit (标记结束)
7. 接收 transcript.final (最终结果)
8. 发送 session.end (关闭会话)
```

## 支持的 Provider

| Provider | STT | TTS | 说明 |
|----------|-----|-----|------|
| `tengen` | ✅ | ✅ | 默认提供商 |
| `azure` | ✅ | ✅ | Microsoft Azure Speech Services |
| `qwen_realtime` | ✅ | ✅ | 阿里云通义千问实时语音 |
| `voxnexus` | ✅ | ✅ | VoxNexus 语音服务 |

## 错误处理

```go
// 预定义错误
client.ErrSessionNotReady      // 会话未就绪
client.ErrSessionClosed        // 会话已关闭
client.ErrProviderNotSupported // 不支持的提供商
client.ErrInvalidConfig        // 无效配置
client.ErrTimeout              // 操作超时

// 错误类型判断
if client.IsConnectionError(err) {
    // 连接错误，可能需要重试
}
if client.IsRetryable(err) {
    // 可重试的错误
}
```

## 前置条件

1. 启动 Speech Arena Gateway
2. 获取有效的 API Key

```bash
# 启动 Gateway
cd speech-arena/speech
./gateway-server
```

## 相关文档

- [WebSocket 协议规范](../docs/websocket_protocol_spec.md)
- [Gateway API 文档](../docs/websocket_example_design.md)
