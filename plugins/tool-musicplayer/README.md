# musicplayer（DSC 音乐播放工具插件）

DSC 宿主上的 tool 类型插件：提供音乐/音频播放工具，支持播放内置音效与本地 MP3/WAV
文件，全局采样率统一为 44100Hz 立体声 16bit。

## 平台说明

本插件依赖 `github.com/ebitengine/oto/v3` 音频库输出 PCM。

- 受支持平台：`darwin/amd64`、`darwin/arm64`、`windows/amd64`——这些平台由 oto
  以纯 Go driver（AudioToolbox / WASAPI）实现，`CGO_ENABLED=0` 即可交叉编译，已通过
  目标平台集各平台 `go build` / `go vet` 校验。
- `linux/amd64`、`linux/arm64`、`linux/loong64`：oto 在 Linux 走 ALSA driver，
  `driver_unix.go` 为 CGO 文件（`#cgo pkg-config: alsa`），须在安装 ALSA 开发头文件与
  pkg-config 的目标机上以 `CGO_ENABLED=1` 构建；裸 `go build`（CGO_ENABLED=0）交叉编
  译不含该 driver，无法离线校验。
- `freebsd/amd64`：oto 无对应 FreeBSD 音频 driver，本插件不支持该平台。
