package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// setUserHome 临时把 Windows 主目录切换到 temp 目录，测试结束后还原，
// 让默认播放目录的持久化读写落在独立环境下，不污染真实主目录。
func setUserHome(t *testing.T) string {
	t.Helper()
	key := "USERPROFILE"
	prev, had := os.LookupEnv(key)
	home := t.TempDir()
	if err := os.Setenv(key, home); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
	return home
}

// ---------- collectAudioFiles ----------

func TestCollectAudioFilesDir(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"b.wav", "a.mp3", "c.txt", "sub"} {
		p := filepath.Join(dir, name)
		if name == "sub" {
			os.MkdirAll(p, 0o755)
		} else {
			os.WriteFile(p, []byte("x"), 0o644)
		}
	}
	files, err := collectAudioFiles(dir)
	if err != nil {
		t.Fatalf("collect dir: %v", err)
	}
	want := []string{
		filepath.Join(dir, "a.mp3"),
		filepath.Join(dir, "b.wav"),
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("got %v, want %v", files, want)
	}
}

func TestCollectAudioFilesSingleAndErrors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x.mp3")
	os.WriteFile(f, []byte("x"), 0o644)
	files, err := collectAudioFiles(f)
	if err != nil || len(files) != 1 || files[0] != f {
		t.Fatalf("single file: files=%v err=%v", files, err)
	}
	if _, err := collectAudioFiles(filepath.Join(t.TempDir(), "nope.mp3")); err == nil {
		t.Fatalf("missing file should error")
	}
	if _, err := collectAudioFiles(filepath.Join(t.TempDir(), "y.txt")); err == nil {
		t.Fatalf("unsupported single file should error")
	}
	empty := t.TempDir()
	if _, err := collectAudioFiles(empty); err == nil {
		t.Fatalf("empty dir should error")
	}
}

// ---------- resample16Stereo ----------

func TestResampleIdentityAndMonoUpmix(t *testing.T) {
	// 44100 stereo ⇒ 原样返回（避免无谓拷贝差异）
	stereoFrames := 8
	input := make([]byte, stereoFrames*2*2)
	for f := 0; f < stereoFrames; f++ {
		binary.LittleEndian.PutUint16(input[f*4:], uint16(int16(f)))
		binary.LittleEndian.PutUint16(input[f*4+2:], uint16(int16(-f)))
	}
	if out := resample16Stereo(input, 44100, 2); !bytes.Equal(out, input) {
		t.Fatalf("44100 stereo resample should be identity, got different bytes")
	}

	// 单声道 ⇒ 立体声左右完全一致
	mono := make([]byte, 4)
	for i := range mono {
		mono[i] = byte(i + 1)
	}
	out := resample16Stereo(mono, 44100, 1)
	left := binary.LittleEndian.Uint16(out[0:2])
	right := binary.LittleEndian.Uint16(out[2:4])
	if left != right {
		t.Fatalf("mono upmix: left=%d right=%d should be equal", left, right)
	}
}

// ---------- decodeWAV ----------

func TestDecodeWAV(t *testing.T) {
	data := []int16{100, -100, 200, -200}

	var wav bytes.Buffer
	wav.WriteString("RIFF")
	binary.Write(&wav, binary.LittleEndian, uint32(36+len(data)*2))
	wav.WriteString("WAVE")
	wav.WriteString("fmt ")
	binary.Write(&wav, binary.LittleEndian, uint32(16))
	binary.Write(&wav, binary.LittleEndian, uint16(1))         // audioFormat = PCM
	binary.Write(&wav, binary.LittleEndian, uint16(2))         // channels = 2
	binary.Write(&wav, binary.LittleEndian, uint32(44100))     // sampleRate
	binary.Write(&wav, binary.LittleEndian, uint32(44100*2*2)) // byteRate
	binary.Write(&wav, binary.LittleEndian, uint16(2))         // blockAlign
	binary.Write(&wav, binary.LittleEndian, uint16(16))        // bitsPerSample
	wav.WriteString("data")
	binary.Write(&wav, binary.LittleEndian, uint32(len(data)*2))
	for _, s := range data {
		binary.Write(&wav, binary.LittleEndian, s)
	}

	pcm, rate, channels, err := decodeWAV(bytes.NewReader(wav.Bytes()))
	if err != nil {
		t.Fatalf("decodeWAV: %v", err)
	}
	if rate != 44100 || channels != 2 {
		t.Fatalf("rate=%d channels=%d, want 44100/2", rate, channels)
	}
	if len(pcm) != len(data)*2 {
		t.Fatalf("pcm len=%d, want %d", len(pcm), len(data)*2)
	}
	first := int16(binary.LittleEndian.Uint16(pcm[0:2]))
	if first != 100 {
		t.Fatalf("first sample=%d, want 100", first)
	}
}

// ---------- pcmDuration ----------

func TestPCMDurationNoTruncation(t *testing.T) {
	bytesPerSecond := globalSampleRate * globalChannels * bytesPerSample
	// 10.5 秒的 PCM：整型除法曾截成 10s，现应精确返回 10.5s
	pcm := make([]byte, int(10.5*float64(bytesPerSecond)))
	dur := pcmDuration(pcm)
	if got := dur.Seconds(); math.Abs(got-10.5) > 1e-6 {
		t.Fatalf("duration = %v (%.3fs), want 10.5s", dur, got)
	}
	// 空 PCM 时长应为 0
	if d := pcmDuration(nil); d != 0 {
		t.Fatalf("empty pcm duration = %v, want 0", d)
	}
}

// ---------- 默认播放目录持久化 ----------

func TestDefaultSrcPersistence(t *testing.T) {
	os.Unsetenv("DSC_MUSICPLAYER_SRC")
	setUserHome(t)

	dir := t.TempDir()
	if err := saveDefaultSrc(dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	if got := loadDefaultSrc(); got != filepath.Clean(dir) {
		t.Fatalf("load after save = %q, want %q", got, filepath.Clean(dir))
	}
	data, err := os.ReadFile(defaultSrcFile())
	if err != nil {
		t.Fatalf("default src file not written: %v", err)
	}
	if string(data) != filepath.Clean(dir) {
		t.Fatalf("file content = %q, want %q", string(data), filepath.Clean(dir))
	}
}

func TestDefaultSrcInvalidDir(t *testing.T) {
	setUserHome(t)
	if err := saveDefaultSrc(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatalf("non-existent dir should fail")
	}
}
