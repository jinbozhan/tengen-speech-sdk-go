// Package main STT 流式识别示例
//
// 本示例展示完整的 WebSocket 流式语音识别流程：
//   1. 建立 WebSocket 连接 (/ws/stt?provider=xxx)
//   2. 等待 session.ready 消息，获取会话ID
//   3. 发送 session.config 配置语言、采样率、VAD等参数
//   4. 分块发送 audio.append 消息（音频数据 base64 编码）
//   5. 实时接收 transcript.partial 部分识别结果
//   6. 发送 input.commit 标记音频结束
//   7. 接收 transcript.final 最终识别结果
//   8. 发送 session.end 关闭会话
//
// 使用方法:
//   ./stt_stream audio.wav
//   ./stt_stream -provider azure -language zh-CN audio.wav
//   ./stt_stream -realtime audio.wav  # 模拟实时发送速度
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

// 命令行参数
var (
	gatewayURL string
	provider   string
	apiKey     string
	language   string
	sampleRate int
	enableVAD  bool
	realtime   bool // 是否模拟实时发送速度
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:8080", "Gateway WebSocket URL")
	flag.StringVar(&provider, "provider", "azure", "STT provider (azure, qwen)")
	flag.StringVar(&apiKey, "apikey", "", "API Key for authentication")
	flag.StringVar(&language, "language", "zh-CN", "Recognition language (zh-CN, en-US)")
	flag.IntVar(&sampleRate, "sample-rate", 16000, "Audio sample rate in Hz")
	flag.BoolVar(&enableVAD, "vad", true, "Enable Voice Activity Detection")
	flag.BoolVar(&realtime, "realtime", false, "Simulate realtime audio streaming speed")
}

func main() {
	flag.Parse()

	// 获取音频文件路径
	audioFile := flag.Arg(0)
	if audioFile == "" {
		printUsage()
		os.Exit(1)
	}

	// 检查文件是否存在
	if _, err := os.Stat(audioFile); os.IsNotExist(err) {
		log.Fatalf("音频文件不存在: %s", audioFile)
	}

	// 创建可取消的 context
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

	// 打印配置信息
	printConfig(audioFile)

	// 执行流式识别
	start := time.Now()
	if err := runStreamingSTT(ctx, audioFile); err != nil {
		log.Fatalf("识别失败: %v", err)
	}

	fmt.Printf("\n识别完成! 总耗时: %dms\n", time.Since(start).Milliseconds())
}

// printUsage 打印使用说明
func printUsage() {
	fmt.Println("STT 流式识别示例")
	fmt.Println()
	fmt.Println("Usage: stt_stream [options] <audio_file>")
	fmt.Println()
	fmt.Println("Options:")
	flag.PrintDefaults()
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  stt_stream audio.wav")
	fmt.Println("  stt_stream -provider azure -language zh-CN audio.wav")
	fmt.Println("  stt_stream -realtime audio.wav  # 模拟实时发送")
	fmt.Println()
	fmt.Println("WebSocket 协议流程:")
	fmt.Println("  1. 建立连接 → /ws/stt?provider=xxx")
	fmt.Println("  2. 接收 session.ready")
	fmt.Println("  3. 发送 session.config (语言、采样率、VAD)")
	fmt.Println("  4. 循环发送 audio.append (音频分块)")
	fmt.Println("  5. 实时接收 transcript.partial (部分结果)")
	fmt.Println("  6. 发送 input.commit (标记结束)")
	fmt.Println("  7. 接收 transcript.final (最终结果)")
	fmt.Println("  8. 发送 session.end (关闭会话)")
}

// printConfig 打印配置信息
func printConfig(audioFile string) {
	fmt.Println("==========================================")
	fmt.Println("  STT 流式识别示例")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Println("配置:")
	fmt.Printf("  Gateway:     %s\n", gatewayURL)
	fmt.Printf("  Provider:    %s\n", provider)
	fmt.Printf("  Language:    %s\n", language)
	fmt.Printf("  Sample Rate: %d Hz\n", sampleRate)
	fmt.Printf("  VAD:         %v\n", enableVAD)
	fmt.Printf("  Realtime:    %v\n", realtime)
	fmt.Printf("  Audio File:  %s\n", audioFile)
	fmt.Println()
}

