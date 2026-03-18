# 统一日志改造：全面引入 log/slog

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将全代码库的日志从 `log.Printf` / `log.Fatalf` 统一迁移到 `log/slog`，输出格式包含 datetime、level、file:line、message。

**Architecture:** 新建 `logging` 包提供自定义 slog Handler，输出人类友好格式。SDK 库代码（stt/tts/transport）通过 `slog` 包级函数输出结构化日志。示例和 cmd 工具在 main 入口调用 `logging.Setup()` 初始化。

**Tech Stack:** Go 1.22 标准库 `log/slog`，零外部依赖。

---

## File Structure

| 操作 | 文件路径 | 职责 |
|------|---------|------|
| **Create** | `logging/handler.go` | 自定义 slog Handler + Setup 便捷函数 |
| Modify | `transport/conn.go` | 7 处 `log.Printf` → `slog` |
| Modify | `stt/client.go` | 3 处 `log.Printf` → `slog` |
| Modify | `stt/session.go` | 9 处 `log.Printf` → `slog` |
| Modify | `tts/client.go` | 1 处 `log.Printf` → `slog` |
| Modify | `tts/session.go` | 12 处 `log.Printf` → `slog` |
| Modify | `examples-sdk/stt_stream/main.go` | `log.Fatalf` → `slog.Error` + exit |
| Modify | `examples-sdk/tts_stream/main.go` | 同上 |
| ~~Modify~~ | ~~`examples-ws/stt_stream/main.go`~~ | _(已删除)_ |
| ~~Modify~~ | ~~`examples-ws/tts_stream/main.go`~~ | _(已删除)_ |
| Modify | `cmd/tts_benchmark/main.go` | `log.Printf/Fatalf` → `slog` |
| Modify | `cmd/tts_benchmark/benchmark.go` | `log.Printf` → `slog` |
| Modify | `cmd/tts_detailed_timing/main.go` | `log.Fatal` → `slog.Error` + exit |
| Modify | `cmd/test_vad_clip_asr/main.go` | `log.Fatalf/Printf` → `slog` |

---

### Task 1: Create `logging/handler.go`

**Files:**
- Create: `logging/handler.go`

- [ ] **Step 1: Create the logging package**

```go
// Package logging 提供统一的日志 Handler
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// Handler 自定义 slog Handler，输出格式：
// 2024-01-01 15:04:05.000 INFO  transport/conn.go:116 WebSocket connected url=ws://... duration=45ms
type Handler struct {
	level slog.Leveler
	w     io.Writer
	mu    *sync.Mutex
	attrs []slog.Attr
}

// NewHandler 创建自定义 Handler
func NewHandler(w io.Writer, level slog.Leveler) *Handler {
	return &Handler{
		level: level,
		w:     w,
		mu:    &sync.Mutex{},
	}
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	// datetime
	buf := make([]byte, 0, 256)
	buf = append(buf, r.Time.Format("2006-01-02 15:04:05.000")...)

	// level (固定宽度)
	buf = append(buf, ' ')
	switch {
	case r.Level >= slog.LevelError:
		buf = append(buf, "ERROR"...)
	case r.Level >= slog.LevelWarn:
		buf = append(buf, "WARN "...)
	case r.Level >= slog.LevelInfo:
		buf = append(buf, "INFO "...)
	default:
		buf = append(buf, "DEBUG"...)
	}

	// source (package/file:line)
	buf = append(buf, ' ')
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		// 保留 "包名/文件名:行号"，例如 "transport/conn.go:116"
		dir, file := filepath.Split(f.File)
		parent := filepath.Base(dir)
		buf = append(buf, fmt.Sprintf("%s/%s:%d", parent, file, f.Line)...)
	}

	// message
	buf = append(buf, ' ')
	buf = append(buf, r.Message...)

	// pre-set attrs
	for _, a := range h.attrs {
		buf = append(buf, ' ')
		buf = append(buf, fmt.Sprintf("%s=%v", a.Key, a.Value)...)
	}

	// record attrs
	r.Attrs(func(a slog.Attr) bool {
		buf = append(buf, ' ')
		buf = append(buf, fmt.Sprintf("%s=%v", a.Key, a.Value)...)
		return true
	})

	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &Handler{
		level: h.level,
		w:     h.w,
		mu:    h.mu,
		attrs: newAttrs,
	}
}

func (h *Handler) WithGroup(_ string) slog.Handler {
	// 不支持 group，直接返回自身
	return h
}

// Setup 初始化全局 slog 默认 logger
func Setup(level slog.Level) {
	handler := NewHandler(os.Stderr, level)
	slog.SetDefault(slog.New(handler))
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./logging/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add logging/handler.go
git commit -m "feat: add logging package with custom slog handler"
```

