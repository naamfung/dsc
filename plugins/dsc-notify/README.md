# notify（DSC 通知音效插件）

DSC 宿主上的通用（dsc 类型）插件：不注册任何模型可调用的工具，仅经 `Hook.OnEvent`
订阅宿主事件，在 agent 回合完成（成功 / 失败）时程序性播放对应音效，完全由宿主事件
驱动、不依赖模型调用。

## 平台说明

本插件依赖 `github.com/ebitengine/oto/v3` 音频库播放 PCM 内置音效。

- 受支持平台：目标平台集七端全部支持（`darwin/amd64`、`darwin/arm64`、
  `windows/amd64`、`linux/amd64`、`linux/arm64`、`linux/loong64`、`freebsd/amd64`）。
  oto v3.5.0 起 Unix 侧全部为纯 Go 驱动：Linux/FreeBSD 默认走 PulseAudio（纯 Go
  客户端），失败时回退 ALSA（purego 运行时动态加载 `libasound.so.2`），编译期不再
  需要 ALSA 开发头文件与 pkg-config，`CGO_ENABLED=0` 即可交叉编译全部七端。
- `freebsd/amd64`：以 `CGO_ENABLED=0` 纯 Go 交叉编译时，因 purego fakecgo 的
  限制须附加 `-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std"`。
- 运行时依赖：Linux/FreeBSD 播放音效须系统存在 PulseAudio 服务或 ALSA 运行时库
  （`libasound.so.2`），两者皆无时音频初始化报错。

需要无声模式时，设置环境变量 `DSC_NOTIFY_NO_AUDIO=1` 可跳过音频上下文初始化，仅保留
事件日志。
