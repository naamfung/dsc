package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	dsc "dsc-sdk"
	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

// 全局采样率与声道固定（与 dsc-notify 一致），文件解码后统一重采样到该格式播放。
const (
	globalSampleRate = 44100
	globalChannels   = 2
	bytesPerSample   = 2 // 16bit
)

var (
	globalCtx *oto.Context

	pbMu     sync.Mutex
	pbCancel context.CancelFunc // 当前播放任务取消句柄；新播放先取消旧任务
)

// ---------- 播放状态控制 ----------

func stopCurrentPlayback() {
	pbMu.Lock()
	defer pbMu.Unlock()
	if pbCancel != nil {
		pbCancel()
	}
}

// playTrack 播放单个文件（已重采样为 44100/2ch/16bit PCM）。异步：Play 后等待播放时长，
// 期间可被 ctx 取消（立即停止）。返回 context.Canceled 表示被主动停止。
func playTrack(ctx context.Context, path string, vol float64) error {
	pcm, err := decodeFile(path)
	if err != nil {
		return err
	}
	player := globalCtx.NewPlayer(bytes.NewReader(pcm))
	player.SetVolume(vol)
	player.Play()

	bytesPerSecond := globalSampleRate * globalChannels * bytesPerSample
	dur := time.Duration(len(pcm)/bytesPerSecond) * time.Second
	if dur == 0 {
		dur = 1 * time.Second
	}

	select {
	case <-ctx.Done():
		player.Close()
		return ctx.Err()
	case <-time.After(dur):
	}
	// 缓冲尾部排空，避免结尾被截断
	select {
	case <-ctx.Done():
	default:
		time.Sleep(200 * time.Millisecond)
	}
	player.Close()
	return nil
}

// runLoop 按循环策略顺序播放文件列表（files 已过滤并排序）。
// loop=off 播完一轮即止；loop=one/list 循环播放直到被停止。
func runLoop(ctx context.Context, files []string, loop string, vol float64) {
	count := len(files)
	for i := 0; ; i++ {
		idx := i % count
		err := playTrack(ctx, files[idx], vol)
		if err == context.Canceled {
			log.Printf("⏹ 已停止播放")
			return
		}
		if err != nil {
			log.Printf("❌ 播放失败 %s: %v", files[idx], err)
		}
		// loop=off：一整轮（count 个文件）播完后自然结束
		if loop == "off" && idx == count-1 {
			log.Printf("✅ 播放队列结束（共 %d 首）", count)
			return
		}
	}
}

// ---------- 文件收集 ----------

// collectAudioFiles 解析 path：文件返回自身；目录返回其中 .mp3/.wav（升序排序）。
func collectAudioFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if !isSupported(filepath.Ext(path)) {
			return nil, fmt.Errorf("不支持的文件格式: %s（仅支持 .mp3/.wav）", filepath.Ext(path))
		}
		return []string{path}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !isSupported(filepath.Ext(e.Name())) {
			continue
		}
		files = append(files, filepath.Join(path, e.Name()))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("目录 %s 中没有 .mp3/.wav 文件", path)
	}
	sort.Strings(files)
	return files, nil
}

func isSupported(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".mp3" || ext == ".wav"
}

// ---------- 解码与重采样 ----------

// decodeFile 解码 mp3/wav 为统一的 44100Hz 立体声 16bit PCM。
func decodeFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp3":
		decoder, err := mp3.NewDecoder(f)
		if err != nil {
			return nil, fmt.Errorf("mp3 解码失败: %w", err)
		}
		raw, err := io.ReadAll(decoder)
		if err != nil {
			return nil, fmt.Errorf("读取 mp3 PCM 失败: %w", err)
		}
		return resample16Stereo(raw, decoder.SampleRate(), 2), nil
	case ".wav":
		raw, sampleRate, channels, err := decodeWAV(f)
		if err != nil {
			return nil, err
		}
		return resample16Stereo(raw, sampleRate, channels), nil
	}
	return nil, fmt.Errorf("不支持的文件格式: %s", ext)
}

// decodeWAV 解析 16bit PCM WAV，返回 PCM、采样率、声道数。
func decodeWAV(r io.Reader) ([]byte, int, int, error) {
	header := make([]byte, 12)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, 0, 0, fmt.Errorf("读取 RIFF 头失败: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("无效的 WAV 文件")
	}
	var sampleRate, channels int
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
			if binary.LittleEndian.Uint16(fmtData[0:2]) != 1 {
				return nil, 0, 0, fmt.Errorf("仅支持 PCM 格式 (format=1)")
			}
			channels = int(binary.LittleEndian.Uint16(fmtData[2:4]))
			if binary.LittleEndian.Uint16(fmtData[14:16]) != 16 {
				return nil, 0, 0, fmt.Errorf("仅支持 16bit 采样")
			}
			sampleRate = int(binary.LittleEndian.Uint32(fmtData[4:8]))
		case "data":
			pcm, err := io.ReadAll(io.LimitReader(r, int64(chunkSize)))
			if err != nil {
				return nil, 0, 0, fmt.Errorf("读取 data 失败: %w", err)
			}
			if channels < 1 || channels > 2 {
				return nil, 0, 0, fmt.Errorf("仅支持 1/2 声道，当前 %d 声道", channels)
			}
			return pcm, sampleRate, channels, nil
		default:
			if _, err := io.CopyN(io.Discard, r, int64(chunkSize)); err != nil {
				return nil, 0, 0, fmt.Errorf("跳过块 %s 失败: %w", chunkID, err)
			}
		}
	}
	return nil, 0, 0, fmt.Errorf("未找到 data 块")
}

