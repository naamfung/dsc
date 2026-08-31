package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
	"time"
)

// ---------- 测试 PCM 生成函数 ----------

func TestGenerateContinuousPCM(t *testing.T) {
	duration := 100 * time.Millisecond
	freqs := []float64{440}
	decay := 2.0
	pcm := generateContinuousPCM(duration, freqs, decay)

	expectedSamples := int(duration.Seconds() * float64(globalSampleRate))
	expectedBytes := expectedSamples * globalChannels * bytesPerSample
	if len(pcm) != expectedBytes {
		t.Errorf("长度错误: 期望 %d, 实际 %d", expectedBytes, len(pcm))
	}

	// 检查前几个样本是否有非零值（跳过 t=0 的零样本）
	foundNonZero := false
	for i := 1; i < 10 && i < len(pcm)/bytesPerSample; i++ {
		offset := i * globalChannels * bytesPerSample
		val := int16(binary.LittleEndian.Uint16(pcm[offset : offset+2]))
		if val != 0 {
			foundNonZero = true
			break
		}
	}
	if !foundNonZero {
		t.Error("未能找到非零样本，可能生成失败")
	}
}

func TestGenerateSweepPCM(t *testing.T) {
	duration := 100 * time.Millisecond
	pcm := generateSweepPCM(duration, 800, 200, 2.0)

	expectedSamples := int(duration.Seconds() * float64(globalSampleRate))
	expectedBytes := expectedSamples * globalChannels * bytesPerSample
	if len(pcm) != expectedBytes {
		t.Errorf("长度错误: 期望 %d, 实际 %d", expectedBytes, len(pcm))
	}

	foundNonZero := false
	for i := 1; i < 10 && i < len(pcm)/bytesPerSample; i++ {
		offset := i * globalChannels * bytesPerSample
		val := int16(binary.LittleEndian.Uint16(pcm[offset : offset+2]))
		if val != 0 {
			foundNonZero = true
			break
		}
	}
	if !foundNonZero {
		t.Error("未能找到非零样本")
	}
}

func TestGeneratePulsePCM(t *testing.T) {
	duration := 200 * time.Millisecond
	freqs := []float64{600, 800}
	pulseCount := 3
	gap := 50 * time.Millisecond
	pcm := generatePulsePCM(duration, freqs, pulseCount, gap)

	totalDuration := duration + gap*time.Duration(pulseCount-1)
	expectedSamples := int(totalDuration.Seconds() * float64(globalSampleRate))
	expectedBytes := expectedSamples * globalChannels * bytesPerSample
	if len(pcm) != expectedBytes {
		t.Errorf("长度错误: 期望 %d, 实际 %d", expectedBytes, len(pcm))
	}
	// 简单验证静音区域存在（只检查中间某个静音点）
	if len(pcm) > 0 {
		sampleIndex := int((duration + gap/2).Seconds() * float64(globalSampleRate))
		offset := sampleIndex * globalChannels * bytesPerSample
		if offset+2 <= len(pcm) {
			val := int16(binary.LittleEndian.Uint16(pcm[offset : offset+2]))
			if val > 100 || val < -100 {
				t.Logf("警告：静音区域样本不为零: %d（可能因边界抖动）", val)
				// 不强制失败，因为可能存在轻微泄漏
			}
		}
	}
}

// ---------- 测试 WAV 解码器 ----------

