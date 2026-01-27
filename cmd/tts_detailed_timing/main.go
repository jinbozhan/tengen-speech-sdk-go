// Package main 提供TTS详细时延分析工具
//
// 用于分析单个TTS请求的详细时间戳，帮助识别性能瓶颈
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/tts"
)

// 颜色代码
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
)

func main() {
	var (
		gateway    string
		provider   string
		apiKey     string
		voice      string
		text       string
		iterations int
	)

	flag.StringVar(&gateway, "gateway", "ws://localhost:7861", "Gateway WebSocket URL")
	flag.StringVar(&provider, "provider", "qwen_realtime", "TTS provider (qwen_realtime, azure)")
	flag.StringVar(&apiKey, "api-key", os.Getenv("GATEWAY_API_KEY"), "API Key")
	flag.StringVar(&voice, "voice", "en-NG-OkunNeutral", "Voice ID (Qwen: en-NG-OkunNeutral, Azure: en-NG-EzinneNeural)")
	// 默认使用尼日利亚英语(en-NG)文本
	flag.StringVar(&text, "text", "The development of artificial intelligence has transformed the way we interact with technology in our daily lives.", "Text to synthesize")
	flag.IntVar(&iterations, "iterations", 1, "Number of iterations to run")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "TTS Detailed Timing - TTS请求时延详细分析工具\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n")
		fmt.Fprintf(os.Stderr, "  %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "示例:\n")
		fmt.Fprintf(os.Stderr, "  # 测试Qwen Realtime\n")
		fmt.Fprintf(os.Stderr, "  %s -provider qwen_realtime -voice en-NG-OkunNeutral -text \"Hello world\" -iterations 3\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 测试Azure TTS\n")
		fmt.Fprintf(os.Stderr, "  %s -provider azure -voice en-NG-EzinneNeural -iterations 5\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// 检查参数
	if text == "" {
		log.Fatal("Text cannot be empty")
	}

	fmt.Printf("\n%s========================================%s\n", colorCyan, colorReset)
	fmt.Printf("%s  TTS Detailed Timing Analysis%s\n", colorCyan, colorReset)
	fmt.Printf("%s========================================%s\n", colorCyan, colorReset)
	fmt.Printf("Gateway:    %s\n", gateway)
	fmt.Printf("Provider:   %s\n", provider)
	fmt.Printf("Voice:      %s\n", voice)
	fmt.Printf("Text:       %s\n", truncateText(text, 60))
	fmt.Printf("Iterations: %d\n", iterations)
	fmt.Printf("%s========================================%s\n\n", colorCyan, colorReset)

	// 运行测试
	var totalTTFB, totalConnect, totalSynthesis, totalTime int64
	successCount := 0

	for i := 0; i < iterations; i++ {
		fmt.Printf("%s[Iteration %d/%d]%s\n", colorYellow, i+1, iterations, colorReset)

		result, err := runSingleRequest(gateway, provider, apiKey, voice, text)
		if err != nil {
			fmt.Printf("%s✗ Error: %v%s\n\n", colorRed, err, colorReset)
			continue
		}

		// 打印详细时间戳
		printDetailedTimings(result)

		// 累计统计
		totalTTFB += result.TTFBMs
		totalConnect += result.ConnectMs
		totalSynthesis += result.SynthesisMs
		totalTime += result.TotalMs
		successCount++

		if i < iterations-1 {
			fmt.Println()
			time.Sleep(500 * time.Millisecond) // 请求间隔
		}
	}

	// 打印汇总统计
	if successCount > 0 {
		fmt.Printf("\n%s========================================%s\n", colorCyan, colorReset)
		fmt.Printf("%s  Summary Statistics (%d successful)%s\n", colorCyan, successCount, colorReset)
		fmt.Printf("%s========================================%s\n", colorCyan, colorReset)
		fmt.Printf("Average TTFB:       %s%6d ms%s\n", colorGreen, totalTTFB/int64(successCount), colorReset)
		fmt.Printf("Average Connect:    %s%6d ms%s\n", colorBlue, totalConnect/int64(successCount), colorReset)
		fmt.Printf("Average Synthesis:  %s%6d ms%s\n", colorYellow, totalSynthesis/int64(successCount), colorReset)
		fmt.Printf("Average Total:      %s%6d ms%s\n", colorCyan, totalTime/int64(successCount), colorReset)
		fmt.Printf("%s========================================%s\n\n", colorCyan, colorReset)
	}
}

// RequestResult 请求结果
type RequestResult struct {
	StartTime    time.Time
	ConnectedAt  time.Time
	CommitSentAt time.Time
	FirstByteAt  time.Time
	CompleteAt   time.Time

	ConnectMs   int64
	TTFBMs      int64
	SynthesisMs int64
	TotalMs     int64
	TotalBytes  int64
	ChunkCount  int
}

// runSingleRequest 执行单次请求
func runSingleRequest(gateway, provider, apiKey, voice, text string) (*RequestResult, error) {
	ctx := context.Background()
	result := &RequestResult{
		StartTime: time.Now(),
	}

	// 创建客户端
	config := &tts.Config{
		GatewayURL:     gateway,
		Provider:       provider,
		APIKey:         apiKey,
		VoiceID:        voice,
		Speed:          1.0,
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	client, err := tts.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	defer client.Close()

	// 创建流式会话
	stream, err := client.SynthesizeStream(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("synthesize: %w", err)
	}
	defer stream.Close()

	// 获取时间戳
	result.ConnectedAt = stream.ConnectedAt()
	result.CommitSentAt = stream.CommitSentAt()
	result.ConnectMs = stream.ConnectDuration().Milliseconds()

	// 接收音频数据
	buf := make([]byte, 4096)
	firstChunk := true

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("read: %w", err)
		}

		if n > 0 {
			if firstChunk {
				// 优先使用 session 记录的精确首包时间
				result.FirstByteAt = stream.FirstChunkReceivedAt()
				if result.FirstByteAt.IsZero() {
					// 回退到应用层时间
					result.FirstByteAt = time.Now()
				}

				// 使用精确的 TTFB（从 commit 到首包）
				result.SynthesisMs = stream.TTFB()
				if result.SynthesisMs == 0 && !result.CommitSentAt.IsZero() {
					result.SynthesisMs = result.FirstByteAt.Sub(result.CommitSentAt).Milliseconds()
				}

				// TTFBMs 保持从 StartTime 到 FirstByteAt（用于兼容性对比）
				result.TTFBMs = result.FirstByteAt.Sub(result.StartTime).Milliseconds()
				firstChunk = false
			}
			result.ChunkCount++
			result.TotalBytes += int64(n)
		}
	}

	result.CompleteAt = time.Now()
	result.TotalMs = result.CompleteAt.Sub(result.StartTime).Milliseconds()

	// 检查流错误
	if streamErr := stream.Error(); streamErr != nil {
		return nil, streamErr
	}

	return result, nil
}

