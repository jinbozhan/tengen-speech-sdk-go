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

const (
	// 超时
	connectTimeout = 30 * time.Second  // WebSocket 连接超时
	readTimeout    = 120 * time.Second // 读超时
	writeTimeout   = 10 * time.Second  // 写超时

	// 音频发送
	chunkDurationMs = 100 // 每块音频时长（毫秒）

	// WAV
	wavHeaderSize = 44 // WAV 文件头字节数
)

var (
	gatewayURL   string
	provider     string
	apiKey       string
	language     string
	sampleRate   int
	sendInterval int
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:8080", "Gateway URL")
	flag.StringVar(&provider, "provider", "azure", "STT provider (azure, qwen)")
	flag.StringVar(&apiKey, "apikey", "", "API Key for authentication")
	flag.StringVar(&language, "language", "zh-CN", "Recognition language")
	flag.IntVar(&sampleRate, "sample-rate", 8000, "Audio sample rate")
	flag.IntVar(&sendInterval, "send-interval", chunkDurationMs, "Send interval in ms (default matches chunk duration for real-time simulation)")
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
		GatewayURL:     gatewayURL,
		Provider:       provider,
		APIKey:         apiKey,
		Language:       language,
		SampleRate:     sampleRate,
		AudioFormat:    "pcm",
		ConnectTimeout: connectTimeout,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
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
	session, err := client.RecognizeStream(ctx, opts)
	if err != nil {
		logging.Error("Failed to create session", "error", err)
		os.Exit(1)
	}
	defer session.Close()

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

	// 启动发送音频的goroutine
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sendAudio(session, file, sampleRate)
	}()

	start := time.Now()

	// 流式识别
	finalTexts, err := recognizeStreaming(session)
	if err != nil {
		logging.Error("Recognition failed", "error", err)
		os.Exit(1)
	}

	// 等待发送完成
	if err := <-sendDone; err != nil {
		logging.Error("Failed to send audio", "error", err)
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
func recognizeStreaming(session *stt.Session) ([]string, error) {
	var finalTexts []string

loop:
	for event := range session.Events() {
		switch event.Type {
		case stt.EventPartial:
			logging.Info("[Partial]", "text", event.Text)

		case stt.EventFinal:
			logging.Info("[Final]", "start", event.StartTime.Seconds(), "end", event.EndTime.Seconds(), "text", event.Text)
			finalTexts = append(finalTexts, event.Text)

		case stt.EventSpeechStarted:
			logging.Info("[Speech] speech.started")

		// 活动心跳静默忽略
		case stt.EventProcessing:

		case stt.EventInputDone:
			break loop

		case stt.EventClosed:
			break loop

		case stt.EventError:
			logging.Error("Recognition error", "error", event.Error)
		}
	}

	return finalTexts, nil
}

// sendAudio 发送音频数据到会话
func sendAudio(session *stt.Session, reader io.Reader, sampleRate int) error {
	// 100ms音频块 @ sampleRate, 16-bit
	chunkSize := sampleRate * 2 * chunkDurationMs / 1000
	buf := make([]byte, chunkSize)

	for {
		n, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if n > 0 {
			if err := session.Send(buf[:n]); err != nil {
				return err
			}
			// 控制发送节奏
			time.Sleep(time.Duration(sendInterval) * time.Millisecond)
		}
	}

	// 提交输入
	return session.Commit()
}
