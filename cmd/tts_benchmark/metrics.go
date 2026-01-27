// Package main 提供TTS并发测试工具
package main

import (
	"sort"
	"sync"
	"time"
)

// RequestMetrics 单次请求的指标
type RequestMetrics struct {
	VoiceID   string // 音色ID
	WorkerID  int    // Worker编号
	RequestID int    // 请求编号
	Text      string // 测试文本
	TextLen   int    // 文本长度

	// 时间戳
	StartTime    time.Time // 请求开始时间
	ConnectedAt  time.Time // WebSocket 连接成功时间
	ConfigDoneAt time.Time // session.config_done 收到时间
	FirstByteAt  time.Time // 首个 audio.delta 收到时间 (TTFB)
	CompleteAt   time.Time // audio.done 收到时间

	// 计算指标 (毫秒)
	ConnectMs   int64 // 连接耗时（TCP+TLS+WS握手）
	ConfigMs    int64 // 配置耗时
	SynthesisMs int64 // 纯服务端处理时间（commit到首包，与TTFBMs相同）
	TTFBMs      int64 // Time To First Byte（从commit到首包，精确测量）
	TotalMs     int64 // 总耗时

	// 数据统计
	ChunkCount int   // 音频块数量
	TotalBytes int64 // 音频总字节

	// 状态
	Success bool   // 是否成功
	Error   string // 错误信息（如有）

	// 音频文件路径（如果保存了）
	AudioFile string
}

// AggregatedMetrics 聚合指标
type AggregatedMetrics struct {
	VoiceID       string  // 音色ID ("ALL" 表示总体)
	TotalRequests int     // 总请求数
	SuccessCount  int     // 成功数
	FailCount     int     // 失败数
	SuccessRate   float64 // 成功率

	// ConnectMs 统计（建连耗时：TCP+TLS+WS握手）
	ConnectMin int64
	ConnectMax int64
	ConnectAvg int64
	ConnectP50 int64
	ConnectP95 int64
	ConnectP99 int64

	// SynthesisMs 统计（纯服务端处理时间：commit到首包）
	SynthesisMin int64
	SynthesisMax int64
	SynthesisAvg int64
	SynthesisP50 int64
	SynthesisP95 int64
	SynthesisP99 int64

	// TTFB 统计 (毫秒)
	TTFBMin int64
	TTFBMax int64
	TTFBAvg int64
	TTFBP50 int64
	TTFBP95 int64
	TTFBP99 int64

	// 总耗时统计 (毫秒)
	TotalTimeMin int64
	TotalTimeMax int64
	TotalTimeAvg int64
	TotalTimeP50 int64
	TotalTimeP95 int64
	TotalTimeP99 int64

	// 吞吐量
	RPS         float64 // 每秒请求数
	BytesPerSec float64 // 每秒字节数

	// 错误分布
	ErrorCounts map[string]int
}

// MetricsCollector 线程安全的指标收集器
type MetricsCollector struct {
	mu        sync.Mutex
	metrics   []RequestMetrics
	startTime time.Time
	endTime   time.Time
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: make([]RequestMetrics, 0, 1000),
	}
}

// Start 标记测试开始
func (c *MetricsCollector) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.startTime = time.Now()
}

// End 标记测试结束
func (c *MetricsCollector) End() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.endTime = time.Now()
}

// Record 记录单次请求指标
func (c *MetricsCollector) Record(m RequestMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metrics = append(c.metrics, m)
}

// GetAll 获取所有指标
func (c *MetricsCollector) GetAll() []RequestMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]RequestMetrics, len(c.metrics))
	copy(result, c.metrics)
	return result
}

// Duration 返回测试持续时间
func (c *MetricsCollector) Duration() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.endTime.IsZero() {
		return time.Since(c.startTime)
	}
	return c.endTime.Sub(c.startTime)
}

// Aggregate 计算聚合指标
func (c *MetricsCollector) Aggregate() map[string]*AggregatedMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 按 VoiceID 分组
	grouped := make(map[string][]RequestMetrics)
	for _, m := range c.metrics {
		grouped[m.VoiceID] = append(grouped[m.VoiceID], m)
	}

	duration := c.endTime.Sub(c.startTime).Seconds()
	if duration <= 0 {
		duration = 1
	}

	result := make(map[string]*AggregatedMetrics)
	for voiceID, metrics := range grouped {
		result[voiceID] = calculateAggregated(voiceID, metrics, duration)
	}

	// 添加总体统计
	result["ALL"] = calculateAggregated("ALL", c.metrics, duration)

	return result
}

