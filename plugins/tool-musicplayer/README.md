# musicplayer（DSC 音樂播放工具插件）

DSC 宿主上的 tool 類型插件：提供音樂/音頻播放工具，支持播放內置音效與本地 MP3/WAV
文件，全局採樣率統一為 44100Hz 立體聲 16bit。

## 平台説明

本插件依賴 `github.com/ebitengine/oto/v3` 音頻庫輸出 PCM。

- 受支持平台：目標平台集七端全部支持（`darwin/amd64`、`darwin/arm64`、
  `windows/amd64`、`linux/amd64`、`linux/arm64`、`linux/loong64`、`freebsd/amd64`）。
  oto v3.5.0 起 Unix 側全部為純 Go 驅動：Linux/FreeBSD 默認走 PulseAudio（純 Go
  客户端），失敗時回退 ALSA（purego 運行時動態加載 `libasound.so.2`），編譯期不再
  需要 ALSA 開發頭文件與 pkg-config，`CGO_ENABLED=0` 即可交叉編譯全部七端。
- `freebsd/amd64`：以 `CGO_ENABLED=0` 純 Go 交叉編譯時，因 purego fakecgo 的
  限制須附加 `-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std"`。
- 運行時依賴：Linux/FreeBSD 播放聲音須系統存在 PulseAudio 服務或 ALSA 運行時庫
  （`libasound.so.2`），兩者皆無時音頻初始化報錯；可用 `DSC_MUSICPLAYER_NO_AUDIO=1`
  跳過音頻初始化。