// runStreamingSTT 执行流式 STT 识别
func runStreamingSTT(ctx context.Context, audioPath string) error {
	// ========== Step 1: 创建 STT 客户端 ==========
	config := &stt.Config{
		GatewayURL:     gatewayURL,
		Provider:       provider,
		APIKey:         apiKey,
		Language:       language,
		SampleRate:     sampleRate,
		EnableVAD:      enableVAD,
		ConnectTimeout: 30 * time.Second,
		ReadTimeout:    120 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	client, err := stt.NewClient(config)
	if err != nil {
		return fmt.Errorf("创建客户端失败: %w", err)
	}
	defer client.Close()

	// ========== Step 2: 打开音频文件 ==========
	file, err := os.Open(audioPath)
	if err != nil {
		return fmt.Errorf("打开音频文件失败: %w", err)
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, _ := file.Stat()
	fmt.Printf("音频文件大小: %d bytes\n", fileInfo.Size())

	// 跳过 WAV 文件头（如果是 WAV 格式）
	if strings.HasSuffix(strings.ToLower(audioPath), ".wav") {
		file.Seek(44, io.SeekStart)
		fmt.Println("检测到 WAV 格式，跳过 44 字节文件头")
	}

	// ========== Step 3: 建立流式会话 ==========
	// 这一步会：
	//   - 建立 WebSocket 连接到 /ws/stt?provider=xxx
	//   - 等待 session.ready 消息
	//   - 发送 session.config 消息
	fmt.Println()
	fmt.Println("建立流式会话...")

	opts := &stt.StreamOptions{
		Language:   language,
		SampleRate: sampleRate,
		EnableVAD:  enableVAD,
	}

	session, err := client.RecognizeStream(ctx, opts)
	if err != nil {
		return fmt.Errorf("建立会话失败: %w", err)
	}
	defer session.Close()

	fmt.Printf("会话已就绪: session_id=%s\n", session.ID)
	fmt.Println()

	// ========== Step 4: 并发执行发送和接收 ==========
	// 启动发送音频的 goroutine
	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sendAudioChunks(session, file, sampleRate)
	}()

	// 主 goroutine 处理识别事件
	fmt.Println("开始识别...")
	fmt.Println("------------------------------------------")

	texts, err := handleSTTEvents(session)
	if err != nil {
		return fmt.Errorf("处理识别事件失败: %w", err)
	}

	// 等待发送完成
	if sendErr := <-sendDone; sendErr != nil {
		return fmt.Errorf("发送音频失败: %w", sendErr)
	}

	// ========== Step 5: 输出最终结果 ==========
	fmt.Println("------------------------------------------")
	fmt.Println()
	fmt.Println("最终识别结果:")
	if len(texts) > 0 {
		fmt.Printf("  %s\n", strings.Join(texts, ""))
	} else {
		fmt.Println("  (无识别结果)")
	}

	return nil
}

// sendAudioChunks 分块发送音频数据
//
// 音频分块策略:
//   - 每块大小 = 采样率 * 2 字节 / 10 = 100ms 音频数据
//   - 例如 16kHz 采样率: 16000 * 2 / 10 = 3200 字节/块
//
// 发送流程:
//   1. 读取一块音频数据
//   2. 调用 session.Send() 发送 audio.append 消息
//   3. (可选) 等待 100ms 模拟实时发送
//   4. 重复直到文件结束
//   5. 调用 session.Commit() 发送 input.commit 消息
func sendAudioChunks(session *stt.Session, file *os.File, sampleRate int) error {
	// 计算每块大小: 100ms 音频 @ 16-bit PCM
	// 16kHz: 16000 samples/sec * 2 bytes/sample * 0.1 sec = 3200 bytes
	chunkSize := sampleRate * 2 / 10
	buf := make([]byte, chunkSize)

	chunkCount := 0
	totalBytes := int64(0)
	startTime := time.Now()

	for {
		n, err := file.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取音频数据失败: %w", err)
		}

		if n > 0 {
			// 发送 audio.append 消息
			if err := session.Send(buf[:n]); err != nil {
				return fmt.Errorf("发送音频块失败: %w", err)
			}

			chunkCount++
			totalBytes += int64(n)

			// 可选：模拟实时发送速度
			if realtime {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n发送完成: %d 块, %d bytes, 耗时 %dms\n",
		chunkCount, totalBytes, elapsed.Milliseconds())

	// 发送 input.commit 消息，标记音频结束
	if err := session.Commit(); err != nil {
		return fmt.Errorf("提交输入失败: %w", err)
	}
	fmt.Println("已发送 input.commit")

	return nil
}

// handleSTTEvents 处理 STT 识别事件
//
// 事件类型:
//   - EventPartial: 部分识别结果，实时显示（会被覆盖）
//   - EventFinal:   最终识别结果，追加到结果列表
//   - EventError:   识别错误
//   - EventClosed:  会话关闭
func handleSTTEvents(session *stt.Session) ([]string, error) {
	var finalTexts []string
	var lastErr error

	for event := range session.Events() {
		switch event.Type {
		case stt.EventPartial:
			// 部分识别结果 - 同一行覆盖显示
			fmt.Printf("\r[部分] %s", event.Text)

		case stt.EventFinal:
			// 最终识别结果 - 换行显示
			fmt.Printf("\r[最终] %s\n", event.Text)
			finalTexts = append(finalTexts, event.Text)

		case stt.EventError:
			// 识别错误
			fmt.Printf("\r[错误] %v\n", event.Error)
			lastErr = event.Error

		case stt.EventClosed:
			// 会话已关闭
			return finalTexts, lastErr
		}
	}

	return finalTexts, lastErr
}
