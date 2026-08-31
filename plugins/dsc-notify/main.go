package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	dsc "dsc-sdk"
	"dsc/core"
	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

// ---------- 常量 ----------
const (
	// 全局采样率固定为 44100 Hz（最主流）
	globalSampleRate = 44100
	globalChannels   = 2
	bytesPerSample   = 2 // 16bit
	maxInt16         = 32767
)

var (
	soundMap  map[string][]byte // 内置音效 PCM 缓存
	globalCtx *oto.Context      // 全局音频上下文
)

// ---------- PCM 生成函数（采用 globalSampleRate） ----------

func generateContinuousPCM(duration time.Duration, freqs []float64, decay float64) []byte {
	numSamples := int(duration.Seconds() * float64(globalSampleRate))
	buf := make([]byte, numSamples*globalChannels*bytesPerSample)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(globalSampleRate)
		envelope := math.Exp(-t * decay)
		var val float64
		for _, f := range freqs {
			val += math.Sin(2 * math.Pi * f * t)
		}
		val *= envelope * 0.3 * float64(maxInt16)
		if val > float64(maxInt16) {
			val = float64(maxInt16)
		} else if val < -float64(maxInt16) {
			val = -float64(maxInt16)
		}
		intVal := int16(val)
		offset := i * globalChannels * bytesPerSample
		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(intVal))
		binary.LittleEndian.PutUint16(buf[offset+2:offset+4], uint16(intVal))
	}
	return buf
}

func generateSweepPCM(duration time.Duration, startFreq, endFreq float64, decay float64) []byte {
	numSamples := int(duration.Seconds() * float64(globalSampleRate))
	buf := make([]byte, numSamples*globalChannels*bytesPerSample)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(globalSampleRate)
		freq := startFreq + (endFreq-startFreq)*(t/duration.Seconds())
		envelope := math.Exp(-t * decay)
		val := math.Sin(2*math.Pi*freq*t) * envelope * 0.3 * float64(maxInt16)
		intVal := int16(val)
		offset := i * globalChannels * bytesPerSample
		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(intVal))
		binary.LittleEndian.PutUint16(buf[offset+2:offset+4], uint16(intVal))
	}
	return buf
}

func generatePulsePCM(duration time.Duration, freqs []float64, pulseCount int, gap time.Duration) []byte {
	if pulseCount <= 0 {
		pulseCount = 1
	}
	pulseDuration := duration / time.Duration(pulseCount)
	if pulseDuration <= 0 {
		pulseDuration = 100 * time.Millisecond
	}
	if gap <= 0 {
		gap = 100 * time.Millisecond
	}
	totalDuration := duration + gap*time.Duration(pulseCount-1)
	totalSamples := int(totalDuration.Seconds() * float64(globalSampleRate))
	buf := make([]byte, totalSamples*globalChannels*bytesPerSample)

	for i := 0; i < totalSamples; i++ {
		t := float64(i) / float64(globalSampleRate)
		var val float64
		pulseTotal := pulseDuration + gap
		posInCycle := time.Duration(int(t*float64(time.Second))) % pulseTotal
		if posInCycle < pulseDuration {
			pulseT := float64(posInCycle) / float64(time.Second)
			for _, f := range freqs {
				val += math.Sin(2 * math.Pi * f * pulseT)
			}
			envelope := 1.0 - float64(posInCycle)/float64(pulseDuration)*0.2
			val *= envelope * 0.25 * float64(maxInt16)
		}
		intVal := int16(val)
		offset := i * globalChannels * bytesPerSample
		binary.LittleEndian.PutUint16(buf[offset:offset+2], uint16(intVal))
		binary.LittleEndian.PutUint16(buf[offset+2:offset+4], uint16(intVal))
	}
	return buf
}

