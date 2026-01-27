// Package main 提供TTS并发测试工具
package main

import (
	"math/rand"
	"sync"
	"time"
)

// TextProvider 测试文本提供者
type TextProvider struct {
	texts []string
	mu    sync.Mutex
	rng   *rand.Rand
}

// NewTextProvider 创建文本提供者
func NewTextProvider() *TextProvider {
	return &TextProvider{
		texts: defaultTexts(),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GetRandom 随机获取一条测试文本
func (p *TextProvider) GetRandom() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.texts[p.rng.Intn(len(p.texts))]
}

// GetByIndex 按索引获取测试文本（循环）
func (p *TextProvider) GetByIndex(i int) string {
	return p.texts[i%len(p.texts)]
}

// Count 返回文本数量
func (p *TextProvider) Count() int {
	return len(p.texts)
}

// defaultTexts 返回默认测试文本集
// 设计原则:
// 1. 长度分布：短句(~10词)、中句(~30词)、长句(~60词)
// 2. 语言：英文（匹配 en-NG 音色）
// 3. 内容：多样性，包含不同音素和语调
func defaultTexts() []string {
	return []string{
		// 短句 - 测试低延迟场景 (~10 词)
		"Hello, how are you today?",
		"The quick brown fox jumps over the lazy dog.",
		"Welcome to the speech arena platform.",
		"Please wait a moment while we process your request.",
		"Thank you very much for your patience.",
		"Good morning, it's a beautiful day.",
		"Can you help me with this problem?",
		"The weather forecast predicts rain tomorrow.",

		// 中等长度 - 典型应用场景 (~30 词)
		"The development of artificial intelligence has transformed the way we interact with technology in our daily lives, making many tasks more efficient and accessible.",
		"Speech synthesis technology enables machines to convert text into natural sounding human speech with remarkable accuracy and emotional expression.",
		"Our platform provides a unified interface for multiple text to speech providers, offering flexibility, reliability and consistent quality across different services.",
		"The streaming audio transmission ensures low latency response, making real-time applications possible for interactive voice assistants and communication tools.",
		"Voice cloning capabilities allow users to create personalized synthetic voices for various applications, from audiobooks to customer service systems.",
		"To prevent extra charges on your account, what amount can you commit to paying today? We have several payment options available for you.",
		"I understand your concern about the billing issue. Let me review your account and find the best solution for your situation.",

		// 长句 - 测试持续稳定性 (~60 词)
		"In the rapidly evolving landscape of digital communication, voice interfaces have emerged as a crucial bridge between humans and machines, enabling more natural and accessible interactions that transcend traditional text-based interfaces and open new possibilities for users across diverse contexts and abilities, from smart home devices to enterprise applications.",
		"The architecture of modern speech synthesis systems combines neural network models with streaming protocols to deliver high-quality audio output while maintaining low latency, a balance that requires careful optimization of both the acoustic model and the transmission pipeline to ensure smooth real-time performance.",
		"As businesses increasingly adopt voice technology for customer service, accessibility features, and content creation, the demand for scalable and reliable text to speech services continues to grow, driving innovation in both the technical capabilities and the deployment strategies of speech synthesis platforms around the world.",
	}
}
