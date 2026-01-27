// Package main TTS并发测试工具
//
// 用法:
//
//	./tts_benchmark \
//	  -gateway "ws://localhost:7861" \
//	  -api-key "YOUR_API_KEY" \
//	  -voices "en-NG-RoseSerious:40,en-NG-OkunSerious:40" \
//	  -requests 50 \
//	  -save-audio
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	// 命令行参数
	var (
		gateway     string
		provider    string
		apiKey      string
		voiceConfig string
		requests    int
		rampUp      time.Duration
		outputDir   string
		saveAudio   bool
		verbose     bool
	)

	flag.StringVar(&gateway, "gateway", "ws://localhost:7861", "Gateway WebSocket URL")
	flag.StringVar(&provider, "provider", "tengen", "TTS provider (tengen, qwen_realtime)")
	flag.StringVar(&apiKey, "api-key", os.Getenv("GATEWAY_API_KEY"), "API Key for authentication")
	flag.StringVar(&voiceConfig, "voices", "en-NG-RoseSerious:40,en-NG-OkunSerious:40",
		"Voice configuration (format: voice1:concurrency1,voice2:concurrency2)")
	flag.IntVar(&requests, "requests", 50, "Number of requests per worker")
	flag.DurationVar(&rampUp, "rampup", 5*time.Second, "Ramp-up time for workers")
	flag.StringVar(&outputDir, "output", "./benchmark_results", "Output directory for results")
	flag.BoolVar(&saveAudio, "save-audio", false, "Save synthesized audio files")
	flag.BoolVar(&verbose, "verbose", false, "Verbose logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "TTS Benchmark - Gateway TTS 并发测试工具\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n")
		fmt.Fprintf(os.Stderr, "  %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "示例:\n")
		fmt.Fprintf(os.Stderr, "  # 快速冒烟测试\n")
		fmt.Fprintf(os.Stderr, "  %s -voices \"en-NG-RoseSerious:2,en-NG-OkunSerious:2\" -requests 5\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 完整并发测试 (80并发，每worker 50请求)\n")
		fmt.Fprintf(os.Stderr, "  %s -voices \"en-NG-RoseSerious:40,en-NG-OkunSerious:40\" -requests 50\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 保存音频文件\n")
		fmt.Fprintf(os.Stderr, "  %s -save-audio -output ./results\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// 解析 voice 配置
	voices, err := parseVoiceConfig(voiceConfig)
	if err != nil {
		log.Fatalf("Invalid voice config: %v", err)
	}

	// 检查 API Key
	if apiKey == "" {
		log.Println("Warning: No API key provided. Set -api-key or GATEWAY_API_KEY environment variable.")
	}

	config := &BenchmarkConfig{
		GatewayURL: gateway,
		Provider:   provider,
		APIKey:     apiKey,
		Voices:     voices,
		Requests:   requests,
		RampUp:     rampUp,
		OutputDir:  outputDir,
		SaveAudio:  saveAudio,
		Verbose:    verbose,
	}

	// 打印配置
	printConfig(config)

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	benchmark := NewBenchmark(config)

	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, stopping gracefully...")
		benchmark.Stop()
		cancel()
	}()

	// 执行测试
	startTime := time.Now()

	if err := benchmark.Run(ctx); err != nil {
		log.Fatalf("Benchmark failed: %v", err)
	}

	elapsed := time.Since(startTime)
	log.Printf("Benchmark completed in %v", elapsed.Round(time.Second))

	// 生成报告
	reporter := NewReporter(config.OutputDir)
	if err := reporter.GenerateReport(benchmark.Collector(), config); err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}
}

// parseVoiceConfig 解析音色配置字符串
// 格式: "voice1:concurrency1,voice2:concurrency2"
func parseVoiceConfig(s string) ([]VoiceConfig, error) {
	var voices []VoiceConfig

	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		parts := strings.Split(part, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format: %s (expected voice:concurrency)", part)
		}

		voiceID := strings.TrimSpace(parts[0])
		if voiceID == "" {
			return nil, fmt.Errorf("empty voice ID in: %s", part)
		}

		var concurrency int
		if _, err := fmt.Sscanf(parts[1], "%d", &concurrency); err != nil {
			return nil, fmt.Errorf("invalid concurrency: %s", parts[1])
		}

		if concurrency <= 0 {
			return nil, fmt.Errorf("concurrency must be positive: %d", concurrency)
		}

		voices = append(voices, VoiceConfig{
			DisplayID:   voiceID,
			Concurrency: concurrency,
		})
	}

	if len(voices) == 0 {
		return nil, fmt.Errorf("no voices configured")
	}

	return voices, nil
}

// printConfig 打印配置信息
func printConfig(config *BenchmarkConfig) {
	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("       TTS Benchmark Configuration")
	fmt.Println("==========================================")
	fmt.Printf("  Gateway:     %s\n", config.GatewayURL)
	fmt.Printf("  Provider:    %s\n", config.Provider)
	fmt.Printf("  API Key:     %s\n", maskAPIKey(config.APIKey))
	fmt.Printf("  Requests:    %d per worker\n", config.Requests)
	fmt.Printf("  Ramp-up:     %v\n", config.RampUp)
	fmt.Printf("  Output:      %s\n", config.OutputDir)
	fmt.Printf("  Save Audio:  %v\n", config.SaveAudio)
	fmt.Println("  Voices:")

	total := 0
	totalReqs := 0
	for _, v := range config.Voices {
		fmt.Printf("    - %s: %d concurrent\n", v.DisplayID, v.Concurrency)
		total += v.Concurrency
		totalReqs += v.Concurrency * config.Requests
	}
	fmt.Printf("  Total Concurrency: %d\n", total)
	fmt.Printf("  Total Requests:    %d\n", totalReqs)
	fmt.Println("==========================================")
	fmt.Println()
}

// maskAPIKey 遮蔽 API Key
func maskAPIKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
