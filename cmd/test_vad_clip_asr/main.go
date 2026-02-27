// Package main - VAD-Clip ASR 端到端正确性测试
//
// 遍历 sample/ 目录下的 WAV 文件，通过 gateway WebSocket 进行流式 STT 识别，
// 验证 VAD 过滤静音帧后 Azure STT 的识别结果是否完整、准确。
//
// 使用方法:
//
//	./test_vad_clip_asr
//	./test_vad_clip_asr -gateway ws://localhost:7861 -provider azure -language en-NG
//	./test_vad_clip_asr -sample-dir /path/to/wav/files
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jinbozhan/tengen-speech-sdk-go/stt"
)

const (
	wavHeaderSize      = 44              // WAV 文件头大小（字节）
	connectTimeout     = 30 * time.Second // WebSocket 连接超时
	readTimeout        = 120 * time.Second // 识别读超时（长文件需要更久）
	writeTimeout       = 10 * time.Second  // WebSocket 写超时
	filenameTruncLen   = 45               // 表格中文件名截断长度
	textPreviewLen     = 80               // 实时输出文本预览长度
	textSummaryLen     = 65               // 汇总表格文本截断长度
	separatorLen       = 130              // 分隔线长度
)

// 命令行参数
var (
	gatewayURL string
	provider   string
	apiKey     string
	language   string
	sampleRate int
	sampleDir  string
	outputFile string
)

func init() {
	flag.StringVar(&gatewayURL, "gateway", "ws://localhost:7861", "Gateway WebSocket URL")
	flag.StringVar(&provider, "provider", "azure", "STT provider")
	flag.StringVar(&apiKey, "apikey", "", "Gateway API Key for authentication")
	flag.StringVar(&language, "language", "en-NG", "Recognition language")
	flag.IntVar(&sampleRate, "sample-rate", 8000, "Audio sample rate in Hz")
	flag.StringVar(&sampleDir, "sample-dir", "", "WAV files directory (default: <repo>/sample/)")
	flag.StringVar(&outputFile, "output", "", "Output results to file (markdown format)")
}

// fileResult 单个文件的识别结果
type fileResult struct {
	Filename    string
	DurationSec float64
	ElapsedSec  float64
	Text        string
	Segments    []stt.Segment // 分段结果（含时间戳）
	Error       string
}

func main() {
	flag.Parse()

	// 默认 sample 目录：基于可执行文件位置或当前目录
	if sampleDir == "" {
		sampleDir = findSampleDir()
	}

	// 查找 WAV 文件
	wavFiles, err := filepath.Glob(filepath.Join(sampleDir, "*.wav"))
	if err != nil {
		log.Fatalf("查找 WAV 文件失败: %v", err)
	}
	if len(wavFiles) == 0 {
		log.Fatalf("未找到 WAV 文件: %s/*.wav", sampleDir)
	}

	// 排序
	sortStrings(wavFiles)

	// 打印配置
	fmt.Println("==========================================")
	fmt.Println("  VAD-Clip ASR 端到端测试")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Printf("Gateway:     %s\n", gatewayURL)
	fmt.Printf("Provider:    %s\n", provider)
	fmt.Printf("API Key:     %s\n", apiKey)
	fmt.Printf("Language:    %s\n", language)
	fmt.Printf("Sample Rate: %d Hz\n", sampleRate)
	fmt.Printf("Sample Dir:  %s\n", sampleDir)
	fmt.Printf("文件数:      %d\n", len(wavFiles))
	fmt.Println()

	// 创建可取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n正在取消...")
		cancel()
	}()

	// 逐文件识别
	results := make([]fileResult, 0, len(wavFiles))

	for i, wavPath := range wavFiles {
		filename := filepath.Base(wavPath)
		durSec := getWavDurationSec(wavPath)

		fmt.Printf("[%d/%d] %s (%.1fs)\n", i+1, len(wavFiles), filename, durSec)

		start := time.Now()
		text, segments, recErr := recognizeFile(ctx, wavPath)
		elapsed := time.Since(start).Seconds()

		r := fileResult{
			Filename:    filename,
			DurationSec: durSec,
			ElapsedSec:  elapsed,
			Text:        text,
			Segments:    segments,
		}
		if recErr != nil {
			r.Error = recErr.Error()
		}
		results = append(results, r)

		// 实时输出
		if r.Error != "" {
			fmt.Printf("  ERROR: %s\n", r.Error)
		} else if r.Text != "" {
			preview := truncate(r.Text, textPreviewLen)
			fmt.Printf("  Text: %s\n", preview)
		} else {
			fmt.Printf("  (空结果)\n")
		}
		fmt.Printf("  耗时: %.2fs\n\n", elapsed)

		// 检查 context 是否已取消
		if ctx.Err() != nil {
			fmt.Println("测试已取消")
			break
		}
	}

	// 汇总报告
	printSummary(results)

	// 输出到文件
	if outputFile != "" {
		if err := writeMarkdownReport(outputFile, results); err != nil {
			log.Printf("写入报告失败: %v", err)
		} else {
			fmt.Printf("\n报告已写入: %s\n", outputFile)
		}
	}
}

// recognizeFile 识别单个文件
func recognizeFile(ctx context.Context, audioPath string) (string, []stt.Segment, error) {
	config := &stt.Config{
		GatewayURL:     gatewayURL,
		Provider:       provider,
		APIKey:         apiKey,
		Language:       language,
		SampleRate:     sampleRate,
		ConnectTimeout: connectTimeout,
		ReadTimeout:    readTimeout,
		WriteTimeout:   writeTimeout,
	}

	client, err := stt.NewClient(config)
	if err != nil {
		return "", nil, fmt.Errorf("创建客户端失败: %w", err)
	}
	defer client.Close()

	result, err := client.RecognizeFile(ctx, audioPath)
	if err != nil {
		return "", nil, err
	}
	if result.Error != nil {
		return "", nil, result.Error
	}

	return strings.TrimSpace(result.Text), result.Segments, nil
}

