# notify（DSC 通知音效插件）

DSC 宿主上的通用（dsc 类型）插件：不注册任何模型可调用的工具，仅经 `Hook.OnEvent`
订阅宿主事件，在 agent 回合完成（成功 / 失败）时程序性播放对应音效，完全由宿主事件
驱动、不依赖模型调用。

## 平台说明

本插件依赖 `github.com/ebitengine/oto/v3` 音频库播放 PCM 内置音效。

- 受支持平台：`darwin/amd64`、`darwin/arm64`、`windows/amd64`——这些平台由 oto
  以纯 Go driver（AudioToolbox / WASAPI）实现，`CGO_ENABLED=0` 即可交叉编译，已通过
  目标平台集各平台 `go build` / `go vet` 校验。
- `linux/amd64`、`linux/arm64`、`linux/loong64`：oto 在 Linux 走 ALSA driver，
  `driver_unix.go` 为 CGO 文件（`#cgo pkg-config: alsa`），须在安装 ALSA 开发头文件与
  pkg-config 的目标机上以 `CGO_ENABLED=1` 构建；裸 `go build`（CGO_ENABLED=0）交叉编
  译不含该 driver，无法离线校验。
- `freebsd/amd64`：oto 无对应 FreeBSD 音频 driver，本插件不支持该平台。

需要无声模式时，设置环境变量 `DSC_NOTIFY_NO_AUDIO=1` 可跳过音频上下文初始化，仅保留
事件日志。
