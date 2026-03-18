// Package logging 提供人类友好的 slog 日志格式化
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

// Handler 实现 slog.Handler 接口，输出人类友好的日志格式
//
// 输出格式:
//
//	2024-01-01 15:04:05.000 INFO  transport/conn.go:116 WebSocket connected url=ws://localhost:8080 duration=45ms
type Handler struct {
	level slog.Leveler
	w     io.Writer
	mu    *sync.Mutex
	attrs []slog.Attr
	group string
}

// NewHandler 创建自定义日志 Handler
func NewHandler(w io.Writer, level slog.Leveler) *Handler {
	return &Handler{
		level: level,
		w:     w,
		mu:    &sync.Mutex{},
	}
}

// Enabled 检查日志级别是否启用
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle 处理日志记录
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	// 时间
	buf := make([]byte, 0, 256)
	buf = append(buf, r.Time.Format("2006-01-02 15:04:05.000")...)
	buf = append(buf, ' ')

	// 级别（固定宽度 5 字符）
	buf = append(buf, levelString(r.Level)...)
	buf = append(buf, ' ')

	// 来源文件（包名/文件名:行号）
	if r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		if f.File != "" {
			buf = append(buf, shortSource(f.File)...)
			buf = append(buf, ':')
			buf = append(buf, fmt.Sprintf("%d", f.Line)...)
			buf = append(buf, ' ')
		}
	}

	// 消息
	buf = append(buf, r.Message...)

	// 预存的 attrs
	for _, a := range h.attrs {
		buf = appendAttr(buf, h.group, a)
	}

	// Record 中的 attrs
	r.Attrs(func(a slog.Attr) bool {
		buf = appendAttr(buf, h.group, a)
		return true
	})

	buf = append(buf, '\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf)
	return err
}

// WithAttrs 返回附加了属性的新 Handler
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	newAttrs = append(newAttrs, attrs...)
	return &Handler{
		level: h.level,
		w:     h.w,
		mu:    h.mu,
		attrs: newAttrs,
		group: h.group,
	}
}

// WithGroup 返回带分组的新 Handler
func (h *Handler) WithGroup(name string) slog.Handler {
	g := name
	if h.group != "" {
		g = h.group + "." + name
	}
	newAttrs := make([]slog.Attr, len(h.attrs))
	copy(newAttrs, h.attrs)
	return &Handler{
		level: h.level,
		w:     h.w,
		mu:    h.mu,
		attrs: newAttrs,
		group: g,
	}
}

// Level constants re-exported from slog, so callers only need to import logging.
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

// Setup creates a Handler writing to stderr at the given level and sets it as
// the slog default.
func Setup(level slog.Level) {
	handler := NewHandler(os.Stderr, level)
	slog.SetDefault(slog.New(handler))
}

// Log functions — thin wrappers so callers only need to import logging.

func Debug(msg string, args ...any) { slog.Debug(msg, args...) }
func Info(msg string, args ...any)  { slog.Info(msg, args...) }
func Warn(msg string, args ...any)  { slog.Warn(msg, args...) }
func Error(msg string, args ...any) { slog.Error(msg, args...) }

// levelString 返回固定宽度的级别字符串
func levelString(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN "
	case level >= slog.LevelInfo:
		return "INFO "
	default:
		return "DEBUG"
	}
}

// shortSource 截取 "包名/文件名" 形式的源文件路径
// 例如 "/home/user/go/src/project/transport/conn.go" → "transport/conn.go"
func shortSource(file string) string {
	dir, base := filepath.Split(file)
	if dir == "" {
		return base
	}
	parent := filepath.Base(filepath.Clean(dir))
	return parent + "/" + base
}

// appendAttr 将属性追加到 buf
func appendAttr(buf []byte, group string, a slog.Attr) []byte {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return buf
	}

	buf = append(buf, ' ')
	if group != "" {
		buf = append(buf, group...)
		buf = append(buf, '.')
	}
	buf = append(buf, a.Key...)
	buf = append(buf, '=')

	switch a.Value.Kind() {
	case slog.KindString:
		val := a.Value.String()
		if needsQuoting(val) {
			buf = append(buf, fmt.Sprintf("%q", val)...)
		} else {
			buf = append(buf, val...)
		}
	case slog.KindTime:
		buf = append(buf, a.Value.Time().Format(time.RFC3339Nano)...)
	case slog.KindDuration:
		buf = append(buf, a.Value.Duration().String()...)
	default:
		buf = append(buf, fmt.Sprintf("%v", a.Value.Any())...)
	}
	return buf
}

// needsQuoting 判断字符串是否需要引号
func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for _, c := range s {
		if c <= ' ' || c == '"' || c == '=' {
			return true
		}
	}
	return false
}
