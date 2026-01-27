// Package audio 音频格式转换
package audio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Format 音频格式
type Format string

const (
	FormatPCM Format = "pcm"
	FormatWAV Format = "wav"
	FormatMP3 Format = "mp3"
)

// DetectFormat 从文件扩展名检测格式
func DetectFormat(path string) Format {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".wav":
		return FormatWAV
	case ".pcm", ".raw":
		return FormatPCM
	case ".mp3":
		return FormatMP3
	default:
		return FormatPCM // 默认作为PCM处理
	}
}

// ConvertFile 转换音频文件格式
func ConvertFile(inputPath, outputPath string, sampleRate, channels, bitsPerSample int) error {
	inputFormat := DetectFormat(inputPath)
	outputFormat := DetectFormat(outputPath)

	// 读取输入
	var pcm []byte
	var err error

	switch inputFormat {
	case FormatWAV:
		pcm, _, err = ReadWAVFile(inputPath)
	case FormatPCM:
		pcm, err = os.ReadFile(inputPath)
	default:
		return fmt.Errorf("unsupported input format: %s", inputFormat)
	}
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	// 写入输出
	switch outputFormat {
	case FormatWAV:
		return WriteWAVFile(outputPath, pcm, sampleRate, channels, bitsPerSample)
	case FormatPCM:
		return os.WriteFile(outputPath, pcm, 0644)
	default:
		return fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// ReadAudioFile 读取音频文件为PCM
// 自动检测格式，返回PCM数据、采样率、声道数、位深度
func ReadAudioFile(path string) (pcm []byte, sampleRate, channels, bitsPerSample int, err error) {
	format := DetectFormat(path)

	switch format {
	case FormatWAV:
		var header *WAVHeader
		pcm, header, err = ReadWAVFile(path)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		return pcm, int(header.SampleRate), int(header.NumChannels), int(header.BitsPerSample), nil

	case FormatPCM:
		pcm, err = os.ReadFile(path)
		if err != nil {
			return nil, 0, 0, 0, err
		}
		// PCM没有头信息，使用默认值
		return pcm, 16000, 1, 16, nil

	default:
		return nil, 0, 0, 0, fmt.Errorf("unsupported format: %s", format)
	}
}

// WriteAudioFile 写入音频文件
// 自动根据扩展名决定格式
func WriteAudioFile(path string, pcm []byte, sampleRate, channels, bitsPerSample int) error {
	format := DetectFormat(path)

	switch format {
	case FormatWAV:
		return WriteWAVFile(path, pcm, sampleRate, channels, bitsPerSample)
	case FormatPCM:
		return os.WriteFile(path, pcm, 0644)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// ChunkReader 分块读取音频
type ChunkReader struct {
	reader    io.Reader
	chunkSize int
	buf       []byte
}

// NewChunkReader 创建分块读取器
// chunkSize: 每块字节数，对于16kHz 16bit单声道，100ms = 3200字节
func NewChunkReader(reader io.Reader, chunkSize int) *ChunkReader {
	return &ChunkReader{
		reader:    reader,
		chunkSize: chunkSize,
		buf:       make([]byte, chunkSize),
	}
}

// ReadChunk 读取一个音频块
// 返回读取的数据，如果到达文件末尾返回io.EOF
func (r *ChunkReader) ReadChunk() ([]byte, error) {
	n, err := r.reader.Read(r.buf)
	if err != nil {
		return nil, err
	}
	return r.buf[:n], nil
}

// CalculateChunkSize 计算指定时长的音频块大小
// durationMs: 时长（毫秒）
// sampleRate: 采样率
// channels: 声道数
// bitsPerSample: 位深度
func CalculateChunkSize(durationMs, sampleRate, channels, bitsPerSample int) int {
	bytesPerSample := bitsPerSample / 8
	samplesPerChunk := sampleRate * durationMs / 1000
	return samplesPerChunk * channels * bytesPerSample
}

// CalculateDuration 计算PCM数据时长（毫秒）
func CalculateDuration(pcmSize, sampleRate, channels, bitsPerSample int) int {
	bytesPerSample := bitsPerSample / 8
	bytesPerSecond := sampleRate * channels * bytesPerSample
	if bytesPerSecond == 0 {
		return 0
	}
	return pcmSize * 1000 / bytesPerSecond
}

// FileChunkReader 文件分块读取器
type FileChunkReader struct {
	file        *os.File
	chunkReader *ChunkReader
	isWAV       bool
}

// NewFileChunkReader 创建文件分块读取器
func NewFileChunkReader(path string, chunkMs, sampleRate, channels, bitsPerSample int) (*FileChunkReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	isWAV := DetectFormat(path) == FormatWAV
	if isWAV {
		// 跳过WAV头
		file.Seek(WAVHeaderSize, io.SeekStart)
	}

	chunkSize := CalculateChunkSize(chunkMs, sampleRate, channels, bitsPerSample)
	chunkReader := NewChunkReader(file, chunkSize)

	return &FileChunkReader{
		file:        file,
		chunkReader: chunkReader,
		isWAV:       isWAV,
	}, nil
}

// ReadChunk 读取一个音频块
func (r *FileChunkReader) ReadChunk() ([]byte, error) {
	return r.chunkReader.ReadChunk()
}

// Close 关闭文件
func (r *FileChunkReader) Close() error {
	return r.file.Close()
}

// Resample 简单重采样（线性插值）
// 注意：这是一个非常简单的实现，仅用于基本测试
// 生产环境应使用专业的重采样库
func Resample(pcm []byte, fromRate, toRate int) []byte {
	if fromRate == toRate {
		return pcm
	}

	ratio := float64(toRate) / float64(fromRate)
	newLen := int(float64(len(pcm)) * ratio)
	// 确保是偶数（16位采样）
	newLen = (newLen / 2) * 2

	result := make([]byte, newLen)

	for i := 0; i < newLen/2; i++ {
		srcIdx := float64(i) / ratio
		srcIdxInt := int(srcIdx)
		srcIdxFrac := srcIdx - float64(srcIdxInt)

		if srcIdxInt*2+3 < len(pcm) {
			// 线性插值
			s1 := int16(pcm[srcIdxInt*2]) | int16(pcm[srcIdxInt*2+1])<<8
			s2 := int16(pcm[srcIdxInt*2+2]) | int16(pcm[srcIdxInt*2+3])<<8
			sample := int16(float64(s1)*(1-srcIdxFrac) + float64(s2)*srcIdxFrac)
			result[i*2] = byte(sample)
			result[i*2+1] = byte(sample >> 8)
		} else if srcIdxInt*2+1 < len(pcm) {
			result[i*2] = pcm[srcIdxInt*2]
			result[i*2+1] = pcm[srcIdxInt*2+1]
		}
	}

	return result
}