// calculateAggregated 计算聚合指标
func calculateAggregated(voiceID string, metrics []RequestMetrics, durationSec float64) *AggregatedMetrics {
	if len(metrics) == 0 {
		return &AggregatedMetrics{VoiceID: voiceID}
	}

	agg := &AggregatedMetrics{
		VoiceID:       voiceID,
		TotalRequests: len(metrics),
		ErrorCounts:   make(map[string]int),
	}

	var connectValues []int64
	var synthesisValues []int64
	var ttfbValues []int64
	var totalTimeValues []int64
	var totalBytes int64
	var connectSum, synthesisSum, ttfbSum, totalTimeSum int64

	for _, m := range metrics {
		if m.Success {
			agg.SuccessCount++
			if m.ConnectMs > 0 {
				connectValues = append(connectValues, m.ConnectMs)
				connectSum += m.ConnectMs
			}
			if m.SynthesisMs > 0 {
				synthesisValues = append(synthesisValues, m.SynthesisMs)
				synthesisSum += m.SynthesisMs
			}
			if m.TTFBMs > 0 {
				ttfbValues = append(ttfbValues, m.TTFBMs)
				ttfbSum += m.TTFBMs
			}
			if m.TotalMs > 0 {
				totalTimeValues = append(totalTimeValues, m.TotalMs)
				totalTimeSum += m.TotalMs
			}
			totalBytes += m.TotalBytes
		} else {
			agg.FailCount++
			errKey := m.Error
			if errKey == "" {
				errKey = "unknown"
			}
			// 截断错误信息用于分类
			if len(errKey) > 50 {
				errKey = errKey[:50] + "..."
			}
			agg.ErrorCounts[errKey]++
		}
	}

	// 成功率
	if agg.TotalRequests > 0 {
		agg.SuccessRate = float64(agg.SuccessCount) / float64(agg.TotalRequests)
	}

	// ConnectMs 统计（建连耗时）
	if len(connectValues) > 0 {
		sort.Slice(connectValues, func(i, j int) bool { return connectValues[i] < connectValues[j] })
		agg.ConnectMin = connectValues[0]
		agg.ConnectMax = connectValues[len(connectValues)-1]
		agg.ConnectAvg = connectSum / int64(len(connectValues))
		agg.ConnectP50 = percentile(connectValues, 50)
		agg.ConnectP95 = percentile(connectValues, 95)
		agg.ConnectP99 = percentile(connectValues, 99)
	}

	// SynthesisMs 统计（纯服务端处理时间）
	if len(synthesisValues) > 0 {
		sort.Slice(synthesisValues, func(i, j int) bool { return synthesisValues[i] < synthesisValues[j] })
		agg.SynthesisMin = synthesisValues[0]
		agg.SynthesisMax = synthesisValues[len(synthesisValues)-1]
		agg.SynthesisAvg = synthesisSum / int64(len(synthesisValues))
		agg.SynthesisP50 = percentile(synthesisValues, 50)
		agg.SynthesisP95 = percentile(synthesisValues, 95)
		agg.SynthesisP99 = percentile(synthesisValues, 99)
	}

	// TTFB 统计
	if len(ttfbValues) > 0 {
		sort.Slice(ttfbValues, func(i, j int) bool { return ttfbValues[i] < ttfbValues[j] })
		agg.TTFBMin = ttfbValues[0]
		agg.TTFBMax = ttfbValues[len(ttfbValues)-1]
		agg.TTFBAvg = ttfbSum / int64(len(ttfbValues))
		agg.TTFBP50 = percentile(ttfbValues, 50)
		agg.TTFBP95 = percentile(ttfbValues, 95)
		agg.TTFBP99 = percentile(ttfbValues, 99)
	}

	// 总耗时统计
	if len(totalTimeValues) > 0 {
		sort.Slice(totalTimeValues, func(i, j int) bool { return totalTimeValues[i] < totalTimeValues[j] })
		agg.TotalTimeMin = totalTimeValues[0]
		agg.TotalTimeMax = totalTimeValues[len(totalTimeValues)-1]
		agg.TotalTimeAvg = totalTimeSum / int64(len(totalTimeValues))
		agg.TotalTimeP50 = percentile(totalTimeValues, 50)
		agg.TotalTimeP95 = percentile(totalTimeValues, 95)
		agg.TotalTimeP99 = percentile(totalTimeValues, 99)
	}

	// 吞吐量
	if durationSec > 0 {
		agg.RPS = float64(agg.SuccessCount) / durationSec
		agg.BytesPerSec = float64(totalBytes) / durationSec
	}

	return agg
}

// percentile 计算分位数
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx]
}