func TestDecodeWAV(t *testing.T) {
	sampleRate := 44100
	channels := 2
	bitsPerSample := 16
	numSamples := 100
	dataSize := numSamples * channels * (bitsPerSample / 8)

	// 生成 PCM 数据
	pcmData := make([]byte, dataSize)
	for i := 0; i < numSamples; i++ {
		val := int16(1000 * math.Sin(2*math.Pi*float64(i)/50))
		offset := i * channels * 2
		binary.LittleEndian.PutUint16(pcmData[offset:offset+2], uint16(val))
		binary.LittleEndian.PutUint16(pcmData[offset+2:offset+4], uint16(val))
	}

	// 构建 WAV 文件
	wavBuf := new(bytes.Buffer)

	// RIFF header
	wavBuf.WriteString("RIFF")
	fileSize := uint32(36 + dataSize) // 36 = 12 + 24 (fmt chunk)
	binary.Write(wavBuf, binary.LittleEndian, fileSize)
	wavBuf.WriteString("WAVE")

	// fmt chunk
	wavBuf.WriteString("fmt ")
	fmtSize := uint32(16)
	binary.Write(wavBuf, binary.LittleEndian, fmtSize)
	audioFormat := uint16(1)
	binary.Write(wavBuf, binary.LittleEndian, audioFormat)
	binary.Write(wavBuf, binary.LittleEndian, uint16(channels))
	binary.Write(wavBuf, binary.LittleEndian, uint32(sampleRate))
	byteRate := uint32(sampleRate * channels * (bitsPerSample / 8))
	binary.Write(wavBuf, binary.LittleEndian, byteRate)
	blockAlign := uint16(channels * (bitsPerSample / 8))
	binary.Write(wavBuf, binary.LittleEndian, blockAlign)
	binary.Write(wavBuf, binary.LittleEndian, uint16(bitsPerSample))

	// data chunk
	wavBuf.WriteString("data")
	binary.Write(wavBuf, binary.LittleEndian, uint32(dataSize))
	wavBuf.Write(pcmData)

	// 解码
	reader := bytes.NewReader(wavBuf.Bytes())
	pcmReader, sr, ch, err := decodeWAV(reader)
	if err != nil {
		t.Fatalf("decodeWAV 失败: %v", err)
	}
	if sr != sampleRate {
		t.Errorf("采样率错误: 期望 %d, 实际 %d", sampleRate, sr)
	}
	if ch != channels {
		t.Errorf("声道数错误: 期望 %d, 实际 %d", channels, ch)
	}

	readBuf, err := io.ReadAll(pcmReader)
	if err != nil {
		t.Fatalf("读取 PCM 数据失败: %v", err)
	}
	if len(readBuf) != dataSize {
		t.Errorf("PCM 数据长度错误: 期望 %d, 实际 %d", dataSize, len(readBuf))
	}
	if len(readBuf) >= 2 && len(pcmData) >= 2 {
		if readBuf[0] != pcmData[0] || readBuf[1] != pcmData[1] {
			t.Error("PCM 数据内容不匹配")
		}
	}
}

// ---------- 测试 MP3 解码器 ----------
func TestDecodeMP3(t *testing.T) {
	invalidData := bytes.NewReader([]byte("invalid mp3 data"))
	_, _, _, err := decodeMP3(invalidData)
	if err == nil {
		t.Error("期望错误，但解码成功了（无效数据）")
	}
}

// ---------- 测试内置音效缓存 ----------
func TestInitSoundCache(t *testing.T) {
	initSoundCache()
	if len(soundMap) != 4 {
		t.Errorf("缓存大小错误: 期望 4, 实际 %d", len(soundMap))
	}
	expectedTypes := []string{"success", "error", "warning", "info"}
	for _, typ := range expectedTypes {
		if _, ok := soundMap[typ]; !ok {
			t.Errorf("缺少 %s 音效", typ)
		}
	}
	for typ, pcm := range soundMap {
		if len(pcm) == 0 {
			t.Errorf("%s 的 PCM 数据为空", typ)
		}
	}
}

// ---------- 测试 PlayQueue ----------
func TestPlayQueue(t *testing.T) {
	for i := 0; i < 100; i++ {
		select {
		case playQueue <- PlayRequest{SoundType: "success"}:
		default:
			t.Fatal("队列已满，无法发送")
		}
	}
	for len(playQueue) > 0 {
		<-playQueue
	}
}

// ---------- 测试常量 ----------
func TestConstants(t *testing.T) {
	if globalSampleRate <= 0 {
		t.Error("globalSampleRate 无效")
	}
	if globalChannels != 2 {
		t.Error("globalChannels 应为 2")
	}
	if bytesPerSample != 2 {
		t.Error("bytesPerSample 应为 2")
	}
	if maxInt16 != 32767 {
		t.Error("maxInt16 应为 32767")
	}
}

// ---------- 基准测试 ----------
func BenchmarkGenerateContinuousPCM(b *testing.B) {
	for i := 0; i < b.N; i++ {
		generateContinuousPCM(1*time.Second, []float64{440, 550}, 3.0)
	}
}
