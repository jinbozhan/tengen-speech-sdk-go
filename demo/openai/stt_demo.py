#!/usr/bin/env python3
"""OpenAI 兼容 STT Demo - 使用 OpenAI Python SDK 调用 speech-arena STT 接口"""

import argparse
import os
import sys

from openai import OpenAI


def main():
    parser = argparse.ArgumentParser(description="STT Demo: 语音转文本")
    parser.add_argument("--file", required=True,
                        help="音频文件路径 (WAV 格式)")
    parser.add_argument("--model", default="whisper-1",
                        help="模型名称 (默认: whisper-1)")
    parser.add_argument("--language", default="en-NG",
                        help="语言代码, 如 en-NG, zh-CN, en-US (默认: en-NG)")
    parser.add_argument("--format", default="json",
                        choices=["json", "text", "verbose_json"],
                        help="响应格式 (默认: json)")
    args = parser.parse_args()

    if not os.path.exists(args.file):
        print(f"错误: 文件不存在: {args.file}")
        sys.exit(1)

    api_key = os.environ.get("OPENAI_API_KEY")
    if not api_key:
        print("错误: 请设置环境变量 OPENAI_API_KEY")
        print("  export OPENAI_API_KEY='your-api-key'")
        sys.exit(1)

    base_url = os.environ.get("OPENAI_BASE_URL", "http://localhost:7861/v1")

    client = OpenAI(api_key=api_key, base_url=base_url)

    file_size = os.path.getsize(args.file)
    print(f"服务地址: {base_url}")
    print(f"音频文件: {args.file} ({file_size} bytes)")
    print(f"模型: {args.model}")
    print(f"语言: {args.language or '(自动检测)'}")
    print(f"响应格式: {args.format}")
    print()

    try:
        kwargs = {
            "model": args.model,
            "file": open(args.file, "rb"),
            "response_format": args.format,
        }
        if args.language:
            kwargs["language"] = args.language

        transcript = client.audio.transcriptions.create(**kwargs)

        if args.format == "text":
            print(f"识别结果: {transcript}")
        elif args.format == "verbose_json":
            print(f"任务: {transcript.task}")
            print(f"语言: {transcript.language}")
            print(f"时长: {transcript.duration}s")
            print(f"文本: {transcript.text}")
        else:
            print(f"识别结果: {transcript.text}")
    except Exception as e:
        print(f"识别失败: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
