package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
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

	statusMu sync.Mutex
	playStat playStatus // 当前播放状态（由 runLoop/music_play 维护、music_status 查询）
)

// playStatus 记录当前播放状态，供 music_status 工具查询。
type playStatus struct {
	playing bool
	loop    string
	shuffle bool
	volume  int
	src     string        // 来源展示串（文件或「目录（N 首）」）
	song    string        // 当前曲目完整路径
	total   time.Duration // 当前曲目总时长
	started time.Time     // 当前曲目开始播放时刻（计算已播时长）
}

// setPlayStat 在锁内更新播放状态。
func setPlayStat(fn func(*playStatus)) {
	statusMu.Lock()
	defer statusMu.Unlock()
	fn(&playStat)
}

// MusicStatusResult music_status 工具的返回结构。
type MusicStatusResult struct {
	Playing  bool   `json:"playing"`
	Loop     string `json:"loop,omitempty"`
	Shuffle  bool   `json:"shuffle,omitempty"`
	Volume   int    `json:"volume,omitempty"`
	Song     string `json:"song,omitempty"` // 当前曲目文件名（不含路径）
	Path     string `json:"path,omitempty"`
	Src      string `json:"src,omitempty"`
	Duration int64  `json:"duration_seconds,omitempty"` // 当前曲目总时长（秒）
	Elapsed  int64  `json:"elapsed_seconds,omitempty"`  // 已播时长（秒）
}

// ---------- 默认播放目录 ----------

// defaultSrcFile 持久化默认播放目录的路径（存于用户主目录，重启后仍生效）。
func defaultSrcFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".dsc-musicplayer-src"
	}
	return filepath.Join(home, ".dsc", "musicplayer_src.txt")
}

// saveDefaultSrc 写入默认播放目录；目录不变时跳过写盘。
func saveDefaultSrc(path string) error {
	clean := filepath.Clean(path)
	if info, err := os.Stat(clean); err != nil || !info.IsDir() {
		return fmt.Errorf("默认播放目录不存在: %s", clean)
	}
	file := defaultSrcFile()
	if old := loadDefaultSrc(); old == clean {
		return nil
	}
	os.MkdirAll(filepath.Dir(file), 0o755)
	return os.WriteFile(file, []byte(clean), 0o644)
}

