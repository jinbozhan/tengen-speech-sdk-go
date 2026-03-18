# TTS Stream Example 增加 Pitch 设定

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 `examples-sdk/tts_stream/main.go` 中增加 `pitch` 命令行参数，默认值 1.0，并传递到 `tts.Config`。

**Architecture:** SDK 的 `tts.Config` 已支持 `Pitch` 字段（float64），只需在 example 中补上 flag 定义和 Config 赋值即可。对齐现有 `speed`/`volume` 的模式。

**Tech Stack:** Go, flag 标准库, tengen-speech-sdk-go/tts

---

### Task 1: 添加 pitch flag 和 Config 赋值

**Files:**
- Modify: `examples-sdk/tts_stream/main.go:26-38` (var block)
- Modify: `examples-sdk/tts_stream/main.go:40-52` (init func)
- Modify: `examples-sdk/tts_stream/main.go:83-96` (Config struct)
- Modify: `examples-sdk/tts_stream/main.go:104` (日志输出)

**Step 1: 在 var block 增加 pitch 变量**

在 `volume float64` 之后添加：

```go
var (
	gatewayURL    string
	provider      string
	apiKey        string
	voiceID       string
	language      string
	speed         float64
	pitch         float64
	volume        float64
	output        string
	sampleRate    int
	channels      int
	bitsPerSample int
)
```

**Step 2: 在 init() 增加 pitch flag 定义**

在 `speed` flag 之后添加：

```go
flag.Float64Var(&pitch, "pitch", 1.0, "Pitch (-10 to 10, default 1.0)")
```

**Step 3: 在 Config 中赋值 Pitch**

```go
config := &tts.Config{
	GatewayURL:     gatewayURL,
	Provider:       provider,
	APIKey:         apiKey,
	VoiceID:        voiceID,
	Language:       language,
	Speed:          speed,
	Pitch:          pitch,
	Volume:         volume,
	SampleRate:     sampleRate,
	AudioFormat:    "pcm",
	ConnectTimeout: 30 * time.Second,
	ReadTimeout:    120 * time.Second,
	WriteTimeout:   10 * time.Second,
}
```

**Step 4: 更新日志输出，包含 pitch**

```go
fmt.Printf("TTS 配置: provider=%s, voice=%s, speed=%.1f, pitch=%.1f, volume=%.1f\n\n", provider, voiceID, speed, pitch, volume)
```

**Step 5: 编译验证**

Run: `cd /home/mark/code/tengen-speech-sdk-go && go build ./examples-sdk/tts_stream/`
Expected: 编译成功，无错误

**Step 6: 验证 help 输出包含 pitch**

Run: `./tts_stream -h 2>&1 | grep pitch`
Expected: 输出包含 `-pitch float  Pitch (-10 to 10, default 1.0)`

**Step 7: Commit**

```bash
git add examples-sdk/tts_stream/main.go
git commit -m "feat(examples): add pitch flag to tts_stream example

Default pitch is 1.0, matching SDK default.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
