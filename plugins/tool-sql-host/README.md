# tool-sql-host（.dsp 插件宿主）

以 **`.dsp`（dsc's plugin）文件**為載體加載插件的工具插件：一個 `.dsp` 就是一個
SQLite 數據庫，插件的**元數據、代碼與運行期狀態同處一庫**——「程序即數據」。
宿主（本插件）負責打開它、取出代碼執行、把其中註冊的工具暴露給模型；插件運行期
讀寫的數據就是它自己那個文件。

思路源自 [SelfDB / SELF 格式](https://github.com/fzakaria/selfdb)（把 ELF 變成可執行
的 SQLite 庫），但去掉了內核層：這裏不需要可執行位、不需要 `binfmt_misc`，執行主體
是 Go 宿主進程，文件只承載內容。

## 目錄約定與加載

```
<ExecDir>/dsp/*.dsp                     頂層目錄：運行中創建出來的插件
<ExecDir>/plugins/tool-sql-host/dsp/*.dsp  插件自帶目錄：隨包分發的示例與預置插件
```

發佈包（builder 產物）會把插件目錄下的 `dsp/` 與 `examples/` 一併拷入
`plugins/tool-sql-host/`：前者是可用的示例載體（開箱即被加載），後者是它的源目錄，
充當包內的格式説明（發佈包不隨附各插件 README）。

- 只認後綴為 `.dsp` 的普通文件（SQLite 的 `-journal`/`-wal`/`-shm` 伴生文件天然被排除）。
- 每個 `.dsp` 一個獨立 LUA VM；插件內腳本經 `dsc.register_tool` 註冊的工具以
  `sql_` 前綴暴露給宿主，避免與其他插件重名。
- **熱加載**：輪詢（2s）比對**產物哈希**（覆蓋元數據與代碼 blob，不含狀態），
  變化即卸載重載。插件寫自己的狀態不會觸發重載——這正是狀態能與代碼同居一庫的前提。
- **創造模式門控**：非創造模式（`DSC_MODE`）下僅加載啓動時已存在的插件，禁用熱加載
  輪詢與 `pack_dsp`（對齊 tool-lua-host 的「插件創造僅在創造模式允許」）。

不長期持有庫句柄（每次操作開短連接），原因有三：狀態就寫在載體裏，長期持有會讓文件
在 Windows 上無法被替換（重打包熱更新）；釋放句柄後作者可用 `sqlite3` CLI 等外部工具
直接檢查/修改；單次開庫開銷遠小於一次工具調用。只讀文件會被識別為只讀插件：讀操作
照常，寫狀態給出明確錯誤而不是靜默丟棄。

## `.dsp` 格式

強制不變量（`internal/dsp` 在打開時逐條校驗，任一不滿足即拒絕加載）：

1. 路徑後綴必須是 `.dsp`（大小寫不敏感）。
2. 文件頭 16 字節必須是 SQLite 3 魔數。
3. SQLite `application_id` 必須是 `0x44535031`（ASCII `DSP1`）——後綴之外靠庫頭自證身份。
4. `dsp_meta` 表必須存在且含 `name`（插件名）與 `language`（載體語言）。

保留表（統一 `dsp_` 前綴，避免與作者自建表衝突）：

| 表 | 作用 |
| :--- | :--- |
| `dsp_meta` | 插件元數據：`name`/`language`/`entry`/`description`/`version`/`schema` |
| `dsp_blobs` | 代碼與靜態內容：`kind` = `script`（可執行、可 `require`）/ `asset`（可讀） |
| `dsp_state` | 插件自持狀態（KV，`dsc.store.*` 的落點） |
| `dsp_log` | 插件日誌（可選，審計用） |

除保留表外，作者可在同一庫內自由建表（經 `schema.sql` 在打包期落庫）。

### 打包（目錄 → `.dsp`）

源目錄佈局：

```
examples/hello/
  dsp.yaml      清單（可選）：name / language / entry / description / version
  main.lua      入口腳本（blob 名 = entry，kind=script）
  lib.lua       其餘 *.lua → kind=script（可經 dsc.dsp.require 加載）
  data.txt      其餘文件 → kind=asset（可經 dsc.dsp.asset 讀取）
  schema.sql    作者自有表建表腳本（可選，打包期執行）
```

```bash
cd plugins/tool-sql-host
go run ./cmd/dsp-pack -o dsp/hello.dsp ./examples/hello
```

打包是**字節確定**的（同一份源目錄 → 逐字節相同的 `.dsp`），故倉庫內提交的示例載體
由 `TestShippedExampleDspMatchesSource` 與源目錄逐字節比對守住，避免「源碼改了、
載體沒重打包」。

創造模式下模型也可用 `pack_dsp` 工具完成同樣的事（源目錄 → `.dsp`），產物落在
`./dsp` 下即被自動加載。

## 插件腳本可用的 `dsc.*` API

與 tool-lua-host 同源，另加**插件自身庫**相關內建：

| 內建 | 説明 |
| :--- | :--- |
| `dsc.register_tool(name, spec, fn)` | 註冊工具（暴露為 `sql_<name>`） |
| `dsc.store.get/set/delete(k[, v])` | 自持狀態 KV，落盤在自身 `.dsp` 的 `dsp_state`，重啓不丟 |
| `dsc.sql.query(stmt[, params])` | 對自己那個庫做查詢，返回行數組 |
| `dsc.sql.exec(stmt[, params])` | 對自己那個庫做寫操作，返回受影響行數 |
| `dsc.sql.tables()` | 列出自身庫內的表/視圖 |
| `dsc.dsp.require(name)` | 從同一 `.dsp` 加載另一個腳本 blob（模塊，只執行一次） |
| `dsc.dsp.asset(name)` | 讀取同一 `.dsp` 內的靜態內容 |
| `dsc.plugin.{name,path,readonly}` | 插件自述（自省與日誌標註） |
| `dsc.llm.chat / dsc.tool.call / dsc.tool.list / dsc.notify.emit` | 經宿主轉發到聚合 LLM / 任意工具 / 事件總線 |
| `dsc.hook.before_tool/after_tool/on_event` | 註冊宿主流水線鈎子 |
| `dsc.job.spawn/status/list` | 插件內後台任務 |

### SQL 守衞（`dsc.sql.*`）

腳本側 SQL 經保守靜態檢查，維護 `.dsp` 的兩條不變量：

- 禁止 `ATTACH`/`DETACH`：插件只能讀寫自身庫（單文件自持），不得掛載別的文件。
- 禁止運行期 DDL 與寫保留表 `dsp_meta`/`dsp_blobs`：插件結構屬打包期產物
  （見 `schema.sql`），運行期自改代碼後重載是隱性升級。
- 只允許單條語句；只讀 `PRAGMA` 可用，寫型 `PRAGMA` 拒絕。

這是守住格式不變量與可審計性的守衞，不是沙箱——插件腳本本就在宿主進程內執行。

## 平台説明

純 Go 實現（`modernc.org/sqlite`，無 CGO），目標平台集七端全部支持
（`linux/amd64`、`linux/arm64`、`linux/loong64`、`darwin/amd64`、`darwin/arm64`、
`windows/amd64`、`freebsd/amd64`），`CGO_ENABLED=0` 即可交叉編譯。

## 與 tool-lua-host 的差異

| 維度 | tool-lua-host | tool-sql-host |
| :--- | :--- | :--- |
| 載體 | `scripts/<name>/main.lua` 目錄 | 單個 `.dsp`（SQLite 庫）文件 |
| 分發 | 複製目錄 | 複製一個文件 |
| 狀態 | 進程內 KV（重啓即失） | `dsp_state`，隨載體落盤（重啓仍在） |
| 變更檢測 | `main.lua` 內容哈希 | 產物哈希（元數據 + 代碼，不含狀態） |
| 多文件代碼 | 目錄內文件 | `dsp_blobs` 內的多個 script blob（`dsc.dsp.require`） |
| 結構查詢 | 無 | SQL 直接查詢代碼、元數據與狀態 |
