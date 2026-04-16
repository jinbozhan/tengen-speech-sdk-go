// Package main TTS 管道化合成示例
//
// 验证 SDK + 服务端管道化：先快速连续提交所有文本（不等 audio.done），
// 再按序读取每个 stream 的音频。对比串行模式的端到端耗时。
//
// 使用方法:
//
//	./tts_pipeline "句子1" "句子2" "句子3"
//	./tts_pipeline -voice sw-TZ-DaudiNeural "Habari" "Halo" "Asante"
//	./tts_pipeline -mode serial "句子1" "句子2"   # 串行模式（对照组）
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/audio"
	"github.com/jinbozhan/tengen-speech-sdk-go/logging"
	"github.com/jinbozhan/tengen-speech-sdk-go/tts"
)

var (
	gatewayURL    string
	provider      string
	apiKey        string
	voiceID       string
	language      string
	speed         float64
	output        string
	sampleRate    int
	channels      int
	bitsPerSample int
	mode          string // "pipeline" 或 "serial"
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:8080", "Gateway URL")
	flag.StringVar(&provider, "provider", "tengen", "TTS provider")
	flag.StringVar(&apiKey, "apikey", "", "API Key")
	flag.StringVar(&voiceID, "voice", "sw-TZ-DaudiNeural", "Voice ID")
	flag.StringVar(&language, "language", "", "Language code (e.g. en-NG, sw-TZ)")
	flag.Float64Var(&speed, "speed", 1.0, "Speaking speed")
	flag.StringVar(&output, "output", "pipeline_output.wav", "Output WAV file")
	flag.IntVar(&sampleRate, "sample-rate", 8000, "Audio sample rate")
	flag.IntVar(&channels, "channels", 1, "Audio channels")
	flag.IntVar(&bitsPerSample, "bits", 16, "Bits per sample")
	flag.StringVar(&mode, "mode", "pipeline", "Mode: pipeline (fast) or serial (baseline)")
}

type roundResult struct {
	round     int
	text      string
	ttfb      int64 // stream.TTFB() 毫秒
	totalMs   int64
	audioSize int
	err       error
}