---

### Task 2: Migrate `transport/conn.go`

**Files:**
- Modify: `transport/conn.go`

- [ ] **Step 1: Replace import and all log.Printf calls**

Replace `"log"` import with `"log/slog"`.

Replace each `log.Printf` with the appropriate `slog` call:

| Line | Old | New |
|------|-----|-----|
| 116 | `log.Printf("[client.transport] WebSocket connected: url=%s, connect_duration=%dms", ...)` | `slog.Info("WebSocket connected", "component", "transport", "url", c.config.URL, "connect_duration_ms", c.connectedAt.Sub(c.connectStartAt).Milliseconds())` |
| 128 | `log.Printf("[client.transport] Reconnecting attempt %d/%d, backoff=%v", ...)` | `slog.Info("Reconnecting", "component", "transport", "attempt", i, "max", c.config.MaxReconnects, "backoff", backoff)` |
| 144 | `log.Printf("[client.transport] Connect failed: %v", err)` | `slog.Error("Connect failed", "component", "transport", "error", err)` |
| 178 | `log.Printf("[client.transport] WebSocket closed normally")` | `slog.Info("WebSocket closed normally", "component", "transport")` |
| 184 | `log.Printf("[client.transport] WebSocket read error: %v", err)` | `slog.Error("WebSocket read error", "component", "transport", "error", err)` |
| 212 | `log.Printf("[client.transport] Ping failed: %v", err)` | `slog.Warn("Ping failed", "component", "transport", "error", err)` |
| 327 | `log.Printf("[client.transport] WebSocket closed")` | `slog.Info("WebSocket closed", "component", "transport")` |

- [ ] **Step 2: Verify**

Run: `go build ./transport/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add transport/conn.go
git commit -m "refactor: migrate transport logging to slog"
```

---

### Task 3: Migrate `stt/client.go`

**Files:**
- Modify: `stt/client.go`

- [ ] **Step 1: Replace import and all log.Printf calls**

Replace `"log"` import with `"log/slog"`.

| Line | Old | New |
|------|-----|-----|
| 141 | `log.Printf("[client.stt] Idle timeout %v ...")` | `slog.Warn("Idle timeout, closing", "component", "stt", "timeout", recognizeIdleTimeout)` |
| 151 | `log.Printf("[client.stt] RecognizeFile completed: ...")` | `slog.Info("RecognizeFile completed", "component", "stt", "file", audioPath, "text", truncateText(result.Text, 50), "duration_ms", result.Duration.Milliseconds(), "ttfb_ms", result.TTFB.Milliseconds())` |
| 309 | `log.Printf("[client.stt] Idle timeout %v ...")` | `slog.Warn("Idle timeout, closing", "component", "stt", "timeout", recognizeIdleTimeout)` |

- [ ] **Step 2: Verify**

Run: `go build ./stt/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add stt/client.go
git commit -m "refactor: migrate stt/client logging to slog"
```

---

### Task 4: Migrate `stt/session.go`

**Files:**
- Modify: `stt/session.go`

- [ ] **Step 1: Replace import and all log.Printf calls**

Replace `"log"` import with `"log/slog"`.

| Line | Old | New | Level |
|------|-----|-----|-------|
| 102 | Session ready | `slog.Info("Session ready", "component", "stt", "id", s.ID, "provider", s.Provider)` | INFO |
| 149 | Parse message error | `slog.Warn("Parse message error", "component", "stt", "error", err)` | WARN |
| 169 | Unknown message type | `slog.Warn("Unknown message type", "component", "stt", "type", msgType)` | WARN |
| 178 | Parse partial error | `slog.Warn("Parse partial error", "component", "stt", "error", err)` | WARN |
| 192 | Parse final error | `slog.Warn("Parse final error", "component", "stt", "error", err)` | WARN |
| 209 | Parse error message error | `slog.Warn("Parse error message error", "component", "stt", "error", err)` | WARN |
| 225 | Event buffer full | `slog.Warn("Event buffer full, dropping event", "component", "stt", "event_type", event.Type)` | WARN |
| 280 | TTFB | `slog.Info("TTFB", "component", "stt", "ttfb_ms", s.ttfb.Milliseconds())` | INFO |
| 314 | Close timeout | `slog.Warn("Close timeout, forcing close", "component", "stt", "id", s.ID)` | WARN |
| 321 | Session closed | `slog.Info("Session closed", "component", "stt", "id", s.ID)` | INFO |

