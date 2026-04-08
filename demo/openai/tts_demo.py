#!/usr/bin/env python3
"""OpenAI 兼容 TTS Demo - 使用 OpenAI Python SDK 调用 speech-arena TTS 接口"""

import argparse
import os
import sys

from openai import OpenAI


def main():
    parser = argparse.ArgumentParser(description="TTS Demo: 文本合成语音")
    parser.add_argument("--text", default="Hello world, this is a text to speech test.",
                        help="要合成的文本 (默认: 英文测试句)")
    parser.add_argument("--voice", default="en-NG-AbeoNeural",
                        help="音色名称，对应 voice library 的 display_id (默认: en-NG-AbeoNeural)")
    parser.add_argument("--model", default="tts-1",
                        help="模型名称 (默认: tts-1)")
    parser.add_argument("--format", default="wav", choices=["wav", "pcm"],
                        help="输出格式 (默认: wav)")
    parser.add_argument("--speed", type=float, default=1.0,
                        help="语速, 0.25-4.0 (默认: 1.0)")
    parser.add_argument("--output", default="output.wav",
                        help="输出文件路径 (默认: output.wav)")
    args = parser.parse_args()

    api_key = os.environ.get("OPENAI_API_KEY")
    if not api_key:
        print("错误: 请设置环境变量 OPENAI_API_KEY")
        print("  export OPENAI_API_KEY='your-api-key'")
        sys.exit(1)

    base_url = os.environ.get("OPENAI_BASE_URL", "http://localhost:7861/v1")

    client = OpenAI(api_key=api_key, base_url=base_url)

    print(f"服务地址: {base_url}")
    print(f"音色: {args.voice}")
    print(f"模型: {args.model}")
    print(f"格式: {args.format}")
    print(f"语速: {args.speed}")
    print(f"文本: {args.text}")
    print()

    try:
        with client.audio.speech.with_streaming_response.create(
            model=args.model,
            voice=args.voice,
            input=args.text,
            response_format=args.format,
            speed=args.speed,
        ) as response:
            response.stream_to_file(args.output)
        file_size = os.path.getsize(args.output)
        print(f"合成成功! 输出: {args.output} ({file_size} bytes)")
    except Exception as e:
        print(f"合成失败: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