// ---------- 初始化内置音效缓存 ----------
func initSoundCache() {
	soundMap = make(map[string][]byte)

	// success: 上升双音（300Hz → 600Hz），中间间隔50ms，音量适中
	// 生成第一段（130ms）
	pcm1 := generateContinuousPCM(130*time.Millisecond, []float64{300}, 10.0)
	// 静音间隔（50ms）
	silenceSamples := int((50 * time.Millisecond).Seconds() * float64(globalSampleRate))
	silence := make([]byte, silenceSamples*globalChannels*bytesPerSample)
	// 生成第二段（110ms，频率更高）
	pcm2 := generateContinuousPCM(110*time.Millisecond, []float64{600}, 10.0)

	// 拼接成完整音效
	var buf bytes.Buffer
	buf.Write(pcm1)
	buf.Write(silence)
	buf.Write(pcm2)
	pcmSuccess := buf.Bytes()

	// 整体缩放振幅为60%，与 error/warning 的响度匹配
	for i := 0; i < len(pcmSuccess); i += 2 {
		val := int16(binary.LittleEndian.Uint16(pcmSuccess[i : i+2]))
		val = int16(float64(val) * 0.6)
		binary.LittleEndian.PutUint16(pcmSuccess[i:i+2], uint16(val))
	}
	soundMap["success"] = pcmSuccess

	// error: 三连脉冲 700Hz（原设计，保持不变）
	soundMap["error"] = generatePulsePCM(
		150*time.Millisecond,
		[]float64{700},
		3,
		120*time.Millisecond,
	)

	// warning: 双脉冲 600Hz（原设计，保持不变）
	soundMap["warning"] = generatePulsePCM(
		150*time.Millisecond,
		[]float64{600},
		2,
		200*time.Millisecond,
	)

	// info: 双脉冲 400Hz（原设计，保持不变）
	soundMap["info"] = generatePulsePCM(
		150*time.Millisecond,
		[]float64{400},
		2,
		150*time.Millisecond,
	)

	log.Println("✅ 内置音效缓存已生成 (success, error, warning, info)")
}

// ---------- 初始化全局音频上下文 ----------
func initGlobalAudio() error {
	ctx, readyChan, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   globalSampleRate,
		ChannelCount: globalChannels,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return fmt.Errorf("初始化全局音频失败: %w", err)
	}
	globalCtx = ctx
	<-readyChan
	return nil
}

// ---------- 播放引擎（使用全局上下文） ----------

// 播放 PCM 数据（内置音效）
func playPCM(pcm []byte) {
	go func() {
		reader := bytes.NewReader(pcm)
		player := globalCtx.NewPlayer(reader)
		defer player.Close()
		player.Play()

		// 根据数据长度估算播放时长
		bytesPerSecond := globalSampleRate * globalChannels * bytesPerSample
		duration := time.Duration(len(pcm)/bytesPerSecond) * time.Second
		if duration == 0 {
			duration = 1 * time.Second
		}
		time.Sleep(duration + 500*time.Millisecond)
	}()
}

// 播放自定义文件（MP3/WAV）
func playCustomFile(filePath string) error {
	if _, err := os.Stat(filePath); err != nil {
		return fmt.Errorf("文件不存在: %w", err)
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(filePath))
	var reader io.Reader
	var sampleRate int
	var channelCount int
	var errDecode error

	switch ext {
	case ".mp3":
		reader, sampleRate, channelCount, errDecode = decodeMP3(file)
	case ".wav":
		reader, sampleRate, channelCount, errDecode = decodeWAV(file)
	default:
		return fmt.Errorf("不支持的文件格式: %s (仅支持 .mp3 和 .wav)", ext)
	}
	if errDecode != nil {
		return fmt.Errorf("解码失败: %w", errDecode)
	}

	// 检查采样率是否匹配
	if sampleRate != globalSampleRate {
		log.Printf("⚠️ 文件采样率 %dHz 与全局采样率 %dHz 不一致，播放可能变调", sampleRate, globalSampleRate)
	}

	// 读取全部 PCM 数据
	pcmData, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取 PCM 数据失败: %w", err)
	}
	if len(pcmData) == 0 {
		return fmt.Errorf("PCM 数据为空")
	}
	log.Printf("🎵 播放自定义文件: %s, 采样率=%dHz, 声道=%d, 数据长度=%d 字节",
		filePath, sampleRate, channelCount, len(pcmData))

	// 异步播放
	go func() {
		player := globalCtx.NewPlayer(bytes.NewReader(pcmData))
		defer player.Close()
		player.Play()

		// 注意：使用全局采样率计算时长，因为播放器会以全局采样率输出
		// 如果原采样率不同，播放时长会变化，但这里我们只做粗略等待
		bytesPerSecond := globalSampleRate * globalChannels * bytesPerSample
		duration := time.Duration(len(pcmData)/bytesPerSecond) * time.Second
		if duration == 0 {
			duration = 1 * time.Second
		}
		time.Sleep(duration + 500*time.Millisecond)
		log.Printf("✅ 播放完成: %s", filePath)
	}()

	return nil
}

// ---------- 解码器 ----------

// 解码 MP3
func decodeMP3(r io.Reader) (io.Reader, int, int, error) {
	decoder, err := mp3.NewDecoder(r)
	if err != nil {
		return nil, 0, 0, err
	}
	// MP3 通常是立体声，但也可以从 decoder 获取声道数？go-mp3 不直接提供，默认为 2
	return decoder, decoder.SampleRate(), 2, nil
}