- [ ] **Step 2: Verify**

Run: `go build ./stt/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add stt/session.go
git commit -m "refactor: migrate stt/session logging to slog"
```

---

### Task 5: Migrate `tts/client.go`

**Files:**
- Modify: `tts/client.go`

- [ ] **Step 1: Replace import and log.Printf call**

Replace `"log"` import with `"log/slog"`.

| Line | Old | New |
|------|-----|-----|
| 58 | `log.Printf("[client.tts] SynthesizeToFile completed: ...")` | `slog.Info("SynthesizeToFile completed", "component", "tts", "output", outputPath, "size_bytes", written, "duration_ms", time.Since(start).Milliseconds())` |

- [ ] **Step 2: Verify**

Run: `go build ./tts/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add tts/client.go
git commit -m "refactor: migrate tts/client logging to slog"
```

---

### Task 6: Migrate `tts/session.go`

**Files:**
- Modify: `tts/session.go`

- [ ] **Step 1: Replace import and all log.Printf calls**

Replace `"log"` import with `"log/slog"`.

| Line | Old | New | Level |
|------|-----|-----|-------|
| 111 | Session ready | `slog.Info("Session ready", "component", "tts", "id", s.ID, "provider", s.Provider)` | INFO |
| 166 | Parse message error | `slog.Warn("Parse message error", "component", "tts", "error", err)` | WARN |
| 180 | Unknown message type | `slog.Warn("Unknown message type", "component", "tts", "type", msgType)` | WARN |
| 194 | Config done | `slog.Info("Config done", "component", "tts", "id", s.ID)` | INFO |
| 208 | Parse audio.delta error | `slog.Warn("Parse audio.delta error", "component", "tts", "error", err)` | WARN |
| 217 | Decode audio error | `slog.Warn("Decode audio error", "component", "tts", "error", err)` | WARN |
| 237 | Parse error message error | `slog.Warn("Parse error message error", "component", "tts", "error", err)` | WARN |
| 268 | Round completed | `slog.Info("Round completed", "component", "tts", "round", s.roundCount, "id", s.ID)` | INFO |
| 342 | Round started | `slog.Info("Round started", "component", "tts", "round", round, "text_len", len(text), "id", s.ID)` | INFO |
| 410 | Close timeout | `slog.Warn("Close timeout, forcing close", "component", "tts", "id", s.ID)` | WARN |
| 426 | Session closed | `slog.Info("Session closed", "component", "tts", "id", s.ID, "rounds", s.roundCount)` | INFO |

- [ ] **Step 2: Verify**

Run: `go build ./tts/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add tts/session.go
git commit -m "refactor: migrate tts/session logging to slog"
```

---

### Task 7: Migrate `examples-sdk/stt_stream/main.go`

**Files:**
- Modify: `examples-sdk/stt_stream/main.go`

- [ ] **Step 1: Add logging import and Setup call**

Add imports:
```go
"log/slog"
"github.com/jinbozhan/tengen-speech-sdk-go/logging"
```

Remove `"log"` import.

Add as first line of `main()`:
```go
logging.Setup(slog.LevelInfo)
```

- [ ] **Step 2: Replace log.Fatalf calls with slog.Error + os.Exit(1)**

| Line | Old | New |
|------|-----|-----|
| 69 | `log.Fatalf("音频文件不存在: %s", audioFile)` | `slog.Error("音频文件不存在", "file", audioFile); os.Exit(1)` |
| 101 | `log.Fatalf("创建客户端失败: %v", err)` | `slog.Error("创建客户端失败", "error", err); os.Exit(1)` |
| 121 | `log.Fatalf("创建会话失败: %v", err)` | `slog.Error("创建会话失败", "error", err); os.Exit(1)` |
| 128 | `log.Fatalf("打开文件失败: %v", err)` | `slog.Error("打开文件失败", "error", err); os.Exit(1)` |
| 148 | `log.Fatalf("识别失败: %v", err)` | `slog.Error("识别失败", "error", err); os.Exit(1)` |
| 153 | `log.Fatalf("发送音频失败: %v", err)` | `slog.Error("发送音频失败", "error", err); os.Exit(1)` |

Note: `fmt.Printf/Println` for user-facing output (config display, recognition events with `\r`) stay as-is.

- [ ] **Step 3: Verify**