// resample16Stereo 线性插值重采样任意采样率/单双声道 → 44100Hz 立体声 16bit。
func resample16Stereo(pcm []byte, inRate, inCh int) []byte {
	if inCh == 0 {
		inCh = 1
	}
	if inRate == globalSampleRate && inCh == globalChannels {
		return pcm
	}
	frames := len(pcm) / 2 / inCh
	outFrames := int(float64(frames) * float64(globalSampleRate) / float64(inRate))
	if outFrames < 1 {
		outFrames = 1
	}
	out := make([]byte, outFrames*globalChannels*bytesPerSample)
	for f := 0; f < outFrames; f++ {
		src := float64(f) * float64(inRate) / float64(globalSampleRate)
		i0 := int(src)
		if i0 >= frames {
			i0 = frames - 1
		}
		frac := src - float64(i0)
		i1 := i0 + 1
		if i1 >= frames {
			i1 = frames - 1
		}
		for ch := 0; ch < globalChannels; ch++ {
			sc := ch % inCh // 单声道两声道取同一源；立体声按左右
			s0 := int16(binary.LittleEndian.Uint16(pcm[(i0*inCh+sc)*2:]))
			s1 := int16(binary.LittleEndian.Uint16(pcm[(i1*inCh+sc)*2:]))
			v := float64(s0) + (float64(s1)-float64(s0))*frac
			binary.LittleEndian.PutUint16(out[(f*globalChannels+ch)*2:], uint16(int16(v)))
		}
	}
	return out
}

// ---------- 工具注册 ----------

func main() {
	sdk := dsc.New(dsc.Config{Name: "musicplayer", Version: "1.0.0", Type: dsc.TypeTool})

	sdk.Tool(dsc.Tool{
		Name:        "music_play",
		Description: "后台播放背景音乐（mp3/wav），异步播放、不阻塞其它工作。path 为单个音频文件或包含 .mp3/.wav 的目录；loop 取 off(播完即止)/one(单曲循环)/list(目录列表循环)；volume 为音量百分比 0-100。播放即返回，可随时用 music_stop 停止。",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path":   {"type": "string", "description": "音频文件路径或包含 .mp3/.wav 的目录"},
				"loop":   {"type": "string", "enum": ["off", "one", "list"], "description": "off=播完即止; one=单曲循环(文件)/单曲重复; list=列表循环(目录整列表循环)", "default": "off"},
				"volume": {"type": "integer", "minimum": 0, "maximum": 100, "description": "音量百分比 0-100，默认 100", "default": 100}
			},
			"required": ["path"]
		}`),
		Handler: func(_ context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Path   string `json:"path"`
				Loop   string `json:"loop"`
				Volume int    `json:"volume"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if p.Path == "" {
				return "", fmt.Errorf("须提供 path（音频文件或目录）")
			}
			if p.Loop == "" {
				p.Loop = "off"
			}
			if p.Loop != "off" && p.Loop != "one" && p.Loop != "list" {
				return "", fmt.Errorf("loop 仅支持 off/one/list，当前为 %q", p.Loop)
			}
			if p.Volume < 0 || p.Volume > 100 {
				return "", fmt.Errorf("volume 须在 0-100 之间，当前为 %d", p.Volume)
			}
			if globalCtx == nil {
				return "", fmt.Errorf("音频设备不可用（可能无音频，或 DSC_MUSICPLAYER_NO_AUDIO=1）")
			}

			files, err := collectAudioFiles(p.Path)
			if err != nil {
				return "", err
			}

			// 替换正在播放的任务
			pbMu.Lock()
			if pbCancel != nil {
				pbCancel()
			}
			ctx, cancel := context.WithCancel(context.Background())
			pbCancel = cancel
			pbMu.Unlock()

			vol := float64(p.Volume) / 100.0
			if p.Volume == 0 {
				vol = 0
			}
			go runLoop(ctx, files, p.Loop, vol)

			src := p.Path
			if info, err := os.Stat(p.Path); err == nil && info.IsDir() {
				src = fmt.Sprintf("%s（%d 首）", p.Path, len(files))
			}
			return fmt.Sprintf("🎵 开始播放 %s（loop=%s, 音量=%d%%）", src, p.Loop, p.Volume), nil
		},
	})

	sdk.Tool(dsc.Tool{
		Name:        "music_stop",
		Description: "停止当前后台音乐播放（若有）。",
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			stopCurrentPlayback()
			return "⏹ 已停止音乐播放", nil
		},
	})

	sdk.OnStart(func(ctx context.Context) error {
		if os.Getenv("DSC_MUSICPLAYER_NO_AUDIO") == "1" {
			log.Println("⚠️ DSC_MUSICPLAYER_NO_AUDIO=1，跳过音频初始化")
			return nil
		}
		audioCtx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   globalSampleRate,
			ChannelCount: globalChannels,
			Format:       oto.FormatSignedInt16LE,
		})
		if err != nil {
			return fmt.Errorf("初始化音频失败: %w", err)
		}
		globalCtx = audioCtx
		<-ready
		log.Println("✅ 音乐播放器就绪（44100Hz 立体声）")
		return nil
	})

	sdk.Serve()
}