// 解码 WAV（仅支持 PCM 16bit）
func decodeWAV(r io.Reader) (io.Reader, int, int, error) {
	header := make([]byte, 12)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, 0, 0, fmt.Errorf("读取 RIFF 头失败: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("无效的 WAV 文件")
	}

	var sampleRate, channelCount int

	for {
		chunkID := make([]byte, 4)
		if _, err := io.ReadFull(r, chunkID); err != nil {
			if err == io.EOF {
				break
			}
			return nil, 0, 0, fmt.Errorf("读取块ID失败: %w", err)
		}
		var chunkSize uint32
		if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
			return nil, 0, 0, fmt.Errorf("读取块大小失败: %w", err)
		}

		switch string(chunkID) {
		case "fmt ":
			fmtData := make([]byte, chunkSize)
			if _, err := io.ReadFull(r, fmtData); err != nil {
				return nil, 0, 0, fmt.Errorf("读取 fmt 数据失败: %w", err)
			}
			if len(fmtData) < 16 {
				return nil, 0, 0, fmt.Errorf("fmt 块数据不足")
			}
			audioFormat := binary.LittleEndian.Uint16(fmtData[0:2])
			if audioFormat != 1 {
				return nil, 0, 0, fmt.Errorf("仅支持 PCM 格式 (format=1), 当前为 %d", audioFormat)
			}
			channelCount = int(binary.LittleEndian.Uint16(fmtData[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
			bitsPerSample := int(binary.LittleEndian.Uint16(fmtData[14:16]))
			if bitsPerSample != 16 {
				return nil, 0, 0, fmt.Errorf("仅支持 16bit 采样, 当前为 %d", bitsPerSample)
			}
		case "data":
			return io.LimitReader(r, int64(chunkSize)), sampleRate, channelCount, nil
		default:
			if _, err := io.CopyN(io.Discard, r, int64(chunkSize)); err != nil {
				return nil, 0, 0, fmt.Errorf("跳过块 %s 失败: %w", chunkID, err)
			}
		}
	}
	return nil, 0, 0, fmt.Errorf("未找到 data 块")
}

// ---------- 播放队列 ----------

type PlayRequest struct {
	SoundType  string
	CustomPath string
}

var playQueue = make(chan PlayRequest, 100)

func playWorker() {
	for req := range playQueue {
		if globalCtx == nil {
			log.Printf("⚠️ 音频上下文未初始化，忽略播放请求: %+v", req)
			continue
		}
		if req.CustomPath != "" {
			if err := playCustomFile(req.CustomPath); err != nil {
				log.Printf("❌ 播放自定义文件失败: %v", err)
			}
		} else {
			pcm, ok := soundMap[req.SoundType]
			if !ok {
				log.Printf("⚠️ 未知音效类型: %s，使用 success", req.SoundType)
				pcm = soundMap["success"]
			}
			playPCM(pcm)
		}
	}
}

// main 以公共 SDK（dsc-sdk）启动通用（dsc 类型）插件：不注册任何模型可调用的
// 工具，仅经 Hook.OnEvent 订阅宿主事件实现程序性完成音效——agent 回合完成
// （成功 idle / 失败 error）时播放相应音效，完全由宿主事件驱动、不依赖模型调用。
func main() {
	sdk := dsc.New(dsc.Config{Name: "notify", Version: "1.0.0", Type: dsc.TypeDsc})
	sdk.Hook(dsc.Hook{
		OnEvent: func(ctx context.Context, eventType, dataJSON string) {
			switch eventType {
			case string(core.EventAgentStatus):
				var ev core.AgentStatusEvent
				if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
					return
				}
				if ev.Status != core.AgentStatusIdle {
					return // 仅回合完成提醒，运行中/起始不打扰
				}
				// 事件已到达：先记日志（不受音频初始化影响，便于观测程序性触发）
				log.Println("🔔 回合完成(success)：agent/status=idle 事件到达")
				if globalCtx == nil {
					return // 音频未初始化（如无音频设备 / DSC_NOTIFY_NO_AUDIO），跳过播放
				}
				playQueue <- PlayRequest{SoundType: "success"}
			case string(core.EventAgentError):
				// 回合失败 → error 音效
				if globalCtx == nil {
					return // 音频未初始化，跳过播放
				}
				playQueue <- PlayRequest{SoundType: "error"}
			}
		},
	})
	sdk.OnStart(func(ctx context.Context) error {
		initSoundCache()
		if os.Getenv("DSC_NOTIFY_NO_AUDIO") == "1" {
			log.Println("⚠️ DSC_NOTIFY_NO_AUDIO=1，跳过音频初始化（静默测试模式）")
		} else if err := initGlobalAudio(); err != nil {
			log.Printf("⚠️ 初始化音频失败（后续播放将不可用）: %v", err)
		}
		go playWorker()
		return nil
	})
	sdk.Serve()
}
