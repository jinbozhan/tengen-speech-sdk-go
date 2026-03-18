# Tengen Speech SDK for Go

Go 语言语音服务客户端 SDK，连接 Speech Arena Gateway，提供流式 TTS（文本转语音）和流式 STT（语音转文本）功能。

## 安装

```bash
go get github.com/jinbozhan/tengen-speech-sdk-go@v0.1.1
```

## TTS - 流式文本转语音

### 单次合成

```go
import (
    "io"
    "time"
    "github.com/jinbozhan/tengen-speech-sdk-go/tts"
)

config := &tts.Config{
    GatewayURL:     "ws://localhost:8080",
    Provider:       "qwen",
    APIKey:         "sk_xxx",
    VoiceID:        "loongstella",
    Speed:          1.0,
    SampleRate:     8000,
    AudioFormat:    "pcm",
    ConnectTimeout: 30 * time.Second,
    ReadTimeout:    120 * time.Second,
}

client, _ := tts.NewClient(config)
defer client.Close()

// 流式合成，stream 实现 io.Reader
stream, _ := client.SynthesizeStream(ctx, "你好，世界")
defer stream.Close()
io.Copy(outputFile, stream)
```

### 多轮合成（Session 复用）

创建一个 Session 后可多次合成，避免重复建连：

```go
session, _ := client.CreateSession(ctx, nil)
defer session.Close()

for _, text := range []string{"第一句", "第二句", "第三句"} {
    stream, _ := session.SynthesizeStream(ctx, text)
    buf := make([]byte, 4096)
    for {
        n, err := stream.Read(buf)
        if n > 0 {
            // 处理音频 buf[:n]
        }
        if err == io.EOF {
            break
        }
    }
}
```

## STT - 流式语音转文本

```go
import (
    "time"
    "github.com/jinbozhan/tengen-speech-sdk-go/stt"
)

config := &stt.Config{
    GatewayURL:     "ws://localhost:8080",
    Provider:       "azure",
    APIKey:         "sk_xxx",
    Language:       "zh-CN",
    SampleRate:     8000,
    AudioFormat:    "pcm",
    ConnectTimeout: 30 * time.Second,
    ReadTimeout:    120 * time.Second,
}

client, _ := stt.NewClient(config)
defer client.Close()

// 创建流式会话
session, _ := client.RecognizeStream(ctx, &stt.StreamOptions{
    Language:    "zh-CN",
    SampleRate:  8000,
    AudioFormat: "pcm",
})
defer session.Close()

// goroutine 发送音频
go func() {
    chunkSize := 8000 * 2 * 100 / 1000 // 100ms 音频块
    buf := make([]byte, chunkSize)
    for {
        n, err := audioReader.Read(buf)
        if err == io.EOF {
            break
        }
        session.Send(buf[:n])
    }
    session.Commit() // 标记发送完毕
}()

// 接收识别事件
for event := range session.Events() {
    switch event.Type {
    case stt.EventPartial:
        fmt.Printf("\r[Partial] %s", event.Text)
    case stt.EventFinal:
        fmt.Printf("\r[Final] [%.3fs-%.3fs] %s\n",
            event.StartTime.Seconds(), event.EndTime.Seconds(), event.Text)
    case stt.EventError:
        fmt.Printf("[Error] %v\n", event.Error)
    case stt.EventInputDone, stt.EventClosed:
        break
    }
}

fmt.Printf("TTFB: %dms\n", session.TTFB().Milliseconds())
```

## 命令行示例

```bash
./build.sh
```

### TTS

```bash
./bin/tts_stream_sdk "你好，世界"
./bin/tts_stream_sdk "第一句" "第二句" "第三句"
./bin/tts_stream_sdk -provider qwen -voice loongstella -apikey "sk_xxx" "测试"
./bin/tts_stream_sdk -speed 1.5 -sample-rate 16000 -output result.wav "快速播放"
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-gateway` | `ws://localhost:8080` | Gateway 地址 |
| `-provider` | `qwen` | TTS 提供商 |
| `-apikey` | - | API Key |
| `-voice` | `loongstella` | Voice ID |
| `-language` | - | 语言代码（文本归一化用） |
| `-speed` | `1.0` | 语速 (0.5-2.0) |
| `-pitch` | `1.0` | 音调 (0.5-2.0) |
| `-volume` | `1.0` | 音量 (0.0-1.0) |
| `-output` | `output.wav` | 输出文件路径 |
| `-sample-rate` | `8000` | 采样率 |

### STT

```bash
./bin/stt_stream_sdk audio.wav
./bin/stt_stream_sdk -provider qwen -language en-US recording.wav
./bin/stt_stream_sdk -apikey "sk_xxx" audio.wav
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-gateway` | `ws://localhost:8080` | Gateway 地址 |
| `-provider` | `azure` | STT 提供商 |
| `-apikey` | - | API Key |
| `-language` | `zh-CN` | 识别语言 |
| `-sample-rate` | `8000` | 采样率 |

## 支持的 Provider

| Provider | STT | TTS | 说明 |
|----------|-----|-----|------|
| `tengen` | Y | Y | 默认提供商 |
| `azure` | Y | Y | Microsoft Azure Speech Services |

## 前置条件

1. 运行 Speech Arena Gateway
2. 获取有效的 API Key（如需认证）
