#!/bin/bash
# Tengen Speech SDK Build Script
# 用于编译 SDK 和示例程序

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# 创建 bin 目录
mkdir -p bin

echo "=========================================="
echo "  Tengen Speech SDK Build"
echo "=========================================="
echo ""

# 1. 依赖管理
echo "[1/8] 更新依赖..."
go mod tidy

# 2. 编译所有包
echo "[2/8] 编译所有包..."
go build ./...

# 3. 编译 TTS SDK Demo
echo "[3/8] 编译 TTS SDK Demo..."
go build -o bin/tts_stream_sdk ./examples-sdk/tts_stream
echo "      -> bin/tts_stream_sdk"

# 4. 编译 STT SDK Demo
echo "[4/8] 编译 STT SDK Demo..."
go build -o bin/stt_stream_sdk ./examples-sdk/stt_stream
echo "      -> bin/stt_stream_sdk"

# 5. 编译 STT WebSocket 示例
echo "[5/8] 编译 STT WebSocket 示例..."
go build -o bin/stt_stream_ws ./examples-ws/stt_stream
echo "      -> bin/stt_stream_ws"

# 6. 编译 TTS WebSocket 示例
echo "[6/8] 编译 TTS WebSocket 示例..."
go build -o bin/tts_stream_ws ./examples-ws/tts_stream
echo "      -> bin/tts_stream_ws"

# 7. 编译 TTS Benchmark
echo "[7/8] 编译 TTS Benchmark..."
go build -o bin/tts_benchmark ./cmd/tts_benchmark
echo "      -> bin/tts_benchmark"

# 8. 编译 TTS Detailed Timing
echo "[8/8] 编译 TTS Detailed Timing..."
go build -o bin/tts_detailed_timing ./cmd/tts_detailed_timing
echo "      -> bin/tts_detailed_timing"

echo ""
echo "=========================================="
echo "  构建完成!"
echo "=========================================="
echo ""
echo "使用方法:"
echo ""
echo "  TTS SDK Demo:"
echo "    ./bin/tts_stream_sdk \"你好，世界\""
echo "    ./bin/tts_stream_sdk -h  # 查看帮助"
echo ""
echo "  STT SDK Demo:"
echo "    ./bin/stt_stream_sdk audio.wav"
echo "    ./bin/stt_stream_sdk -h  # 查看帮助"
echo ""
echo "  STT WebSocket 示例:"
echo "    ./bin/stt_stream_ws audio.wav"
echo "    ./bin/stt_stream_ws -realtime audio.wav"
echo "    ./bin/stt_stream_ws -h  # 查看帮助"
echo ""
echo "  TTS WebSocket 示例:"
echo "    ./bin/tts_stream_ws \"你好，世界\""
echo "    ./bin/tts_stream_ws -voice en-NG-OkunNeutral \"测试\""
echo "    ./bin/tts_stream_ws -h  # 查看帮助"
echo ""
echo "  TTS Benchmark (性能测试):"
echo "    ./bin/tts_benchmark -provider tengen -voice en-NG-OkunNeutral -iterations 10"
echo "    ./bin/tts_benchmark -h  # 查看帮助"
echo ""
echo "  TTS Detailed Timing (详细耗时分析):"
echo "    ./bin/tts_detailed_timing -provider tengen -voice en-NG-OkunNeutral -iterations 3"
echo "    ./bin/tts_detailed_timing -h  # 查看帮助"
echo ""
