// Package main TTS 多轮流式合成示例
//
// 本示例展示在单个 WebSocket 连接内连续合成多段文本：
//   1. 建立 WebSocket 连接并创建 Session
//   2. 循环合成多段文本，复用同一个连接
//   3. 展示连接复用带来的性能提升
//
// WebSocket 协议流程:
//   建立连接 → session.ready → session.config
//   ├─ Round 1: text.append → input.commit → audio.delta... → audio.done
//   ├─ Round 2: text.append → input.commit → audio.delta... → audio.done
//   └─ Round N: text.append → input.commit → audio.delta... → audio.done
//   关闭连接 → session.end
//
// 使用方法:
//   ./tts_stream "第一句" "第二句" "第三句"
//   ./tts_stream -voice loongstella "你好" "世界"
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

	"github.com/jinbozhan/tengen-speech-sdk-go/tts"
)

var (
	gatewayURL string
	provider   string
	apiKey     string
	voiceID    string
	speed      float64
	pitch      float64
	volume     float64
	output     string
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:8080", "Gateway URL")
	flag.StringVar(&provider, "provider", "qwen_realtime", "TTS provider")
	flag.StringVar(&apiKey, "apikey", "", "API Key")
	flag.StringVar(&voiceID, "voice", "loongstella", "Voice ID")
	flag.Float64Var(&speed, "speed", 1.0, "Speaking speed (0.5-2.0)")
	flag.Float64Var(&pitch, "pitch", 1.0, "Voice pitch (0.5-2.0)")
	flag.Float64Var(&volume, "volume", 1.0, "Audio volume (0.0-1.0)")
	flag.StringVar(&output, "output", "output.pcm", "Output PCM file")
}

func main() {
	flag.Parse()

	texts := flag.Args()
	if len(texts) == 0 {
		printUsage()
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

	// 打印配置
	printConfig(texts)

	// 创建客户端
	config := &tts.Config{
		GatewayURL:     gatewayURL,
		Provider:       provider,
		APIKey:         apiKey,
		VoiceID:        voiceID,
		Speed:          speed,
		Pitch:          pitch,
		Volume:         volume,
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	client, err := tts.NewClient(config)
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	// 创建 Session（可复用资源）
	fmt.Println("正在建立 WebSocket 连接...")
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

	// 写入 PCM 文件
	if err := os.WriteFile(output, allPCMData, 0644); err != nil {
		log.Fatalf("写入文件失败: %v", err)
	}
	fmt.Printf("\n音频已保存到: %s (%d bytes)\n", output, len(allPCMData))

	// 打印统计
	printSummary(results)
}

func printUsage() {
	fmt.Println("TTS 多轮流式合成示例")
	fmt.Println()
	fmt.Println("Usage: tts_stream [options] <text1> [text2 text3 ...]")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  tts_stream \"你好，世界\"")
	fmt.Println("  tts_stream \"第一句\" \"第二句\" \"第三句\"")
	fmt.Println("  tts_stream -voice loongstella \"测试\" \"文本\"")
}

func printConfig(texts []string) {
	fmt.Println("==========================================")
	fmt.Println("  TTS 多轮流式合成")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Printf("配置: provider=%s, voice=%s, speed=%.1f\n", provider, voiceID, speed)
	fmt.Printf("文本: %d 段\n", len(texts))
	fmt.Println()
}

// RoundResult 单轮合成结果
type RoundResult struct {
	Round     int
	Text      string
	TTFB      time.Duration
	TotalTime time.Duration
	AudioSize int
}

// synthesizeStream 使用 Session 合成单段文本
func synthesizeStream(ctx context.Context, session *tts.Session, round int, text string) (RoundResult, []byte, error) {
	fmt.Printf("------------------------------------------\n")
	fmt.Printf("Round %d: \"%s\"\n", round, truncate(text, 30))
	fmt.Println("------------------------------------------")

	start := time.Now()

	// 调用 Session 的 SynthesizeStream 方法
	// 这会通过同一个 WebSocket 连接发送:
	//   - text.append
	//   - input.commit
	stream, err := session.SynthesizeStream(ctx, text)
	if err != nil {
		return RoundResult{}, nil, err
	}

	// 读取音频数据
	var pcmData []byte
	var firstChunkTime time.Time
	buf := make([]byte, 4096)
	chunkCount := 0

	for {
		n, err := stream.Read(buf)
		if n > 0 {
			if firstChunkTime.IsZero() {
				firstChunkTime = time.Now()
			}
			pcmData = append(pcmData, buf[:n]...)
			chunkCount++
			fmt.Printf("\r[audio.delta #%d] +%d bytes, 累计: %d bytes", chunkCount, n, len(pcmData))
		}
		if err == io.EOF {
			// 收到 audio.done
			break
		}
		if err != nil {
			return RoundResult{}, nil, err
		}
	}

	fmt.Println()
	fmt.Println("[audio.done] 合成完成")

	// 计算 TTFB
	var ttfb time.Duration
	if !firstChunkTime.IsZero() {
		ttfb = firstChunkTime.Sub(start)
	}
	totalTime := time.Since(start)

	fmt.Printf("TTFB: %dms, Total: %dms, Size: %d bytes\n", ttfb.Milliseconds(), totalTime.Milliseconds(), len(pcmData))

	return RoundResult{
		Round:     round,
		Text:      text,
		TTFB:      ttfb,
		TotalTime: totalTime,
		AudioSize: len(pcmData),
	}, pcmData, nil
}

func printSummary(results []RoundResult) {
	if len(results) <= 1 {
		return
	}

	fmt.Println()
	fmt.Println("==========================================")
	fmt.Println("Summary")
	fmt.Println("==========================================")

	for _, r := range results {
		fmt.Printf("Round %d: TTFB %dms\n", r.Round, r.TTFB.Milliseconds())
	}
	fmt.Println("==========================================")
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
