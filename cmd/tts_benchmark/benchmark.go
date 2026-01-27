// Package main 提供TTS并发测试工具
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/tts"
)

// BenchmarkConfig 测试配置
type BenchmarkConfig struct {
	GatewayURL string        // Gateway WebSocket URL
	Provider   string        // 提供商
	APIKey     string        // API Key
	Voices     []VoiceConfig // 音色配置列表
	Requests   int           // 每个 worker 的请求数
	RampUp     time.Duration // 预热时间
	OutputDir  string        // 输出目录
	SaveAudio  bool          // 是否保存音频
	Verbose    bool          // 详细日志
}

// VoiceConfig 音色配置
type VoiceConfig struct {
	DisplayID   string // 音色显示ID
	Concurrency int    // 并发数
}

// Benchmark 测试执行器
type Benchmark struct {
	config    *BenchmarkConfig
	collector *MetricsCollector
	texts     *TextProvider

	// 统计
	activeWorkers int64
	completedReqs int64
	totalRequests int64

	// 控制
	stopCh chan struct{}
}

// NewBenchmark 创建测试执行器
func NewBenchmark(config *BenchmarkConfig) *Benchmark {
	// 计算总请求数
	var totalReqs int64
	for _, v := range config.Voices {
		totalReqs += int64(v.Concurrency * config.Requests)
	}

	return &Benchmark{
		config:        config,
		collector:     NewMetricsCollector(),
		texts:         NewTextProvider(),
		totalRequests: totalReqs,
		stopCh:        make(chan struct{}),
	}
}

// Run 执行测试
func (b *Benchmark) Run(ctx context.Context) error {
	// 确保音频输出目录存在
	if b.config.SaveAudio {
		audioDir := filepath.Join(b.config.OutputDir, "audio")
		if err := os.MkdirAll(audioDir, 0755); err != nil {
			return fmt.Errorf("create audio dir: %w", err)
		}
	}

	// 计算总并发数
	totalConcurrency := 0
	for _, v := range b.config.Voices {
		totalConcurrency += v.Concurrency
	}

	log.Printf("Starting benchmark:")
	log.Printf("  Gateway:     %s", b.config.GatewayURL)
	log.Printf("  Provider:    %s", b.config.Provider)
	log.Printf("  Voices:      %d types", len(b.config.Voices))
	log.Printf("  Concurrency: %d total", totalConcurrency)
	log.Printf("  Requests:    %d per worker, %d total", b.config.Requests, b.totalRequests)
	log.Printf("  Save Audio:  %v", b.config.SaveAudio)

	b.collector.Start()

	var wg sync.WaitGroup
	workerID := 0

	// 为每个音色启动对应数量的 worker
	for _, voice := range b.config.Voices {
		for i := 0; i < voice.Concurrency; i++ {
			wg.Add(1)

			// RampUp: 逐步启动 worker
			if b.config.RampUp > 0 && totalConcurrency > 1 {
				delay := time.Duration(workerID) * b.config.RampUp / time.Duration(totalConcurrency)
				time.Sleep(delay)
			}

			go b.runWorker(ctx, &wg, workerID, voice.DisplayID)
			workerID++

			if b.config.Verbose {
				log.Printf("Started worker %d for voice %s", workerID-1, voice.DisplayID)
			}
		}
	}

	// 启动进度报告
	progressDone := make(chan struct{})
	go b.reportProgress(progressDone)

	// 等待所有 worker 完成
	wg.Wait()
	close(progressDone)

	b.collector.End()

	return nil
}

// runWorker 单个 Worker 的执行逻辑
func (b *Benchmark) runWorker(ctx context.Context, wg *sync.WaitGroup, workerID int, voiceID string) {
	defer wg.Done()
	atomic.AddInt64(&b.activeWorkers, 1)
	defer atomic.AddInt64(&b.activeWorkers, -1)

	for reqID := 0; reqID < b.config.Requests; reqID++ {
		select {
		case <-ctx.Done():
			return
		case <-b.stopCh:
			return
		default:
		}

		metrics := b.executeRequest(ctx, workerID, reqID, voiceID)
		b.collector.Record(metrics)
		atomic.AddInt64(&b.completedReqs, 1)

		if b.config.Verbose {
			status := "OK"
			if !metrics.Success {
				status = fmt.Sprintf("FAIL: %s", metrics.Error)
			}
			log.Printf("Worker %d req %d: %s, TTFB=%dms, Total=%dms",
				workerID, reqID, status, metrics.TTFBMs, metrics.TotalMs)
		}
	}
}

