// Package main TTS 多轮合成示例
//
// 展示 Session 作为可复用资源的使用方式。
//
// 使用方法:
//
//	./tts_stream "第一句" "第二句" "第三句"
//	./tts_stream -voice en-NG-RoseNeutral "Hello" "World"
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/audio"
	"github.com/jinbozhan/tengen-speech-sdk-go/tts"
)

var (
	gatewayURL    string
	provider      string
	apiKey        string
	voiceID       string
	language      string
	speed         float64
	volume        float64
	output        string
	sampleRate    int
	channels      int
	bitsPerSample int
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:8080", "Gateway URL")
	flag.StringVar(&provider, "provider", "qwen_realtime", "TTS provider")
	flag.StringVar(&apiKey, "apikey", "", "API Key")
	flag.StringVar(&voiceID, "voice", "loongstella", "Voice ID")
	flag.StringVar(&language, "language", "", "Language code for text normalization (e.g. en-NG, sw-TZ)")
	flag.Float64Var(&speed, "speed", 1.0, "Speaking speed (0.5-2.0)")
	flag.Float64Var(&volume, "volume", 1.0, "Volume (0.0-1.0)")
	flag.StringVar(&output, "output", "output.wav", "Output WAV file")
	flag.IntVar(&sampleRate, "sample-rate", 8000, "Audio sample rate")
	flag.IntVar(&channels, "channels", 1, "Audio channels")
	flag.IntVar(&bitsPerSample, "bits", 16, "Bits per sample")
}

func main() {
	flag.Parse()

	texts := flag.Args()
	if len(texts) == 0 {
		fmt.Println("Usage: tts_stream [options] <text1> [text2 text3 ...]")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  tts_stream \"你好，世界\"")
		fmt.Println("  tts_stream \"第一句\" \"第二句\" \"第三句\"")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在取消...")
		cancel()
	}()

	// 创建客户端
	config := &tts.Config{
		GatewayURL:     gatewayURL,
		Provider:       provider,
		APIKey:         apiKey,
		VoiceID:        voiceID,
		Language:       language,
		Speed:          speed,
		Volume:         volume,
		SampleRate:     sampleRate,
		AudioFormat:    "pcm",
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	client, err := tts.NewClient(config)
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	fmt.Printf("TTS 配置: provider=%s, voice=%s, speed=%.1f, volume=%.1f\n\n", provider, voiceID, speed, volume)

	// 创建 Session（可复用资源）
	session, err := client.CreateSession(ctx, nil)
	if err != nil {
		log.Fatalf("创建会话失败: %v", err)
	}
	defer session.Close()

	fmt.Printf("Session 已创建: id=%s, 建连耗时=%dms\n\n", session.ID, session.ConnectDuration().Milliseconds())

	// 多轮合成，复用同一个 Session
	var allPCMData []byte
	var results []RoundResult

	for i, text := range texts {
		result, pcmData, err := synthesizeStream(ctx, session, i+1, text)
		if err != nil {
			log.Fatalf("第 %d 轮合成失败: %v", i+1, err)
		}
		results = append(results, result)
		allPCMData = append(allPCMData, pcmData...)
	}

	// 写入 WAV 文件
	if err := audio.WriteWAVFile(output, allPCMData, sampleRate, channels, bitsPerSample); err != nil {
		log.Fatalf("写入WAV失败: %v", err)
	}
	fmt.Printf("\n音频已保存到: %s (%d bytes)\n", output, len(allPCMData))

	// 打印统计
	printSummary(results)
}

// RoundResult 单轮合成结果
type RoundResult struct {
	Round     int
	Text      string
	TTFB      time.Duration
	TotalTime time.Duration
	AudioSize int
}

// synthesizeStream 使用 Session 合成单段文本，返回结果和 PCM 数据
func synthesizeStream(ctx context.Context, session *tts.Session, round int, text string) (RoundResult, []byte, error) {
	fmt.Printf("第 %d 轮: \"%s\"\n", round, truncate(text, 30))

	start := time.Now()

	// 调用 Session 的 SynthesizeStream 方法
	stream, err := session.SynthesizeStream(ctx, text)
	if err != nil {
		return RoundResult{}, nil, err
	}

	// 读取音频数据
	var pcmData []byte
	var firstChunkTime time.Time
	buf := make([]byte, 4096)

	for {
		n, err := stream.Read(buf)
		if n > 0 {
			if firstChunkTime.IsZero() {
				firstChunkTime = time.Now()
			}
			pcmData = append(pcmData, buf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return RoundResult{}, nil, err
		}
	}

	// 计算 TTFB
	var ttfb time.Duration
	if !firstChunkTime.IsZero() {
		ttfb = firstChunkTime.Sub(start)
	}
	totalTime := time.Since(start)

	fmt.Printf("  TTFB: %dms, Total: %dms, Size: %d bytes\n", ttfb.Milliseconds(), totalTime.Milliseconds(), len(pcmData))

	return RoundResult{
		Round:     round,
		Text:      text,
		TTFB:      ttfb,
		TotalTime: totalTime,
		AudioSize: len(pcmData),
	}, pcmData, nil
}

// printSummary 打印合成统计
func printSummary(results []RoundResult) {
	if len(results) <= 1 {
		return
	}

	fmt.Println()
	fmt.Println("=========================================")
	fmt.Println("Summary")
	fmt.Println("=========================================")

	for _, r := range results {
		fmt.Printf("Round %d: TTFB %dms\n", r.Round, r.TTFB.Milliseconds())
	}
	fmt.Println("=========================================")
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
