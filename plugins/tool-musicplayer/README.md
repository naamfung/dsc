# musicplayer（DSC 音乐播放工具插件）

DSC 宿主上的 tool 类型插件：提供音乐/音频播放工具，支持播放内置音效与本地 MP3/WAV
文件，全局采样率统一为 44100Hz 立体声 16bit。

## 平台说明

本插件依赖 `github.com/ebitengine/oto/v3` 音频库输出 PCM。

- 受支持平台：目标平台集七端全部支持（`darwin/amd64`、`darwin/arm64`、
  `windows/amd64`、`linux/amd64`、`linux/arm64`、`linux/loong64`、`freebsd/amd64`）。
  oto v3.5.0 起 Unix 侧全部为纯 Go 驱动：Linux/FreeBSD 默认走 PulseAudio（纯 Go
  客户端），失败时回退 ALSA（purego 运行时动态加载 `libasound.so.2`），编译期不再
  需要 ALSA 开发头文件与 pkg-config，`CGO_ENABLED=0` 即可交叉编译全部七端。
- `freebsd/amd64`：以 `CGO_ENABLED=0` 纯 Go 交叉编译时，因 purego fakecgo 的
  限制须附加 `-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std"`。
- 运行时依赖：Linux/FreeBSD 播放声音须系统存在 PulseAudio 服务或 ALSA 运行时库
  （`libasound.so.2`），两者皆无时音频初始化报错；可用 `DSC_MUSICPLAYER_NO_AUDIO=1`
  跳过音频初始化。
