// Package main STT 客户端演示程序
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/stt"
)

const (
	// 超时
	connectTimeout = 30 * time.Second  // WebSocket 连接超时
	readTimeout    = 120 * time.Second // 读超时
	writeTimeout   = 10 * time.Second  // 写超时

	// 音频发送
	chunkDurationMs = 100                  // 每块音频时长（毫秒）
	sendInterval    = 1 * time.Millisecond // 块间发送间隔

	// WAV
	wavHeaderSize = 44 // WAV 文件头字节数
)

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
		log.Fatalf("音频文件不存在: %s", audioFile)
	}

	// 创建带取消的context
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
		log.Fatalf("创建客户端失败: %v", err)
	}
	defer client.Close()

	fmt.Printf("STT配置:\n")
	fmt.Printf("  Gateway: %s\n", gatewayURL)
	fmt.Printf("  Provider: %s\n", provider)
	fmt.Printf("  Language: %s\n", language)
	fmt.Printf("  Sample Rate: %d Hz\n", sampleRate)
	fmt.Printf("  Audio File: %s\n", audioFile)
	fmt.Println()

	start := time.Now()

	err = recognizeStreaming(ctx, client, audioFile)

	if err != nil {
		log.Fatalf("识别失败: %v", err)
	}

	elapsed := time.Since(start)
	fmt.Printf("\n识别完成! 耗时: %dms\n", elapsed.Milliseconds())
}

// recognizeStreaming 流式识别并显示部分结果
func recognizeStreaming(ctx context.Context, client *stt.Client, audioPath string) error {
	// 打开音频文件
	file, err := os.Open(audioPath)
	if err != nil {
		return fmt.Errorf("打开文件: %w", err)
	}
	defer file.Close()

	// 创建流式会话
	opts := &stt.StreamOptions{
		Language:    language,
		SampleRate:  sampleRate,
		AudioFormat: "pcm",
	}
	session, err := client.RecognizeStream(ctx, opts)
	if err != nil {
		return fmt.Errorf("创建会话: %w", err)
	}
	defer session.Close()

	// 跳过WAV头
	if strings.HasSuffix(strings.ToLower(audioPath), ".wav") {
		file.Seek(wavHeaderSize, io.SeekStart)
	}

	// 启动发送音频的goroutine
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sendAudio(session, file, sampleRate)
	}()

	// 收集最终结果
	var finalTexts []string

	// 处理识别事件
	fmt.Println("识别中...")
	for event := range session.Events() {
		switch event.Type {
		case stt.EventPartial:
			// 显示部分结果（覆盖同一行）
			fmt.Printf("\r[部分] %s", event.Text)
		case stt.EventFinal:
			// 显示最终结果（换行），包含时间戳
			fmt.Printf("\r[最终] [%.3fs - %.3fs] %s\n",
				event.StartTime.Seconds(), event.EndTime.Seconds(), event.Text)
			finalTexts = append(finalTexts, event.Text)
		case stt.EventError:
			return fmt.Errorf("识别错误: %v", event.Error)
		case stt.EventClosed:
			// 会话结束
		}
	}

	// 等待发送完成
	if err := <-sendDone; err != nil {
		return fmt.Errorf("发送音频: %w", err)
	}

	// 显示完整结果
	if len(finalTexts) > 0 {
		fmt.Println()
		fmt.Printf("TTFB: %dms\n", session.TTFB().Milliseconds())
		fmt.Println("完整识别结果:")
		fmt.Println(strings.Join(finalTexts, ""))
	}

	return nil
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
			time.Sleep(sendInterval)
		}
	}

	// 提交输入
	return session.Commit()
}