Run: `go build ./examples-sdk/stt_stream/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add examples-sdk/stt_stream/main.go
git commit -m "refactor: migrate examples-sdk/stt_stream logging to slog"
```

---

### Task 8: Migrate `examples-sdk/tts_stream/main.go`

**Files:**
- Modify: `examples-sdk/tts_stream/main.go`

- [ ] **Step 1: Add logging import and Setup, replace log.Fatalf**

Same pattern as Task 7:
- Add `logging.Setup(slog.LevelInfo)` at start of `main()`
- Replace `"log"` with `"log/slog"` + `logging` import
- Replace 4 `log.Fatalf` calls (lines 103, 112, 125, 133) with `slog.Error(...)` + `os.Exit(1)`

- [ ] **Step 2: Verify**

Run: `go build ./examples-sdk/tts_stream/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add examples-sdk/tts_stream/main.go
git commit -m "refactor: migrate examples-sdk/tts_stream logging to slog"
```

---

### ~~Task 9 & 10: examples-ws/ (REMOVED)~~

> `examples-ws/` 已被删除，这两个 task 不再适用。见 "删除 examples-ws/，统一使用 SDK 接口" 改动。

---

### Task 11: Migrate `cmd/tts_benchmark/main.go`

**Files:**
- Modify: `cmd/tts_benchmark/main.go`

- [ ] **Step 1: Add logging import and Setup, replace log calls**

- Add `logging.Setup(slog.LevelInfo)` at start of `main()`
- Replace `"log"` with `"log/slog"` + `logging` import
- Replace calls:

| Line | Old | New |
|------|-----|-----|
| 70 | `log.Fatalf("Invalid voice config: %v", err)` | `slog.Error("Invalid voice config", "error", err); os.Exit(1)` |
| 75 | `log.Println("Warning: ...")` | `slog.Warn("No API key provided, set -api-key or GATEWAY_API_KEY")` |
| 114 | `log.Fatalf("Benchmark failed: %v", err)` | `slog.Error("Benchmark failed", "error", err); os.Exit(1)` |
| 118 | `log.Printf("Benchmark completed in %v", elapsed)` | `slog.Info("Benchmark completed", "elapsed", elapsed.Round(time.Second))` |
| 123 | `log.Fatalf("Failed to generate report: %v", err)` | `slog.Error("Failed to generate report", "error", err); os.Exit(1)` |

- [ ] **Step 2: Verify**

Run: `go build ./cmd/tts_benchmark/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/tts_benchmark/main.go
git commit -m "refactor: migrate cmd/tts_benchmark/main logging to slog"
```

---

### Task 12: Migrate `cmd/tts_benchmark/benchmark.go`

**Files:**
- Modify: `cmd/tts_benchmark/benchmark.go`

- [ ] **Step 1: Replace import and all log.Printf calls**

Replace `"log"` with `"log/slog"`.

Key replacements:

| Lines | Old | New | Level |
|-------|-----|-----|-------|
| 84-90 | `log.Printf("Starting benchmark:")` etc. | `slog.Info("Starting benchmark", "gateway", ..., "voices", ..., "concurrency", ..., "requests_per_worker", ..., "total_requests", ..., "save_audio", ...)` | INFO |
| 112 | `log.Printf("Started worker %d ...")` | `slog.Info("Started worker", "worker_id", workerID-1, "voice", voice.DisplayID)` | INFO |
| 154 | `log.Printf("Worker %d req %d: ...")` | `slog.Info("Request completed", "worker_id", workerID, "req_id", reqID, "status", status, "ttfb_ms", metrics.TTFBMs, "total_ms", metrics.TotalMs)` | INFO |
| 172 | `[START]` | `slog.Debug("[START]", "worker_id", workerID, "req_id", reqID, "voice", voiceID)` | DEBUG |
| 195 | `[CONNECT_START]` | `slog.Debug("[CONNECT_START]", ...)` | DEBUG |
| 221 | `[CONNECTED]` | `slog.Debug("[CONNECTED]", ...)` | DEBUG |
| 230 | `[COMMIT_SENT]` | `slog.Debug("[COMMIT_SENT]", ...)` | DEBUG |
| 271 | `[FIRST_CHUNK]` | `slog.Debug("[FIRST_CHUNK]", ...)` | DEBUG |
| 291 | `[COMPLETE]` | `slog.Debug("[COMPLETE]", ...)` | DEBUG |
| 308 | Warning save audio | `slog.Warn("Failed to save audio", "error", err)` | WARN |
| 347 | Progress | `slog.Info("Progress", "completed", completed, "total", b.totalRequests, "percent", percent, "active_workers", active)` | INFO |

