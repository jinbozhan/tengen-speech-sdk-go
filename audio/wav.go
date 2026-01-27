// Package audio 提供音频处理工具
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// WAV文件头大小
const WAVHeaderSize = 44

// WAVHeader WAV文件头
type WAVHeader struct {
	ChunkID       [4]byte // "RIFF"
	ChunkSize     uint32  // 文件大小 - 8
	Format        [4]byte // "WAVE"
	Subchunk1ID   [4]byte // "fmt "
	Subchunk1Size uint32  // 16 for PCM
	AudioFormat   uint16  // 1 for PCM
	NumChannels   uint16  // 声道数
	SampleRate    uint32  // 采样率
	ByteRate      uint32  // SampleRate * NumChannels * BitsPerSample/8
	BlockAlign    uint16  // NumChannels * BitsPerSample/8
	BitsPerSample uint16  // 位深度
	Subchunk2ID   [4]byte // "data"
	Subchunk2Size uint32  // 数据大小
}

// ReadWAVHeader 读取WAV文件头
func ReadWAVHeader(r io.Reader) (*WAVHeader, error) {
	header := &WAVHeader{}
	if err := binary.Read(r, binary.LittleEndian, header); err != nil {
		return nil, fmt.Errorf("read WAV header: %w", err)
	}

	// 验证头部
	if string(header.ChunkID[:]) != "RIFF" {
		return nil, fmt.Errorf("invalid WAV: expected RIFF, got %s", header.ChunkID)
	}
	if string(header.Format[:]) != "WAVE" {
		return nil, fmt.Errorf("invalid WAV: expected WAVE, got %s", header.Format)
	}

	return header, nil
}

// WriteWAVHeader 写入WAV文件头
func WriteWAVHeader(w io.Writer, sampleRate, numChannels, bitsPerSample int, dataSize uint32) error {
	header := &WAVHeader{
		ChunkID:       [4]byte{'R', 'I', 'F', 'F'},
		ChunkSize:     dataSize + 36, // 总大小 - 8
		Format:        [4]byte{'W', 'A', 'V', 'E'},
		Subchunk1ID:   [4]byte{'f', 'm', 't', ' '},
		Subchunk1Size: 16, // PCM格式
		AudioFormat:   1,  // PCM
		NumChannels:   uint16(numChannels),
		SampleRate:    uint32(sampleRate),
		ByteRate:      uint32(sampleRate * numChannels * bitsPerSample / 8),
		BlockAlign:    uint16(numChannels * bitsPerSample / 8),
		BitsPerSample: uint16(bitsPerSample),
		Subchunk2ID:   [4]byte{'d', 'a', 't', 'a'},
		Subchunk2Size: dataSize,
	}

	return binary.Write(w, binary.LittleEndian, header)
}

// ReadWAVFile 读取WAV文件，返回PCM数据和头信息
func ReadWAVFile(path string) ([]byte, *WAVHeader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	header, err := ReadWAVHeader(file)
	if err != nil {
		return nil, nil, err
	}

	// 读取PCM数据
	pcm := make([]byte, header.Subchunk2Size)
	n, err := io.ReadFull(file, pcm)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, nil, fmt.Errorf("read PCM data: %w", err)
	}

	return pcm[:n], header, nil
}

// WriteWAVFile 将PCM数据写入WAV文件
func WriteWAVFile(path string, pcm []byte, sampleRate, numChannels, bitsPerSample int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// 写入头部
	if err := WriteWAVHeader(file, sampleRate, numChannels, bitsPerSample, uint32(len(pcm))); err != nil {
		return err
	}

	// 写入PCM数据
	_, err = file.Write(pcm)
	return err
}

