// Package main STT 客户端演示程序
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

	"github.com/jinbozhan/tengen-speech-sdk-go/logging"
	"github.com/jinbozhan/tengen-speech-sdk-go/stt"
)

const wavHeaderSize = 44 // WAV 文件头字节数

var (
	gatewayURL string
	provider   string
	apiKey     string
	language   string
	sampleRate int
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:8080", "Gateway URL")
	flag.StringVar(&provider, "provider", "azure", "STT provider (azure, qwen)")
	flag.StringVar(&apiKey, "apikey", "", "API Key for authentication")
	flag.StringVar(&language, "language", "zh-CN", "Recognition language")
	flag.IntVar(&sampleRate, "sample-rate", 8000, "Audio sample rate")
}

func main() {
	flag.Parse()
	logging.Setup(logging.LevelInfo)

	// 获取音频文件路径
	audioFile := flag.Arg(0)
	if audioFile == "" {
		fmt.Println("Usage: stt_demo [options] <audio_file>")
		fmt.Println()
		fmt.Println("Options:")
		flag.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  stt_demo audio.wav")
		fmt.Println("  stt_demo -provider qwen -language en-US recording.wav")
		os.Exit(1)
	}

	// 检查文件是否存在
	if _, err := os.Stat(audioFile); os.IsNotExist(err) {
		logging.Error("Audio file not found", "file", audioFile)
		os.Exit(1)
	}

	// 创建带取消的context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 处理中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nCancelling...")
		cancel()
	}()

	// 创建STT配置
	config := &stt.Config{
		GatewayURL:  gatewayURL,
		Provider:    provider,
		APIKey:      apiKey,
		Language:    language,
		SampleRate:  sampleRate,
		AudioFormat: "pcm",
	}

	// 创建客户端
	client, err := stt.NewClient(config)
	if err != nil {
		logging.Error("Failed to create client", "error", err)
		os.Exit(1)
	}
	defer client.Close()

	// 创建流式会话
	opts := &stt.StreamOptions{
		Language:    language,
		SampleRate:  sampleRate,
		AudioFormat: "pcm",
	}
	session, err := client.CreateSession(ctx, opts)
	if err != nil {
		logging.Error("Failed to create session", "error", err)
		os.Exit(1)
	}
	defer session.Close()
	logging.Info("Session created", "id", session.ID, "connect_duration_ms", session.ConnectDuration().Milliseconds())

	// 打开音频文件
	file, err := os.Open(audioFile)
	if err != nil {
		logging.Error("Failed to open file", "error", err)
		os.Exit(1)
	}
	defer file.Close()

	// 跳过WAV头
	if strings.HasSuffix(strings.ToLower(audioFile), ".wav") {
		file.Seek(wavHeaderSize, io.SeekStart)
	}

	start := time.Now()

	// 流式识别: 从文件发送音频，EndInput 后接收事件
	go func() {
		chunkSize := sampleRate * 2 / 10 // 100ms
		buf := make([]byte, chunkSize)
		for {
			n, err := file.Read(buf)
			if err == io.EOF {
				break
			}
            // 发送数据分块
			if n > 0 {
				session.Send(buf[:n])
                // 模拟实时速度
                time.Sleep(100 * time.Millisecond)
			}
		}
		// 数据结束：EndInput发送session.end到服务端
		session.EndInput()
	}()

	finalTexts, err := recognizeStreaming(session.Events())
	if err != nil {
		logging.Error("Recognition failed", "error", err)
		os.Exit(1)
	}

	elapsed := time.Since(start)

	// 显示完整结果
	if len(finalTexts) > 0 {
		logging.Info("TTFB", "ms", session.TTFB().Milliseconds())
		logging.Info("Final result", "text", strings.Join(finalTexts, ""))
	}

	logging.Info("Recognition complete", "duration_ms", elapsed.Milliseconds())
}

// recognizeStreaming 流式识别，接收事件并返回最终识别文本
func recognizeStreaming(events <-chan *stt.RecognitionEvent) ([]string, error) {
	var finalTexts []string

loop:
	for event := range events {
		switch event.Type {
		case stt.EventTranscriptPartial:
			logging.Info("[Partial]", "text", event.Text)

		case stt.EventTranscriptFinal:
			logging.Info("[Final]", "start", event.StartTime.Seconds(), "end", event.EndTime.Seconds(), "text", event.Text)
			finalTexts = append(finalTexts, event.Text)

		case stt.EventSpeechStarted:
			logging.Info("[Speech] speech.started")

		// 活动心跳静默忽略
		case stt.EventProcessing:

		case stt.EventSessionEnded:
			break loop

		case stt.EventError:
			logging.Error("Recognition error", "error", event.Error)
		}
	}

	return finalTexts, nil
}