// executeRequest 执行单次请求
func (b *Benchmark) executeRequest(ctx context.Context, workerID, reqID int, voiceID string) RequestMetrics {
	metrics := RequestMetrics{
		VoiceID:   voiceID,
		WorkerID:  workerID,
		RequestID: reqID,
		StartTime: time.Now(),
	}

	// 检查是否启用详细时间戳打印
	verboseTiming := os.Getenv("VERBOSE_TIMING") == "1"
	if verboseTiming {
		log.Printf("[START] Worker=%d Req=%d Voice=%s Time=%s",
			workerID, reqID, voiceID, metrics.StartTime.Format("2006-01-02 15:04:05.000"))
	}

	// 获取测试文本
	text := b.texts.GetRandom()
	metrics.Text = text
	metrics.TextLen = len(text)

	// 创建客户端配置
	clientConfig := &tts.Config{
		GatewayURL:     b.config.GatewayURL,
		Provider:       b.config.Provider,
		APIKey:         b.config.APIKey,
		VoiceID:        voiceID,
		Speed:          1.0,
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	// 创建客户端
	if verboseTiming {
		log.Printf("[CONNECT_START] Worker=%d Req=%d", workerID, reqID)
	}
	client, err := tts.NewClient(clientConfig)
	if err != nil {
		metrics.Success = false
		metrics.Error = fmt.Sprintf("create client: %v", err)
		metrics.TotalMs = time.Since(metrics.StartTime).Milliseconds()
		return metrics
	}
	defer client.Close()

	// 创建流式会话
	stream, err := client.SynthesizeStream(ctx, text)
	if err != nil {
		metrics.Success = false
		metrics.Error = fmt.Sprintf("synthesize: %v", err)
		metrics.TotalMs = time.Since(metrics.StartTime).Milliseconds()
		return metrics
	}
	defer stream.Close()

	// 获取建连时间（从 stream 暴露的 session 方法）
	metrics.ConnectMs = stream.ConnectDuration().Milliseconds()
	metrics.ConnectedAt = stream.ConnectedAt()

	if verboseTiming && !metrics.ConnectedAt.IsZero() {
		log.Printf("[CONNECTED] Worker=%d Req=%d ConnectMs=%dms Time=%s",
			workerID, reqID, metrics.ConnectMs, metrics.ConnectedAt.Format("2006-01-02 15:04:05.000"))
	}

	// 获取 commit 发送时间（用于计算 SynthesisMs）
	commitSentAt := stream.CommitSentAt()

	if verboseTiming && !commitSentAt.IsZero() {
		commitDelayMs := commitSentAt.Sub(metrics.StartTime).Milliseconds()
		log.Printf("[COMMIT_SENT] Worker=%d Req=%d CommitDelayMs=%dms Time=%s",
			workerID, reqID, commitDelayMs, commitSentAt.Format("2006-01-02 15:04:05.000"))
	}

	// 接收音频数据
	buf := make([]byte, 4096)
	var audioData []byte
	firstChunk := true

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			metrics.Success = false
			metrics.Error = fmt.Sprintf("read: %v", err)
			break
		}

		if n > 0 {
			if firstChunk {
				// 优先使用 session 记录的精确首包时间
				metrics.FirstByteAt = stream.FirstChunkReceivedAt()
				if metrics.FirstByteAt.IsZero() {
					// 回退到应用层时间
					metrics.FirstByteAt = time.Now()
				}

				// 使用精确的 TTFB（从 commit 到首包）
				metrics.TTFBMs = stream.TTFB()
				if metrics.TTFBMs == 0 && !commitSentAt.IsZero() {
					metrics.TTFBMs = metrics.FirstByteAt.Sub(commitSentAt).Milliseconds()
				}

				// SynthesisMs 保持与 TTFBMs 一致（从 commit 到首包）
				if !commitSentAt.IsZero() {
					metrics.SynthesisMs = metrics.FirstByteAt.Sub(commitSentAt).Milliseconds()
				}

				if verboseTiming {
					log.Printf("[FIRST_CHUNK] Worker=%d Req=%d TTFB=%dms SynthesisMs=%dms ChunkSize=%d Time=%s",
						workerID, reqID, metrics.TTFBMs, metrics.SynthesisMs, n,
						metrics.FirstByteAt.Format("2006-01-02 15:04:05.000"))
				}
				firstChunk = false
			}
			metrics.ChunkCount++
			metrics.TotalBytes += int64(n)

			// 如果需要保存音频，累积数据
			if b.config.SaveAudio {
				audioData = append(audioData, buf[:n]...)
			}
		}
	}

	metrics.CompleteAt = time.Now()
	metrics.TotalMs = metrics.CompleteAt.Sub(metrics.StartTime).Milliseconds()

	if verboseTiming && !metrics.CompleteAt.IsZero() {
		log.Printf("[COMPLETE] Worker=%d Req=%d TotalMs=%dms TotalBytes=%d ChunkCount=%d Time=%s",
			workerID, reqID, metrics.TotalMs, metrics.TotalBytes, metrics.ChunkCount,
			metrics.CompleteAt.Format("2006-01-02 15:04:05.000"))
	}

	// 检查流错误
	if streamErr := stream.Error(); streamErr != nil {
		metrics.Success = false
		metrics.Error = streamErr.Error()
	} else if !firstChunk {
		// 至少收到了一个音频块
		metrics.Success = true

		// 保存音频文件
		if b.config.SaveAudio && len(audioData) > 0 {
			audioFile, err := b.saveAudio(voiceID, workerID, reqID, audioData)
			if err != nil {
				log.Printf("Warning: failed to save audio: %v", err)
			} else {
				metrics.AudioFile = audioFile
			}
		}
	} else {
		metrics.Success = false
		metrics.Error = "no audio data received"
	}

	return metrics
}

// saveAudio 保存音频文件
func (b *Benchmark) saveAudio(voiceID string, workerID, reqID int, data []byte) (string, error) {
	// 文件名格式: {voice}_{worker}_{req}.mp3
	filename := fmt.Sprintf("%s_w%d_r%d.mp3", voiceID, workerID, reqID)
	filepath := filepath.Join(b.config.OutputDir, "audio", filename)

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return "", err
	}

	return filepath, nil
}

// reportProgress 报告进度
func (b *Benchmark) reportProgress(done chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			completed := atomic.LoadInt64(&b.completedReqs)
			active := atomic.LoadInt64(&b.activeWorkers)
			percent := float64(completed) / float64(b.totalRequests) * 100
			log.Printf("Progress: %d/%d (%.1f%%) completed, %d active workers",
				completed, b.totalRequests, percent, active)
		}
	}
}

// Stop 停止测试
func (b *Benchmark) Stop() {
	close(b.stopCh)
}

// Collector 返回指标收集器
func (b *Benchmark) Collector() *MetricsCollector {
	return b.collector
}
