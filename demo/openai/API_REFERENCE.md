# OpenAI 兼容语音 API 参考

## 认证

所有请求需要在 Header 中携带 API Key：

```
Authorization: Bearer <your-api-key>
```

## TTS: `POST /v1/audio/speech`

将文本合成为语音音频。

### 请求

Content-Type: `application/json`

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `model` | string | 是 | - | 模型 ID：`tts-1` / `tts-1-hd` |
| `input` | string | 是 | - | 要合成的文本，最大 4096 字符 |
| `voice` | string | 是 | - | 音色名称，对应 voice library 的 display_id |
| `response_format` | string | 否 | `wav` | 输出格式：`wav`、`pcm`、`opus` |
| `speed` | number | 否 | `1.0` | 语速，0.25 ~ 4.0 |

### 响应

返回二进制音频流。

| response_format | Content-Type | 说明 |
|-----------------|-------------|------|
| `wav` | `audio/wav` | 24kHz 16-bit mono WAV (RIFF 头 + PCM) |
| `pcm` | `audio/pcm` | 24kHz 16-bit mono 原始 PCM (little-endian) |
| `opus` | `audio/opus` | Opus 编码，64kbps VBR |

### 示例

```bash
curl -X POST http://localhost:7861/v1/audio/speech \
  -H "Authorization: Bearer sk-your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"model": "tts-1", "input": "Hello world", "voice": "en-NG-AbeoNeural", "response_format": "wav"}' \
  --output speech.wav
```

```python
from openai import OpenAI

client = OpenAI(api_key="sk-xxx", base_url="http://localhost:7861/v1")

response = client.audio.speech.create(
    model="tts-1",
    voice="en-NG-AbeoNeural",
    input="Hello world",
    response_format="wav",
)
response.stream_to_file("output.wav")
```

---

## STT: `POST /v1/audio/transcriptions`

将音频转录为文本。

### 请求

Content-Type: `multipart/form-data`

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `file` | file | 是 | - | 音频文件，≤25MB，支持 WAV 格式 |
| `model` | string | 是 | - | 模型 ID：`whisper-1` |
| `language` | string | 否 | - | BCP-47 语言码（如 `en-NG`、`zh-CN`、`en-US`、`fr-FR`） |
| `response_format` | string | 否 | `json` | 输出格式：`json`、`text`、`verbose_json` |

### 响应

**json** (默认):
```json
{"text": "识别出的文本内容"}
```

**text**:
```
识别出的文本内容
```

**verbose_json**:
```json
{
  "task": "transcribe",
  "language": "zh",
  "duration": 10.5,
  "text": "识别出的文本内容"
}
```

### 示例

```bash
curl -X POST http://localhost:7861/v1/audio/transcriptions \
  -H "Authorization: Bearer sk-your-api-key" \
  -F "file=@audio.wav" \
  -F "model=whisper-1" \
  -F "language=en-NG"
```

```python
from openai import OpenAI

client = OpenAI(api_key="sk-xxx", base_url="http://localhost:7861/v1")

with open("audio.wav", "rb") as f:
    transcript = client.audio.transcriptions.create(
        model="whisper-1",
        file=f,
        language="en-NG",
    )
print(transcript.text)
```

---

## 错误格式

所有错误返回 JSON：

```json
{
  "error": {
    "message": "具体错误信息",
    "type": "invalid_request_error",
    "code": "invalid_api_key"
  }
}
```

| HTTP 状态码 | type | 场景 |
|------------|------|------|
| 400 | `invalid_request_error` | 参数缺失/无效、不支持的格式 |
| 401 | `authentication_error` | API Key 缺失/无效/过期 |
| 404 | `invalid_request_error` | 音色不存在 |
| 405 | `invalid_request_error` | HTTP 方法不允许 |
| 413 | `invalid_request_error` | 文件过大 |
| 500 | `server_error` | 合成/识别内部错误 |

---

## Model 映射

| OpenAI Model | 实际 Provider | 说明 |
|-------------|--------------|------|
| `tts-1` | Azure | 标准 TTS |
| `tts-1-hd` | Azure | 高质量 TTS（同 provider） |
| `whisper-1` | Azure | 语音识别 |

---

## 音频规格

### TTS 输出

所有输出格式统一 24kHz, 16-bit, mono，与 OpenAI 官方一致。

| 格式 | 采样率 | 位深 | 声道 | 说明 |
|------|--------|------|------|------|
| `wav` | 24kHz | 16-bit | mono | RIFF/WAVE 头 + PCM 数据 |
| `pcm` | 24kHz | 16-bit signed LE | mono | 原始 PCM，无文件头 |

### STT 输入

| 项目 | 规格 |
|------|------|
| 支持格式 | WAV |
| 采样率 | 任意（Azure 内部处理） |
| 最大文件 | 25MB |
| 推荐 | 16kHz mono WAV |

---

## 客户端接入

使用 OpenAI 官方 SDK 时，只需修改 `base_url` 即可切换到本服务。

### 环境变量方式（推荐）

```bash
export OPENAI_API_KEY="your-api-key"
export OPENAI_BASE_URL="http://localhost:7861/v1"
```

设置后，Python / Node.js SDK 会自动读取，无需改代码。

### Python SDK

```python
from openai import OpenAI

client = OpenAI(
    api_key="your-api-key",
    base_url="http://localhost:7861/v1",
)
```

### Node.js SDK

```typescript
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: 'your-api-key',
  baseURL: 'http://localhost:7861/v1',
});
```

---

## 与 OpenAI 官方 API 的差异

| 特性 | OpenAI 官方 | 本实现 | 说明 |
|------|-----------|--------|------|
| TTS 默认格式 | mp3 | wav | 暂不支持 mp3，**建议显式指定 `response_format`** |
| TTS 支持格式 | mp3/opus/aac/flac/wav/pcm | wav/pcm/opus | 支持 opus 编码 |
| TTS 采样率 | 24kHz | 24kHz | 一致 |
| STT 输入格式 | flac/mp3/mp4/wav/... | wav | Azure RecognizeOnce 原生支持 WAV |
| STT verbose_json | 含 words/segments 数组 | 仅 duration/text | Azure 不返回 word-level timestamps |
| voice 参数 | alloy/nova/shimmer 等 | voice library display_id | 使用 Azure voice name，如 en-NG-AbeoNeural |
| 流式 TTS | 支持 | 暂不支持 | 使用 WebSocket `/ws/tts` 替代 |