// loadDefaultSrc 读取默认播放目录：环境变量优先，其次文件。
func loadDefaultSrc() string {
	if env := strings.TrimSpace(os.Getenv("DSC_MUSICPLAYER_SRC")); env != "" {
		return env
	}
	data, err := os.ReadFile(defaultSrcFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// ---------- 播放状态控制 ----------

func stopCurrentPlayback() {
	pbMu.Lock()
	defer pbMu.Unlock()
	if pbCancel != nil {
		pbCancel()
	}
}

// pcmDuration 由统一 44100/2ch/16bit PCM 字节数计算精确播放时长。
// 用浮点运算，避免整型除法截断小数秒导致每首被截短近 1 秒。
func pcmDuration(pcm []byte) time.Duration {
	bytesPerSecond := globalSampleRate * globalChannels * bytesPerSample
	return time.Duration(float64(len(pcm)) / float64(bytesPerSecond) * float64(time.Second))
}

// playPCM 播放已解码的 44100/2ch/16bit PCM。异步：Play 后等待精确播放时长，
// 再等内部缓冲排空（IsPlaying 转 false），保证结尾完整且不引入固定等待；
// 期间可被 ctx 取消（立即停止）。返回 context.Canceled 表示被主动停止。
func playPCM(ctx context.Context, pcm []byte, vol float64) error {
	player := globalCtx.NewPlayer(bytes.NewReader(pcm))
	player.SetVolume(vol)
	player.Play()
	// oto v3.4 的 Player.Close 是 no-op（清理靠 finalizer 回收，时机不可控），
	// 退出时须 Pause 才能真正立即停止输出——否则已缓冲的整首 PCM 会一直播完，
	// 表现为「停止只能在当前歌曲播完后才生效」。Pause 后 mux 对该 player 混音
	// 返回 0（静音），对已自然播完的 player 调用是 no-op。
	defer player.Pause()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(pcmDuration(pcm)):
	}
	// 缓冲尾部排空：等 IsPlaying 变 false（数据已全部交给驱动），避免结尾被截断
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !player.IsPlaying() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// decodedTrack 一首已解码曲目（含解码错误）。
type decodedTrack struct {
	pcm []byte
	err error
}

// runLoop 按循环策略顺序播放文件列表（files 已过滤并排序）。
// 下一首在后台预解码（prefetch），消除曲间因解码造成的静默等待。
// loop=off 播完一轮即止；loop=one/list 循环播放直到被停止。
// 每曲播放前更新 playStat（供 music_status 查询）；被停止/结束时置 playing=false。
func runLoop(ctx context.Context, files []string, loop string, volumePercent int) {
	count := len(files)
	vol := float64(volumePercent) / 100.0

	// prefetchCh 容量 1：后台预解码下一首，播放当前期间即完成解码
	prefetchCh := make(chan decodedTrack, 1)
	prefetch := func(idx int) {
		go func() {
			if ctx.Err() != nil {
				return
			}
			pcm, err := decodeFile(files[idx])
			select {
			case <-ctx.Done():
			case prefetchCh <- decodedTrack{pcm: pcm, err: err}:
			}
		}()
	}

	// 第一首同步解码，保证立即开播
	pcm, err := decodeFile(files[0])
	cur := decodedTrack{pcm: pcm, err: err}

	for i := 0; ; i++ {
		idx := i % count

		// 预解码下一首（loop=off 的最后一首无需预取）
		if !(loop == "off" && idx == count-1) {
			prefetch((i + 1) % count)
		}

		if cur.err != nil {
			log.Printf("❌ 播放失败 %s: %v", files[idx], cur.err)
		} else {
			setPlayStat(func(s *playStatus) {
				s.playing = true
				s.loop = loop
				s.volume = volumePercent
				s.song = files[idx]
				s.total = pcmDuration(cur.pcm)
				s.started = time.Now()
			})
			if err := playPCM(ctx, cur.pcm, vol); err == context.Canceled {
				setPlayStat(func(s *playStatus) { s.playing = false; s.song = "" })
				log.Printf("⏹ 已停止播放")
				return
			} else if err != nil {
				log.Printf("❌ 播放失败 %s: %v", files[idx], err)
			}
		}

		// loop=off：一整轮（count 个文件）播完后自然结束
		if loop == "off" && idx == count-1 {
			setPlayStat(func(s *playStatus) { s.playing = false; s.song = "" })
			log.Printf("✅ 播放队列结束（共 %d 首）", count)
			return
		}

		// 取下首预解码结果；被停止时立即返回，避免阻塞在 prefetchCh 上导致
		// runLoop 泄漏（prefetch 协程遇 ctx 取消不会向 channel 发送）。
		select {
		case cur = <-prefetchCh:
		case <-ctx.Done():
			setPlayStat(func(s *playStatus) { s.playing = false; s.song = "" })
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
		Description: "后台播放背景音乐（mp3/wav），异步播放、不阻塞其它工作。path 为单个音频文件或包含 .mp3/.wav 的目录；可省略以使用默认播放目录（用 music_setdir 预先设定）。loop 取 off(播完即止)/one(单曲循环)/list(目录列表循环)；shuffle 为 true 时随机打乱播放顺序（仅目录有意义）；volume 为音量百分比 1-100（省略或 0 为默认 100，-1 为显式静音）。播放即返回，可随时用 music_stop 停止。",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path":   {"type": "string", "description": "音频文件路径或目录；省略则用默认播放目录（music_setdir 设定）"},
				"loop":   {"type": "string", "enum": ["off", "one", "list"], "description": "off=播完即止; one=单曲循环(文件)/单曲重复; list=列表循环(目录整列表循环)", "default": "off"},
				"shuffle":{"type": "boolean", "description": "随机播放：打乱列表顺序（对多个文件的目录有意义），默认 false 顺序播放", "default": false},
				"volume": {"type": "integer", "minimum": -1, "maximum": 100, "description": "音量百分比 1-100；省略或 0 为默认 100；-1 为显式静音（仍播放但无声）", "default": 100}
			}
		}`),
		Handler: func(_ context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Path    string `json:"path"`
				Loop    string `json:"loop"`
				Shuffle bool   `json:"shuffle"`
				Volume  *int   `json:"volume"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if p.Path == "" {
				p.Path = loadDefaultSrc()
				if p.Path == "" {
					return "", fmt.Errorf("未提供 path，也未设定默认播放目录（请提供 path 或先用 music_setdir 设定）")
				}
			}
			if p.Loop == "" {
				p.Loop = "off"
			}
			if p.Loop != "off" && p.Loop != "one" && p.Loop != "list" {
				return "", fmt.Errorf("loop 仅支持 off/one/list，当前为 %q", p.Loop)
			}
			if globalCtx == nil {
				return "", fmt.Errorf("音频设备不可用（可能无音频，或 DSC_MUSICPLAYER_NO_AUDIO=1）")
			}

			// 音量语义：省略/0 → 默认 100%（Go 的 json.Unmarshal 不套用 JSON Schema 的
			// default，需显式回退，否则首播会静音）；-1 → 显式静音；其余越界值钳制
			// 到上限 100% / 最小可闻 1%，不报错。
			volume := 100
			if p.Volume != nil {
				switch {
				case *p.Volume == -1:
					volume = 0 // 显式静音（仍播放但无声）
				case *p.Volume == 0:
					volume = 100 // 0 与省略等价，默认 100
				case *p.Volume > 100:
					volume = 100 // 超大按上限
				case *p.Volume < -1:
					volume = 1 // 超小按下限
				default:
					volume = *p.Volume // 1-100 原样
				}
			}

			files, err := collectAudioFiles(p.Path)
			if err != nil {
				return "", err
			}
			// 随机播放：Fisher-Yates 打乱列表顺序（仅对多文件目录有意义；
			// 单文件打乱无影响）。loop 沿用：shuffle+list 随机序循环、shuffle+off 随机一轮。
			if p.Shuffle && len(files) > 1 {
				rand.Shuffle(len(files), func(i, j int) { files[i], files[j] = files[j], files[i] })
			} else {
				p.Shuffle = false // 单文件或无列表时不视为随机，避免状态误报
			}

			// 替换正在播放的任务
			pbMu.Lock()
			if pbCancel != nil {
				pbCancel()
			}
			ctx, cancel := context.WithCancel(context.Background())
			pbCancel = cancel
			pbMu.Unlock()

			src := p.Path
			if info, err := os.Stat(p.Path); err == nil && info.IsDir() {
				src = fmt.Sprintf("%s（%d 首）", p.Path, len(files))
			}
			setPlayStat(func(s *playStatus) { s.src = src; s.shuffle = p.Shuffle })
			go runLoop(ctx, files, p.Loop, volume)

			volDesc := fmt.Sprintf("%d%%", volume)
			if p.Volume != nil && *p.Volume == -1 {
				volDesc = "静音(-1)"
			}
			modeDesc := "顺序"
			if p.Shuffle {
				modeDesc = "随机"
			}
			return fmt.Sprintf("🎵 开始播放 %s（loop=%s, %s, 音量=%s）", src, p.Loop, modeDesc, volDesc), nil
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

	sdk.Tool(dsc.Tool{
		Name:        "music_status",
		Description: "查询当前音乐播放状态：playing 是否在播；loop 播放模式 off/one/list；shuffle 是否随机播放；song 当前曲目文件名、path 完整路径、duration_seconds 曲目总时长、elapsed_seconds 已播时长（未在播为 0）；volume 音量 1-100；src 播放来源。未在播放时 playing=false、song 为空。",
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			statusMu.Lock()
			s := playStat
			statusMu.Unlock()
			res := MusicStatusResult{
				Playing:  s.playing,
				Loop:     s.loop,
				Shuffle:  s.shuffle,
				Volume:   s.volume,
				Song:     filepath.Base(s.song),
				Path:     s.song,
				Src:      s.src,
				Duration: int64(s.total.Seconds()),
			}
			if s.playing && !s.started.IsZero() {
				el := time.Since(s.started)
				if el > s.total {
					el = s.total
				}
				res.Elapsed = int64(el.Seconds())
			}
			b, _ := json.Marshal(res)
			return string(b), nil
		},
	})

	sdk.Tool(dsc.Tool{
		Name:        "music_setdir",
		Description: "设定默认播放目录并持久化（重启后仍生效），之后 music_play 可省略 path 直接播放该目录下的 mp3/wav。path 必须为存在的目录。",
		Schema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "要设为默认播放目录的路径"}
			},
			"required": ["path"]
		}`),
		Handler: func(_ context.Context, args json.RawMessage) (string, error) {
			var p struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("参数解析失败: %w", err)
			}
			if err := saveDefaultSrc(p.Path); err != nil {
				return "", err
			}
			return fmt.Sprintf("已设定默认播放目录: %s", filepath.Clean(p.Path)), nil
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