Note: The `verboseTiming` env var check can be removed — these calls naturally become `slog.Debug` which only appears if the user sets the log level to DEBUG.

For benchmark, add level control: if `config.Verbose` is true, use `slog.LevelDebug` in Setup, otherwise `slog.LevelInfo`. Move the `logging.Setup()` call to `main()` and pass level based on `verbose` flag.

- [ ] **Step 2: Verify**

Run: `go build ./cmd/tts_benchmark/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/tts_benchmark/benchmark.go
git commit -m "refactor: migrate cmd/tts_benchmark/benchmark logging to slog"
```

---

### Task 13: Migrate `cmd/tts_detailed_timing/main.go`

**Files:**
- Modify: `cmd/tts_detailed_timing/main.go`

- [ ] **Step 1: Replace import and log.Fatal**

- Add `logging.Setup(slog.LevelInfo)` at start of `main()`
- Replace `"log"` with `"log/slog"` + `logging` import
- Replace `log.Fatal("Text cannot be empty")` with `slog.Error("Text cannot be empty"); os.Exit(1)`

Note: All `fmt.Printf` with color codes are user-facing terminal output — keep as-is.

- [ ] **Step 2: Verify**

Run: `go build ./cmd/tts_detailed_timing/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/tts_detailed_timing/main.go
git commit -m "refactor: migrate cmd/tts_detailed_timing logging to slog"
```

---

### Task 14: Migrate `cmd/test_vad_clip_asr/main.go`

**Files:**
- Modify: `cmd/test_vad_clip_asr/main.go`

- [ ] **Step 1: Replace import and log calls**

- Add `logging.Setup(slog.LevelInfo)` at start of `main()`
- Replace `"log"` with `"log/slog"` + `logging` import

| Line | Old | New |
|------|-----|-----|
| 82 | `log.Fatalf("查找 WAV 文件失败: %v", err)` | `slog.Error("查找 WAV 文件失败", "error", err); os.Exit(1)` |
| 85 | `log.Fatalf("未找到 WAV 文件: %s/*.wav", sampleDir)` | `slog.Error("未找到 WAV 文件", "dir", sampleDir); os.Exit(1)` |
| 166 | `log.Printf("写入报告失败: %v", err)` | `slog.Error("写入报告失败", "error", err)` |

- [ ] **Step 2: Verify**

Run: `go build ./cmd/test_vad_clip_asr/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add cmd/test_vad_clip_asr/main.go
git commit -m "refactor: migrate cmd/test_vad_clip_asr logging to slog"
```

---

### Task 15: Align `examples-sdk/stt_stream` EventError handling

**Files:**
- Modify: `examples-sdk/stt_stream/main.go`

- [ ] **Step 1: Update recognizeStreaming to match ws version's error handling**

Change `EventError` case from immediately returning error to recording and continuing:

```go
// Before:
case stt.EventError:
	return finalTexts, fmt.Errorf("识别错误: %v", event.Error)

// After:
case stt.EventError:
	fmt.Printf("\n[错误] %v\n", event.Error)
	lastErr = event.Error
```

Add `var lastErr error` at the top, return `lastErr` at the end.

Add `EventProcessing` case:
```go
case stt.EventProcessing:
	// 服务端心跳，忽略
```

- [ ] **Step 2: Verify**

Run: `go build ./examples-sdk/stt_stream/`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add examples-sdk/stt_stream/main.go
git commit -m "fix: align stt_stream error handling - record and continue instead of immediate return"
```

---

### Task 16: Final verification

- [ ] **Step 1: Build all packages**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 2: Run go vet**

Run: `go vet ./...`
Expected: no warnings

- [ ] **Step 3: Verify no remaining old log imports in SDK code**

Run: `grep -rn '"log"' stt/ tts/ transport/`
Expected: no matches (only `"log/slog"` should remain)

- [ ] **Step 4: Spot check log output format**

Run one example (if gateway is available):
```bash
./stt_stream -gateway ws://localhost:8080 audio.wav
```

Expected SDK logs format:
```
2026-03-18 15:04:05.000 INFO  transport/conn.go:116 WebSocket connected component=transport url=ws://localhost:8080 connect_duration_ms=45
2026-03-18 15:04:05.100 INFO  stt/session.go:102 Session ready component=stt id=abc123 provider=azure
```