// PCMToWAV 将PCM数据转换为WAV格式（包含头部）
func PCMToWAV(pcm []byte, sampleRate, numChannels, bitsPerSample int) ([]byte, error) {
	// 创建WAV缓冲区
	wav := make([]byte, WAVHeaderSize+len(pcm))

	// 写入头部
	header := &WAVHeader{
		ChunkID:       [4]byte{'R', 'I', 'F', 'F'},
		ChunkSize:     uint32(len(pcm) + 36),
		Format:        [4]byte{'W', 'A', 'V', 'E'},
		Subchunk1ID:   [4]byte{'f', 'm', 't', ' '},
		Subchunk1Size: 16,
		AudioFormat:   1,
		NumChannels:   uint16(numChannels),
		SampleRate:    uint32(sampleRate),
		ByteRate:      uint32(sampleRate * numChannels * bitsPerSample / 8),
		BlockAlign:    uint16(numChannels * bitsPerSample / 8),
		BitsPerSample: uint16(bitsPerSample),
		Subchunk2ID:   [4]byte{'d', 'a', 't', 'a'},
		Subchunk2Size: uint32(len(pcm)),
	}

	// 手动写入头部字节
	copy(wav[0:4], header.ChunkID[:])
	binary.LittleEndian.PutUint32(wav[4:8], header.ChunkSize)
	copy(wav[8:12], header.Format[:])
	copy(wav[12:16], header.Subchunk1ID[:])
	binary.LittleEndian.PutUint32(wav[16:20], header.Subchunk1Size)
	binary.LittleEndian.PutUint16(wav[20:22], header.AudioFormat)
	binary.LittleEndian.PutUint16(wav[22:24], header.NumChannels)
	binary.LittleEndian.PutUint32(wav[24:28], header.SampleRate)
	binary.LittleEndian.PutUint32(wav[28:32], header.ByteRate)
	binary.LittleEndian.PutUint16(wav[32:34], header.BlockAlign)
	binary.LittleEndian.PutUint16(wav[34:36], header.BitsPerSample)
	copy(wav[36:40], header.Subchunk2ID[:])
	binary.LittleEndian.PutUint32(wav[40:44], header.Subchunk2Size)

	// 复制PCM数据
	copy(wav[WAVHeaderSize:], pcm)

	return wav, nil
}

// WAVToPCM 从WAV数据提取PCM（跳过头部）
func WAVToPCM(wav []byte) ([]byte, *WAVHeader, error) {
	if len(wav) < WAVHeaderSize {
		return nil, nil, fmt.Errorf("WAV data too short")
	}

	// 解析头部
	header := &WAVHeader{}
	copy(header.ChunkID[:], wav[0:4])
	header.ChunkSize = binary.LittleEndian.Uint32(wav[4:8])
	copy(header.Format[:], wav[8:12])
	copy(header.Subchunk1ID[:], wav[12:16])
	header.Subchunk1Size = binary.LittleEndian.Uint32(wav[16:20])
	header.AudioFormat = binary.LittleEndian.Uint16(wav[20:22])
	header.NumChannels = binary.LittleEndian.Uint16(wav[22:24])
	header.SampleRate = binary.LittleEndian.Uint32(wav[24:28])
	header.ByteRate = binary.LittleEndian.Uint32(wav[28:32])
	header.BlockAlign = binary.LittleEndian.Uint16(wav[32:34])
	header.BitsPerSample = binary.LittleEndian.Uint16(wav[34:36])
	copy(header.Subchunk2ID[:], wav[36:40])
	header.Subchunk2Size = binary.LittleEndian.Uint32(wav[40:44])

	// 验证
	if string(header.ChunkID[:]) != "RIFF" || string(header.Format[:]) != "WAVE" {
		return nil, nil, fmt.Errorf("invalid WAV format")
	}

	return wav[WAVHeaderSize:], header, nil
}

// GetWAVInfo 获取WAV文件信息
func GetWAVInfo(path string) (*WAVHeader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return ReadWAVHeader(file)
}

// GetAudioDuration 计算音频时长（秒）
func GetAudioDuration(header *WAVHeader) float64 {
	if header.ByteRate == 0 {
		return 0
	}
	return float64(header.Subchunk2Size) / float64(header.ByteRate)
}

// GetAudioDurationFromFile 从文件获取音频时长
func GetAudioDurationFromFile(path string) (float64, error) {
	header, err := GetWAVInfo(path)
	if err != nil {
		return 0, err
	}
	return GetAudioDuration(header), nil
}