func main() {
	flag.Parse()
	logging.Setup(logging.LevelInfo)

	texts := flag.Args()
	if len(texts) == 0 {
		fmt.Println("Usage: tts_pipeline [options] <text1> [text2 text3 ...]")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  # 管道化（默认）：快速提交 3 句，不等 audio.done")
		fmt.Println("  ./tts_pipeline \"Habari za leo\" \"Halo mimi ni Juma\" \"Asante sana\"")
		fmt.Println()
		fmt.Println("  # 串行（对照组）：等每句完成再发下一句")
		fmt.Println("  ./tts_pipeline -mode serial \"Habari za leo\" \"Halo mimi ni Juma\"")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nCancelling...")
		cancel()
	}()

	// 创建客户端和 session
	config := &tts.Config{
		GatewayURL:  gatewayURL,
		Provider:    provider,
		APIKey:      apiKey,
		VoiceID:     voiceID,
		Language:    language,
		Speed:       speed,
		SampleRate:  sampleRate,
		AudioFormat: "pcm",
	}

	client, err := tts.NewClient(config)
	if err != nil {
		logging.Error("Failed to create client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	session, err := client.CreateSession(ctx, nil)
	if err != nil {
		logging.Error("Failed to create session", "error", err)
		os.Exit(1)
	}
	defer session.Close()
	logging.Info("Session created", "id", session.ID, "connect_ms", session.ConnectDuration().Milliseconds())

	fmt.Printf("\n=== TTS %s mode (%d rounds) ===\n\n", strings.ToUpper(mode), len(texts))

	var results []roundResult
	var allPCM []byte
	totalStart := time.Now()

	switch mode {
	case "pipeline":
		results, allPCM = runPipeline(ctx, session, texts)
	case "serial":
		results, allPCM = runSerial(ctx, session, texts)
	default:
		fmt.Printf("Unknown mode: %s (use 'pipeline' or 'serial')\n", mode)
		os.Exit(1)
	}

	totalElapsed := time.Since(totalStart)

	// 写入 WAV
	if len(allPCM) > 0 {
		if err := audio.WriteWAVFile(output, allPCM, sampleRate, channels, bitsPerSample); err != nil {
			logging.Error("Failed to write WAV", "error", err)
		} else {
			fmt.Printf("\nAudio saved: %s (%d bytes)\n", output, len(allPCM))
		}
	}

	// 打印结果
	printResults(results, totalElapsed)
}

// runPipeline 管道化模式：先快速提交所有 round，再按序读取
func runPipeline(ctx context.Context, session *tts.Session, texts []string) ([]roundResult, []byte) {
	// 阶段 1：快速提交所有 round
	streams := make([]*tts.AudioStream, len(texts))
	for i, text := range texts {
		stream, err := session.SynthesizeStream(ctx, text)
		if err != nil {
			logging.Error("SynthesizeStream failed", "round", i+1, "error", err)
			break
		}
		streams[i] = stream
		fmt.Printf("[COMMIT] round=%d text=%q pending=%d\n", i+1, truncate(text, 30), session.PendingRounds())
	}
	fmt.Printf("\nAll %d rounds committed\n\n", len(texts))

	// 阶段 2：按序读取每个 stream 的音频
	var results []roundResult
	var allPCM []byte

	for i, stream := range streams {
		if stream == nil {
			continue
		}
		start := time.Now()

		pcmData, err := io.ReadAll(stream)
		elapsed := time.Since(start).Milliseconds()

		r := roundResult{
			round:     i + 1,
			text:      texts[i],
			ttfb:      stream.TTFB(),
			totalMs:   elapsed,
			audioSize: len(pcmData),
			err:       err,
		}
		results = append(results, r)
		allPCM = append(allPCM, pcmData...)

		if err != nil {
			fmt.Printf("[ERROR]      round=%d err=%v\n", i+1, err)
		} else {
			fmt.Printf("[AUDIO_DONE] round=%d ttfb=%dms total=%dms bytes=%d\n",
				i+1, r.ttfb, r.totalMs, r.audioSize)
		}
	}

	return results, allPCM
}

// runSerial 串行模式（对照组）：等每轮完成再发下一轮
func runSerial(ctx context.Context, session *tts.Session, texts []string) ([]roundResult, []byte) {
	var results []roundResult
	var allPCM []byte

	for i, text := range texts {
		fmt.Printf("[COMMIT] round=%d text=%q\n", i+1, truncate(text, 30))
		start := time.Now()

		stream, err := session.SynthesizeStream(ctx, text)
		if err != nil {
			logging.Error("SynthesizeStream failed", "round", i+1, "error", err)
			results = append(results, roundResult{round: i + 1, text: text, err: err})
			continue
		}

		pcmData, err := io.ReadAll(stream)
		elapsed := time.Since(start).Milliseconds()

		r := roundResult{
			round:     i + 1,
			text:      text,
			ttfb:      stream.TTFB(),
			totalMs:   elapsed,
			audioSize: len(pcmData),
			err:       err,
		}
		results = append(results, r)
		allPCM = append(allPCM, pcmData...)

		if err != nil {
			fmt.Printf("[ERROR]      round=%d err=%v\n", i+1, err)
		} else {
			fmt.Printf("[AUDIO_DONE] round=%d ttfb=%dms total=%dms bytes=%d\n",
				i+1, r.ttfb, r.totalMs, r.audioSize)
		}
	}

	return results, allPCM
}

func printResults(results []roundResult, totalElapsed time.Duration) {
	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("%-6s %-30s %-10s %-10s %-10s %s\n", "Round", "Text", "TTFB(ms)", "Total(ms)", "Bytes", "Status")
	fmt.Println(strings.Repeat("-", 85))

	allOK := true
	for _, r := range results {
		status := "OK"
		if r.err != nil {
			status = r.err.Error()
			allOK = false
		}
		fmt.Printf("%-6d %-30s %-10d %-10d %-10d %s\n",
			r.round, truncate(r.text, 28), r.ttfb, r.totalMs, r.audioSize, status)
	}

	fmt.Printf("\nTotal: %dms\n", totalElapsed.Milliseconds())
	if allOK {
		fmt.Println("PASS")
	} else {
		fmt.Println("FAIL")
		os.Exit(1)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + ".."
}
