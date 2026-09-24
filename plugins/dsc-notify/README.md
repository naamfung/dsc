# notify（DSC 通知音效插件）

DSC 宿主上的通用（dsc 類型）插件：不註冊任何模型可調用的工具，僅經 `Hook.OnEvent`
訂閲宿主事件，在 agent 回合完成（成功 / 失敗）時程序性播放對應音效，完全由宿主事件
驅動、不依賴模型調用。

## 平台説明

本插件依賴 `github.com/ebitengine/oto/v3` 音頻庫播放 PCM 內置音效。

- 受支持平台：目標平台集七端全部支持（`darwin/amd64`、`darwin/arm64`、
  `windows/amd64`、`linux/amd64`、`linux/arm64`、`linux/loong64`、`freebsd/amd64`）。
  oto v3.5.0 起 Unix 側全部為純 Go 驅動：Linux/FreeBSD 默認走 PulseAudio（純 Go
  客户端），失敗時回退 ALSA（purego 運行時動態加載 `libasound.so.2`），編譯期不再
  需要 ALSA 開發頭文件與 pkg-config，`CGO_ENABLED=0` 即可交叉編譯全部七端。
- `freebsd/amd64`：以 `CGO_ENABLED=0` 純 Go 交叉編譯時，因 purego fakecgo 的
  限制須附加 `-gcflags="github.com/ebitengine/purego/internal/fakecgo=-std"`。
- 運行時依賴：Linux/FreeBSD 播放音效須系統存在 PulseAudio 服務或 ALSA 運行時庫
  （`libasound.so.2`），兩者皆無時音頻初始化報錯。

需要無聲模式時，設置環境變量 `DSC_NOTIFY_NO_AUDIO=1` 可跳過音頻上下文初始化，僅保留
事件日誌。