// printDetailedTimings 打印详细时间戳
func printDetailedTimings(result *RequestResult) {
	fmt.Printf("  %s[START]%s        %s (baseline)\n",
		colorBlue, colorReset, result.StartTime.Format("15:04:05.000"))

	if !result.ConnectedAt.IsZero() {
		relativeMs := result.ConnectedAt.Sub(result.StartTime).Milliseconds()
		fmt.Printf("  %s[CONNECTED]%s    %s (+%dms) - WebSocket建连完成\n",
			colorBlue, colorReset, result.ConnectedAt.Format("15:04:05.000"), relativeMs)
	}

	if !result.CommitSentAt.IsZero() {
		relativeMs := result.CommitSentAt.Sub(result.StartTime).Milliseconds()
		fmt.Printf("  %s[COMMIT_SENT]%s  %s (+%dms) - 文本提交发送\n",
			colorYellow, colorReset, result.CommitSentAt.Format("15:04:05.000"), relativeMs)
	}

	if !result.FirstByteAt.IsZero() {
		relativeMs := result.FirstByteAt.Sub(result.StartTime).Milliseconds()
		fmt.Printf("  %s[FIRST_CHUNK]%s  %s (+%dms) - 首个音频块收到\n",
			colorGreen, colorReset, result.FirstByteAt.Format("15:04:05.000"), relativeMs)
	}

	if !result.CompleteAt.IsZero() {
		relativeMs := result.CompleteAt.Sub(result.StartTime).Milliseconds()
		fmt.Printf("  %s[COMPLETE]%s     %s (+%dms) - 合成完成\n",
			colorCyan, colorReset, result.CompleteAt.Format("15:04:05.000"), relativeMs)
	}

	// 打印关键指标
	fmt.Printf("\n  %s关键指标:%s\n", colorCyan, colorReset)
	fmt.Printf("    • Connect Time:    %s%6d ms%s (TCP+TLS+WebSocket握手)\n",
		colorBlue, result.ConnectMs, colorReset)
	fmt.Printf("    • Synthesis Time:  %s%6d ms%s (commit发送到首包)\n",
		colorYellow, result.SynthesisMs, colorReset)
	fmt.Printf("    • TTFB:            %s%6d ms%s (请求开始到首包)\n",
		colorGreen, result.TTFBMs, colorReset)
	fmt.Printf("    • Total Time:      %s%6d ms%s (完整合成时间)\n",
		colorCyan, result.TotalMs, colorReset)
	fmt.Printf("    • Total Bytes:     %d bytes (%d chunks)\n",
		result.TotalBytes, result.ChunkCount)

	// 时间分解
	fmt.Printf("\n  %s时间分解:%s\n", colorCyan, colorReset)
	configMs := result.CommitSentAt.Sub(result.ConnectedAt).Milliseconds()
	if configMs > 0 {
		fmt.Printf("    • 建连阶段:        %6d ms (%.1f%%)\n",
			result.ConnectMs, float64(result.ConnectMs)/float64(result.TTFBMs)*100)
		fmt.Printf("    • 配置阶段:        %6d ms (%.1f%%) (session.config + text.append)\n",
			configMs, float64(configMs)/float64(result.TTFBMs)*100)
		fmt.Printf("    • 合成阶段:        %6d ms (%.1f%%) (commit到首包)\n",
			result.SynthesisMs, float64(result.SynthesisMs)/float64(result.TTFBMs)*100)
	}
}

// truncateText 截断文本
func truncateText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
