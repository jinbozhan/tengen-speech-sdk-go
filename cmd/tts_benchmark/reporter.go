// Package main 提供TTS并发测试工具
package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Reporter 报告生成器
type Reporter struct {
	outputDir string
	timestamp string
}

// NewReporter 创建报告生成器
func NewReporter(outputDir string) *Reporter {
	return &Reporter{
		outputDir: outputDir,
		timestamp: time.Now().Format("20060102_150405"),
	}
}

// GenerateReport 生成完整报告
func (r *Reporter) GenerateReport(collector *MetricsCollector, config *BenchmarkConfig) error {
	// 确保输出目录存在
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	metrics := collector.GetAll()
	aggregated := collector.Aggregate()

	// 1. 生成摘要报告（控制台输出）
	r.printSummary(aggregated, config, collector.Duration())

	// 2. 生成详细 CSV
	if err := r.writeDetailCSV(metrics); err != nil {
		return fmt.Errorf("write detail csv: %w", err)
	}

	// 3. 生成聚合 JSON
	if err := r.writeAggregatedJSON(aggregated, config, collector.Duration()); err != nil {
		return fmt.Errorf("write aggregated json: %w", err)
	}

	fmt.Printf("\nResults saved to: %s\n", r.outputDir)

	return nil
}

// printSummary 打印摘要到控制台
func (r *Reporter) printSummary(aggregated map[string]*AggregatedMetrics, config *BenchmarkConfig, duration time.Duration) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("                        TTS Benchmark Report")
	fmt.Println(strings.Repeat("=", 80))

	fmt.Printf("\nConfiguration:\n")
	fmt.Printf("  Gateway:         %s\n", config.GatewayURL)
	fmt.Printf("  Provider:        %s\n", config.Provider)
	fmt.Printf("  Requests/Worker: %d\n", config.Requests)
	fmt.Printf("  Test Duration:   %v\n", duration.Round(time.Second))

	// 按音色输出
	for voiceID, m := range aggregated {
		if voiceID == "ALL" {
			continue
		}
		fmt.Printf("\n%s Voice: %s %s\n", strings.Repeat("-", 20), voiceID, strings.Repeat("-", 20))
		r.printVoiceMetrics(m)
	}

	// 总体统计
	if all, ok := aggregated["ALL"]; ok {
		fmt.Printf("\n%s OVERALL %s\n", strings.Repeat("=", 30), strings.Repeat("=", 30))
		r.printVoiceMetrics(all)
	}

	fmt.Println(strings.Repeat("=", 80))
}

// printVoiceMetrics 打印单个音色的指标
func (r *Reporter) printVoiceMetrics(m *AggregatedMetrics) {
	fmt.Printf("  Requests:     %d (Success: %d, Failed: %d)\n",
		m.TotalRequests, m.SuccessCount, m.FailCount)
	fmt.Printf("  Success Rate: %.2f%%\n", m.SuccessRate*100)

	if m.SuccessCount > 0 {
		fmt.Printf("\n  Connect Time (TCP+TLS+WS):\n")
		fmt.Printf("    Min:  %5d ms\n", m.ConnectMin)
		fmt.Printf("    Max:  %5d ms\n", m.ConnectMax)
		fmt.Printf("    Avg:  %5d ms\n", m.ConnectAvg)
		fmt.Printf("    P50:  %5d ms\n", m.ConnectP50)
		fmt.Printf("    P95:  %5d ms\n", m.ConnectP95)
		fmt.Printf("    P99:  %5d ms\n", m.ConnectP99)

		fmt.Printf("\n  Synthesis Time (commit->first byte):\n")
		fmt.Printf("    Min:  %5d ms\n", m.SynthesisMin)
		fmt.Printf("    Max:  %5d ms\n", m.SynthesisMax)
		fmt.Printf("    Avg:  %5d ms\n", m.SynthesisAvg)
		fmt.Printf("    P50:  %5d ms\n", m.SynthesisP50)
		fmt.Printf("    P95:  %5d ms\n", m.SynthesisP95)
		fmt.Printf("    P99:  %5d ms\n", m.SynthesisP99)

		fmt.Printf("\n  TTFB (Time To First Byte):\n")
		fmt.Printf("    Min:  %5d ms\n", m.TTFBMin)
		fmt.Printf("    Max:  %5d ms\n", m.TTFBMax)
		fmt.Printf("    Avg:  %5d ms\n", m.TTFBAvg)
		fmt.Printf("    P50:  %5d ms\n", m.TTFBP50)
		fmt.Printf("    P95:  %5d ms\n", m.TTFBP95)
		fmt.Printf("    P99:  %5d ms\n", m.TTFBP99)

		fmt.Printf("\n  Total Time:\n")
		fmt.Printf("    Min:  %5d ms\n", m.TotalTimeMin)
		fmt.Printf("    Max:  %5d ms\n", m.TotalTimeMax)
		fmt.Printf("    Avg:  %5d ms\n", m.TotalTimeAvg)
		fmt.Printf("    P50:  %5d ms\n", m.TotalTimeP50)
		fmt.Printf("    P95:  %5d ms\n", m.TotalTimeP95)
		fmt.Printf("    P99:  %5d ms\n", m.TotalTimeP99)

		fmt.Printf("\n  Throughput:\n")
		fmt.Printf("    RPS:        %.2f req/s\n", m.RPS)
		fmt.Printf("    Bandwidth:  %.2f KB/s\n", m.BytesPerSec/1024)
	}

	if len(m.ErrorCounts) > 0 {
		fmt.Printf("\n  Errors:\n")
		for errMsg, count := range m.ErrorCounts {
			fmt.Printf("    [%d] %s\n", count, errMsg)
		}
	}
}

