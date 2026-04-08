# OpenAI 兼容 API Demo

通过 OpenAI 官方 SDK 和 curl 调用 speech-arena 的 TTS/STT 接口。

API 端点参考、参数说明、错误码、音频规格详见 [API_REFERENCE.md](API_REFERENCE.md)。

## 前置条件

- Python 3.8+
- `pip install openai`
- curl
- speech-arena 网关已启动（默认 `http://localhost:7861`）

## 环境变量

```bash
export OPENAI_API_KEY="your-api-key"                    # 必填
export OPENAI_BASE_URL="http://localhost:7861/v1"       # 可选，默认 localhost:7861
```

## Python SDK Demo

### TTS - 文本合成语音

```bash
# 默认参数（en-NG-AbeoNeural 音色，英文测试句）
python tts_demo.py

# 自定义参数
python tts_demo.py --text "Hello world" --voice en-NG-EzinneNeural --output hello.wav

# PCM 格式输出
python tts_demo.py --text "Testing" --format pcm --output test.pcm

# 调整语速
python tts_demo.py --text "Fast playback" --speed 1.5
```

参数:
- `--text` 要合成的文本（默认: 英文测试句）
- `--voice` 音色名称，Azure voice name（默认: en-NG-AbeoNeural）
- `--model` 模型名称（默认: tts-1）
- `--format` 输出格式: wav, pcm, opus（默认: wav）
- `--speed` 语速 0.25-4.0（默认: 1.0）
- `--output` 输出文件路径（默认: output.wav）

### STT - 语音转文本

```bash
# 基本使用
python stt_demo.py --file audio.wav

# 指定语言
python stt_demo.py --file audio.wav --language zh-CN

# verbose 格式（含时长信息）
python stt_demo.py --file audio.wav --format verbose_json
```

参数:
- `--file` 音频文件路径（必填，WAV 格式）
- `--model` 模型名称（默认: whisper-1）
- `--language` 语言代码，如 en-NG, zh-CN, en-US（默认: en-NG）
- `--format` 响应格式: json, text, verbose_json（默认: json）

## 示例音频

目录下附带 3 个测试音频（8kHz mono 16-bit PCM WAV，电话录音）：

| 文件 | 时长 | 大小 |
|------|------|------|
| `sample_short.wav` | 35s | 557KB |
| `sample_medium.wav` | 56s | 879KB |
| `sample_long.wav` | 72s | 1.1MB |

快速测试：

```bash
python stt_demo.py --file sample_short.wav
bash stt_demo.sh sample_short.wav
```

## curl Demo

### TTS

```bash
bash tts_demo.sh
```

演示 WAV 和 PCM 两种格式输出。生成 `tts_output.wav` 和 `tts_output.pcm`。

### STT

```bash
bash stt_demo.sh audio.wav
```

演示 json、text、verbose_json 三种响应格式。

## Voice 和语言范围

voice 参数使用 Azure Speech Service 的 voice name，格式为 `{locale}-{Name}Neural`。

### 常用 voice（en-NG 尼日利亚英语）

| Voice | 性别 | 说明 |
|-------|------|------|
| `en-NG-AbeoNeural` | 男 | 默认 voice |
| `en-NG-EzinneNeural` | 女 | |

### 支持的语言（部分）

| 语言代码 | 语言 | voice 示例 |
|----------|------|-----------|
| `en-NG` | English (Nigeria) | en-NG-AbeoNeural, en-NG-EzinneNeural |
| `en-US` | English (US) | en-US-JennyNeural, en-US-GuyNeural |
| `en-GB` | English (UK) | en-GB-SoniaNeural, en-GB-RyanNeural |
| `zh-CN` | 中文 (普通话) | zh-CN-XiaoxiaoNeural, zh-CN-YunxiNeural |
| `fr-FR` | Fran&ccedil;ais | fr-FR-DeniseNeural, fr-FR-HenriNeural |
| `sw-KE` | Kiswahili (Kenya) | sw-KE-RafikiNeural, sw-KE-ZuriNeural |
| `sw-TZ` | Kiswahili (Tanzania) | sw-TZ-DaudiNeural, sw-TZ-RehemaNeural |

完整列表见 [Azure Speech Service 语言支持](https://learn.microsoft.com/en-us/azure/ai-services/speech-service/language-support?tabs=tts)。

> **注意**: voice 必须与语言匹配。使用 `en-NG-AbeoNeural` 时应传入英文文本，使用 `zh-CN-XiaoxiaoNeural` 时应传入中文文本。

## 注意事项

- `response_format` 支持 `wav`、`pcm`、`opus`，暂不支持 OpenAI 默认的 mp3
- `voice` 参数使用 Azure voice name，不是 OpenAI 的 alloy/nova 等
- WAV/PCM 音频规格: 24kHz, 16-bit, mono; Opus: 64kbps VBR