// printSummary 输出汇总表格
func printSummary(results []fileResult) {
	sep := strings.Repeat("=", separatorLen)
	dash := strings.Repeat("-", separatorLen)

	fmt.Println(sep)
	fmt.Println("汇总报告")
	fmt.Println(sep)
	fmt.Printf("%-3s %-47s %6s %6s  %s\n", "#", "文件名", "时长", "耗时", "识别结果")
	fmt.Println(dash)

	okCount := 0
	for i, r := range results {
		shortName := truncate(r.Filename, filenameTruncLen)
		dur := fmt.Sprintf("%.1fs", r.DurationSec)
		elp := fmt.Sprintf("%.1fs", r.ElapsedSec)

		var status string
		if r.Error != "" {
			status = "ERROR: " + truncate(r.Error, textSummaryLen)
		} else if r.Text != "" {
			okCount++
			status = truncate(r.Text, textSummaryLen)
		} else {
			status = "(空)"
		}
		fmt.Printf("%-3d %-47s %6s %6s  %s\n", i+1, shortName, dur, elp, status)
	}

	fmt.Println(dash)
	fmt.Printf("成功: %d/%d\n", okCount, len(results))
	fmt.Println()
	fmt.Println("提示: 请检查 gateway 日志中的 VAD filter stats (speech_pct) 以确认 VAD 过滤效果")
}

// writeMarkdownReport 输出 Markdown 格式报告
func writeMarkdownReport(path string, results []fileResult) error {
	var sb strings.Builder

	sb.WriteString("# VAD-Clip ASR 测试结果\n\n")
	sb.WriteString(fmt.Sprintf("- **日期**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("- **Gateway**: %s\n", gatewayURL))
	sb.WriteString(fmt.Sprintf("- **Provider**: %s\n", provider))
	sb.WriteString(fmt.Sprintf("- **Language**: %s\n", language))
	sb.WriteString(fmt.Sprintf("- **Sample Rate**: %d Hz\n", sampleRate))
	sb.WriteString("\n")

	okCount := 0
	for _, r := range results {
		if r.Error == "" && r.Text != "" {
			okCount++
		}
	}
	sb.WriteString(fmt.Sprintf("**成功率**: %d/%d\n\n", okCount, len(results)))

	sb.WriteString("| # | 文件名 | 时长 | 耗时 | 识别结果 |\n")
	sb.WriteString("|---|--------|------|------|----------|\n")

	for i, r := range results {
		text := r.Text
		if r.Error != "" {
			text = "ERROR: " + r.Error
		} else if text == "" {
			text = "(空)"
		}
		// 转义 Markdown 中的 pipe
		text = strings.ReplaceAll(text, "|", "\\|")
		sb.WriteString(fmt.Sprintf("| %d | %s | %.1fs | %.1fs | %s |\n",
			i+1, r.Filename, r.DurationSec, r.ElapsedSec, text))
	}

	sb.WriteString("\n## 详细识别文本\n\n")
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, r.Filename))
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("**ERROR**: %s\n\n", r.Error))
		} else if r.Text != "" {
			sb.WriteString(fmt.Sprintf("> %s\n\n", r.Text))
			if len(r.Segments) > 0 {
				sb.WriteString("| # | 开始 | 结束 | 文本 |\n")
				sb.WriteString("|---|------|------|------|\n")
				for j, seg := range r.Segments {
					segText := strings.ReplaceAll(seg.Text, "|", "\\|")
					sb.WriteString(fmt.Sprintf("| %d | %.3fs | %.3fs | %s |\n",
						j+1, seg.StartTime.Seconds(), seg.EndTime.Seconds(), segText))
				}
				sb.WriteString("\n")
			}
		} else {
			sb.WriteString("(无识别结果)\n\n")
		}
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// getWavDurationSec 读取 WAV 文件时长（秒）
func getWavDurationSec(path string) float64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	// 读取 WAV 文件头
	header := make([]byte, wavHeaderSize)
	n, err := f.Read(header)
	if err != nil || n < wavHeaderSize {
		return 0
	}

	// WAV 格式: 偏移 24-27 为采样率（little-endian uint32）
	rate := binary.LittleEndian.Uint32(header[24:28])
	// 偏移 34-35 为 bits per sample
	bitsPerSample := binary.LittleEndian.Uint16(header[34:36])
	// 偏移 22-23 为 channels
	channels := binary.LittleEndian.Uint16(header[22:24])

	if rate == 0 || bitsPerSample == 0 || channels == 0 {
		return 0
	}

	// 文件大小减去头部 = PCM 数据大小
	info, err := f.Stat()
	if err != nil {
		return 0
	}
	dataSize := info.Size() - int64(wavHeaderSize)
	bytesPerSample := int64(bitsPerSample) / 8 * int64(channels)
	totalSamples := dataSize / bytesPerSample

	return float64(totalSamples) / float64(rate)
}

// findSampleDir 查找 sample 目录
func findSampleDir() string {
	// 尝试相对于当前工作目录
	candidates := []string{
		"sample",
		"../../sample", // 从 cmd/test_vad_clip_asr/ 出发
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	// 兜底：当前目录下的 sample
	return "sample"
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// sortStrings 排序字符串切片（简单插入排序，文件数少）
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		key := ss[i]
		j := i - 1
		for j >= 0 && ss[j] > key {
			ss[j+1] = ss[j]
			j--
		}
		ss[j+1] = key
	}
}