// writeDetailCSV 写入详细 CSV 文件
func (r *Reporter) writeDetailCSV(metrics []RequestMetrics) error {
	filename := fmt.Sprintf("detail_%s.csv", r.timestamp)
	filepath := filepath.Join(r.outputDir, filename)

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 写入表头
	headers := []string{
		"voice_id", "worker_id", "request_id", "text_length",
		"connect_ms", "synthesis_ms", "ttfb_ms", "total_ms",
		"chunks", "bytes", "success", "error", "audio_file",
	}
	if err := writer.Write(headers); err != nil {
		return err
	}

	// 写入数据
	for _, m := range metrics {
		record := []string{
			m.VoiceID,
			strconv.Itoa(m.WorkerID),
			strconv.Itoa(m.RequestID),
			strconv.Itoa(m.TextLen),
			strconv.FormatInt(m.ConnectMs, 10),
			strconv.FormatInt(m.SynthesisMs, 10),
			strconv.FormatInt(m.TTFBMs, 10),
			strconv.FormatInt(m.TotalMs, 10),
			strconv.Itoa(m.ChunkCount),
			strconv.FormatInt(m.TotalBytes, 10),
			strconv.FormatBool(m.Success),
			m.Error,
			m.AudioFile,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	fmt.Printf("Detail CSV: %s\n", filepath)
	return nil
}

// SummaryReport JSON 报告结构
type SummaryReport struct {
	Config      ConfigSummary                 `json:"config"`
	DurationSec float64                       `json:"duration_sec"`
	Voices      map[string]*VoiceMetricsSummary `json:"voices"`
	Overall     *VoiceMetricsSummary          `json:"overall"`
}

// ConfigSummary 配置摘要
type ConfigSummary struct {
	Gateway    string   `json:"gateway"`
	Provider   string   `json:"provider"`
	Voices     []string `json:"voices"`
	RequestsPerWorker int `json:"requests_per_worker"`
	SaveAudio  bool     `json:"save_audio"`
}

// VoiceMetricsSummary 音色指标摘要
type VoiceMetricsSummary struct {
	TotalRequests int     `json:"total_requests"`
	SuccessCount  int     `json:"success_count"`
	FailCount     int     `json:"fail_count"`
	SuccessRate   float64 `json:"success_rate"`

	ConnectMs   *LatencyStats `json:"connect_ms"`
	SynthesisMs *LatencyStats `json:"synthesis_ms"`
	TTFBMs      *LatencyStats `json:"ttfb_ms"`
	TotalTimeMs *LatencyStats `json:"total_time_ms"`

	RPS         float64 `json:"rps"`
	BytesPerSec float64 `json:"bytes_per_sec"`

	Errors map[string]int `json:"errors,omitempty"`
}

// LatencyStats 延迟统计
type LatencyStats struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
	Avg int64 `json:"avg"`
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	P99 int64 `json:"p99"`
}

// writeAggregatedJSON 写入聚合 JSON 文件
func (r *Reporter) writeAggregatedJSON(aggregated map[string]*AggregatedMetrics, config *BenchmarkConfig, duration time.Duration) error {
	filename := fmt.Sprintf("summary_%s.json", r.timestamp)
	filepath := filepath.Join(r.outputDir, filename)

	// 构建配置摘要
	var voiceStrs []string
	for _, v := range config.Voices {
		voiceStrs = append(voiceStrs, fmt.Sprintf("%s:%d", v.DisplayID, v.Concurrency))
	}

	report := &SummaryReport{
		Config: ConfigSummary{
			Gateway:           config.GatewayURL,
			Provider:          config.Provider,
			Voices:            voiceStrs,
			RequestsPerWorker: config.Requests,
			SaveAudio:         config.SaveAudio,
		},
		DurationSec: duration.Seconds(),
		Voices:      make(map[string]*VoiceMetricsSummary),
	}

	// 转换指标
	for voiceID, m := range aggregated {
		summary := &VoiceMetricsSummary{
			TotalRequests: m.TotalRequests,
			SuccessCount:  m.SuccessCount,
			FailCount:     m.FailCount,
			SuccessRate:   m.SuccessRate,
			ConnectMs: &LatencyStats{
				Min: m.ConnectMin,
				Max: m.ConnectMax,
				Avg: m.ConnectAvg,
				P50: m.ConnectP50,
				P95: m.ConnectP95,
				P99: m.ConnectP99,
			},
			SynthesisMs: &LatencyStats{
				Min: m.SynthesisMin,
				Max: m.SynthesisMax,
				Avg: m.SynthesisAvg,
				P50: m.SynthesisP50,
				P95: m.SynthesisP95,
				P99: m.SynthesisP99,
			},
			TTFBMs: &LatencyStats{
				Min: m.TTFBMin,
				Max: m.TTFBMax,
				Avg: m.TTFBAvg,
				P50: m.TTFBP50,
				P95: m.TTFBP95,
				P99: m.TTFBP99,
			},
			TotalTimeMs: &LatencyStats{
				Min: m.TotalTimeMin,
				Max: m.TotalTimeMax,
				Avg: m.TotalTimeAvg,
				P50: m.TotalTimeP50,
				P95: m.TotalTimeP95,
				P99: m.TotalTimeP99,
			},
			RPS:         m.RPS,
			BytesPerSec: m.BytesPerSec,
		}

		if len(m.ErrorCounts) > 0 {
			summary.Errors = m.ErrorCounts
		}

		if voiceID == "ALL" {
			report.Overall = summary
		} else {
			report.Voices[voiceID] = summary
		}
	}

	// 写入文件
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return err
	}

	fmt.Printf("Summary JSON: %s\n", filepath)
	return nil
}
