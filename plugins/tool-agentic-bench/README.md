# agentic-bench（DSC 模型能力自動評分測試插件）

`tool-agentic-bench` 是 DSC 上的一組**模型能力評分測試台**（agentic-bench）工具：它把一組內置的運行時集成
測試用例暴露給模型，模型經真實工具逐個完成，插件進程自動判定每項、彙總計分並輸出
報告。評測對象是**模型自身**；同時因 file 類用例需模型驅動文件工具落盤，若全用例
通過也相當於對 DSC 的工具運行時做了集成驗證。

## 平台説明

- 純 Go + `dsc-sdk`，零 CGO，目標平台集七端全部交叉編譯通過
  （`linux/amd64`、`linux/arm64`、`linux/loong64`、`darwin/amd64`、
  `darwin/arm64`、`windows/amd64`、`freebsd/amd64`）。

## 工作原理與防作弊

- 每個用例只把**任務陳述**（Task）暴露給模型；期望答案與判定規則只存在於插件進程
  內部，絕不寫入工具描述 / `Context` / `ListContext`。`bench_next` 只下發任務文字，
  `bench_submit` 失敗時也僅回泛化原因、不透露期望值。插件還帶一條
  `bench_start` 的 Context 引導模型真實完成任務、不得向 bench 索要答案，否則判 FAIL。
- file 類用例由插件**直接讀取產物文件**判定（端態校驗），無需模型回傳內容，杜絕"猜答
  案"與"工具鏈路沒真正走通卻矇混過關"。
- 冪等 + 自動推進：同用例重複 `bench_submit` 返回既有結果、不重複計分；**交卷後插件立即
  在響應裏揭曉下一用例（`next`）**，已評分用例無法重試——這既保證每項僅計一次，也讓
  agent 循環強制前進，杜絕「弱模型反覆重交同一題」造成的死循環。

## 工具一覽

| 工具            | 用途                                                              |
| ---------------- | --------------------------------------------------------------- |
| `bench_start`    | 初始化一次評分：重置結果、創建產物目錄，返回用例總數與各用例 id/標題；並啓動**交卷預算**看門狗 |
| `bench_next`     | 取下一個未評分用例（含任務陳述）；全部完成返回 done:true        |
| `bench_submit`   | 提交某用例完成結果並評分（answer 帶 answer，file 由插件讀盤判定）；交卷後**自動推進到下一用例**（響應 next 直接給出下一題），已評分用例無法重試 |
| `bench_report`   | 自動計分彙總（總得分 + 成敗比 + 逐用例狀態表 + **總耗時**）並把 JSON 落盤（含總耗時與各用例單項耗時） |

> **交卷/超時預算**（都“設置才啓用”）：
> `DSC_BENCH_TIMEOUT`=全局預算（秒），**不設 = 無限制**，到期把剩餘未交卷用例標 skip 並
> 即時落盤報告；`DSC_BENCH_CASE_TIMEOUT`=每案例預算（秒），**默認關閉**，某題下發後超時
> 即標 skip 並自動推進到下一題。兩者可並用：單題超時跳過、整場超時兜底。
>
> **預算 = 兜底，不冤枉及時交卷**：超時觸發的 skip 只是“推進/保底”。若模型之後仍對該題
> 交卷，`bench_submit` 會按真實結果**重新評分**（覆蓋 skip）——最終報告呈現“超時後補齊”的
> 正確得分；只有從頭到尾沒交的題才保留 skip。預算用 `>30s` 的寬鬆值通常不誤傷正常慢題。

## 用例

內置 18 例，覆蓋：計算/數論/精確分數推理、常識/文言史實/多語言書寫、結構化輸出、工具端態、多步文件鏈與 shell 管道、StrReplaceEditor 編輯流程、**DSC CRON 機制真實調用**。

| id | 類型 | 驗證點 |
| -- | ---- | ------ |
| `arith_power`   | answer/num      | 整數冪運算數值 |
| `sqrt_2`        | answer/num      | 開平方精度（容差） |
| `capital_france`| answer/anyof  | 常識問答 + 聯合國官方語言（任一種命中） |
| `ming_capital` | answer/anyof  | 文言史實問答（洪武開國 → 南京）官方語言命中 |
| `ming_capital_judgment` | answer/groups | 判斷力（識別明朝國都歧義，須同時指出明初南京 + 遷都後北京） |
| `odd_sum`       | answer/num     | 1..99 奇數和（2500） |
| `fib_10`        | answer/num     | 斐波那契第 10 項（55） |
| `prime_below_20`| answer/num     | 數論質數（19） |
| `json_health`   | answer/regex   | 結構化 JSON 輸出（計算 1..10 和入鍵） |
| `fraction_sum`  | answer/num     | 精確分數運算（lisp_eval：1/3+1/7=10/21 循環小數） |
| `file_reverse`  | file/ieq        | 文件工具落盤端態（單詞逆序） |
| `file_sum`      | file/num        | 計算並落盤（運行時集成） |
| `file_multi`    | file/groups     | 多行/追加寫入（hello + world） |
| `file_wc_lines` | file/num        | shell wc 統計行數並落盤 |
| `pipe_filter`   | file/num        | **shell 管道**（cat \| grep \| wc 一行工程並算落盤） |
| `editor_flow`   | file/regex      | **StrReplaceEditor 編輯流程**（create→view→str_replace→insert 全鏈路，端態須同時含替換後內容與插入行） |
| `cron_add_id`   | file/regex      | **DSC CRON 機制真實調用**（cron_add 拿到宿主分配的 `cron-<digits>` id 並落盤） |
| `cron_list_roundtrip` | file/regex | **DSC CRON 機制全鏈路**（cron_add → cron_list → cron_set_enabled(false) → cron_list 驗證 disabled → 落盤 id；權重 2） |

> 注：`json_health`、`file_multi`、`editor_flow` 與兩個 `cron_*` 是「格式合規」類——
> 任務本身即要求的輸出格式/內容，無可保密的預期值，故對防作弊掃描標記 `NoLeak` 豁免；
> 其餘用例的期望值一律不下發模型。CRON 用例的「期望」是宿主動態分配的 `cron-<digits>` id 格式，
> 模型無法憑空捏造——只有真實調用 `cron_add` 等工具拿到合法 id 才能通過端態判定。

產物按 `<benchRoot>/bench-out/<case_id>/reply.txt` 落盤；報告寫
`<benchRoot>/bench-out/report.json`。

## 呈現樣式示例（真實運行時輸出）

以本地模型 `Agentic-Turbo-Coder`（llama.cpp 服務）經 `bench` 程序（`-input` 且 stdin
重定向關單輪、`DSC_APPROVAL=never` 無人值守、未設交卷/每案例預算）跑一次完整
**16 用例**的真實結果為例：

- `bench_report` 落盤的 `bench-out/report.json`（機器可讀，原始輸出；含**總耗時**
  `duration_ms` 與各用例**單項耗時** `duration_ms`）：

  ```json
  {
    "bench_root": "…/dist/tmp-test",
    "started_at": "2026-09-08T00:09:58.7730777+08:00",
    "duration_ms": 240974,
    "cases": [
      {"id":"arith_power","title":"整數冪運算","status":"pass","weight":1,"earned":1,"duration_ms":4877,"artifact_content":""},
      {"id":"sqrt_2","title":"開平方精度","status":"pass","weight":1,"earned":1,"duration_ms":6954,"artifact_content":""},
      {"id":"capital_france","title":"常識問答（聯合國官方語言）","status":"pass","weight":1,"earned":1,"duration_ms":4018,"artifact_content":""},
      {"id":"ming_capital","title":"文言史實問答（明朝國都·洪武開國）","status":"pass","weight":1,"earned":1,"duration_ms":4099,"artifact_content":""},
      {"id":"ming_capital_judgment","title":"判斷力（明朝國都的歧義）","status":"pass","weight":1,"earned":1,"duration_ms":3205,"artifact_content":""},
      {"id":"file_reverse","title":"文件工具寫入（端態校驗）","status":"pass","weight":1,"earned":1,"duration_ms":17827,"artifact_content":"fox brown quick the"},
      {"id":"file_sum","title":"計算並落盤（運行時集成）","status":"pass","weight":1,"earned":1,"duration_ms":11855,"artifact_content":"5050"},
      {"id":"odd_sum","title":"數列求和（奇數）","status":"pass","weight":1,"earned":1,"duration_ms":24480,"artifact_content":""},
      {"id":"fib_10","title":"遞歸/遞推數列","status":"pass","weight":1,"earned":1,"duration_ms":4032,"artifact_content":""},
      {"id":"prime_below_20","title":"數論（質數）","status":"fail","weight":1,"earned":0,"duration_ms":3680,"artifact_content":"","feedback":"數值不在容差範圍內（偏差 2）"},
      {"id":"json_health","title":"結構化 JSON 輸出","status":"pass","weight":1,"earned":1,"duration_ms":2633,"artifact_content":""},
      {"id":"fraction_sum","title":"精確分數運算（lisp_eval）","status":"pass","weight":1,"earned":1,"duration_ms":11622,"artifact_content":""},
      {"id":"file_multi","title":"多次寫盤（追加）","status":"pass","weight":1,"earned":1,"duration_ms":7682,"artifact_content":"hello\nworld"},
      {"id":"file_wc_lines","title":"命令統計並落盤（wc）","status":"pass","weight":1,"earned":1,"duration_ms":13156,"artifact_content":"3"},
      {"id":"pipe_filter","title":"Shell 管道（端態驗證）","status":"pass","weight":1,"earned":1,"duration_ms":35649,"artifact_content":"3"},
      {"id":"editor_flow","title":"StrReplaceEditor 編輯流程（view→str_replace→insert）","status":"pass","weight":1,"earned":1,"duration_ms":75141,"artifact_content":"package main\nfunc main() {\n\tprintln(\"world\")\n// edited\n}"}
    ],
    "harness": "dsc-tool-agentic-bench",
    "passed": 15, "failed": 1, "total": 16,
    "score": "93.75", "ratio": "15:1"
  }
  ```

> 彙總口徑（單點制）：`score`（總得分）= 成功數/總案例×100 的**兩位小數數值**（如
> `80.00`）；`ratio`（成敗比）= **成功數:失敗數**（如 `12:3`）。無冗餘的 percent 字段
> ——得分本身就是那個數值。

- TUI 中 `bench_report` 以**表格視圖**渲染為對齊列（狀態 PASS/FAIL/SKIP 着綠/紅/灰，
  含**耗時**列；徽標顯示計數、得分與總耗時）：

  | Case                  | 任務                    | 狀態 | 得分 | 耗時     |
  | --------------------- | ----------------------- | ---- | ---- | -------- |
  | arith_power           | 整數冪運算              | PASS | 1/1  | 4.9s  |
  | sqrt_2                | 開平方精度              | PASS | 1/1  | 7.0s  |
  | capital_france        | 常識問答（聯合國官方語言） | PASS | 1/1  | 4.0s  |
  | ming_capital          | 文言史實問答（洪武開國）  | PASS | 1/1  | 4.1s  |
  | ming_capital_judgment | 判斷力（明朝國都的歧義）  | PASS | 1/1  | 3.2s  |
  | file_reverse          | 文件工具寫入（端態校驗）  | PASS | 1/1  | 17.8s |
  | file_sum              | 計算並落盤（運行時集成）  | PASS | 1/1  | 11.9s |
  | odd_sum               | 數列求和（奇數）        | PASS | 1/1  | 24.5s |
  | fib_10                | 遞歸/遞推數列           | PASS | 1/1  | 4.0s  |
  | prime_below_20        | 數論（質數）            | FAIL | 0/1  | 3.7s  |
  | json_health           | 結構化 JSON 輸出        | PASS | 1/1  | 2.6s  |
  | fraction_sum          | 精確分數運算（lisp_eval）| PASS | 1/1  | 11.6s |
  | file_multi            | 多次寫盤（追加）        | PASS | 1/1  | 7.7s  |
  | file_wc_lines         | 命令統計並落盤（wc）    | PASS | 1/1  | 13.2s |
  | pipe_filter           | Shell 管道（cat\|grep\|wc）| PASS | 1/1  | 35.6s |
  | editor_flow           | StrReplaceEditor 編輯流程| PASS | 1/1  | 75.1s |
  | **— 彙總 —**          | 通過 15/16 · 成敗 15:1 | — | 93.75 | 4分1秒 |

  徽標：`15/16 PASS · 得分 93.75 · 4分1秒`（全過標綠，部分失敗標紅）

> 計時口徑：**總耗時**從 `bench_start` 起算到本次 `bench_report`；**單項耗時**從該用例
> 首次 `bench_next` 下發到 `bench_submit`。均隨報告寫入 `report.json` 並展示於 `bench_report`
> 表格與徽標。

本示例同時演示了一個真實失敗形態：`prime_below_20` 是模型把質數答成 17、而期望為
19（數論判定容差 2 外，判 FAIL）——純模型答題波動，與工具鏈無關。其餘 file 類用例（含
`pipe_filter` 的 shell 管道一行工程 cat \| grep \| wc）均一次通過，證明運行時集成真實可靠；
`editor_flow` 一次通過，證明 StrReplaceEditor 編輯全鏈路（create→view→str_replace→insert）
及沙箱對工作區內相對路徑寫放的端態判定都在運行時真實可用。

## 運行

1. 構建插件（`./build.sh` 已登記，或手動）：

   ```bash
   cd plugins/tool-agentic-bench && go build -o tool-agentic-bench.exe .
   ```

2. 讓模型在會話中經 `load_dsc_plugin tool-agentic-bench`（或把 `tool-agentic-bench` 加入
   config.yaml）加載。

bench 產物根取自宿主注入的 `DSC_WORKSPACE_ROOT`（對齊沙箱：相對路徑寫天然落在其內，
無需 full-access）。建議把工作目錄指向一個乾淨的臨時目錄，避免污染真實工作區。

有兩個運行方式，任選其一。

### 方式一：純命令自動開啓（無人值守 / CI）

不經過 TUI 交互，一切由命令行一次性完成，適合腳本與 CI。

> ⚠️ **避免「只跑 1 輪就退出」的關鍵**
> 宿主對 `-headless`（或 `-input` 且 **stdin 未重定向**）會注入 `DSC_SINGLE_TURN=1`，
> 把 agent 的 ReAct 循環強制為 **1 輪**——模型常剛調完 `bench_start` 就輸出收尾而退出，
> 後面的用例全部沒跑（這正是曾遇到的現象）。
> 完整跑完 agentic 循環的辦法：讓 **stdin 處於重定向狀態**（管道/文件），此時宿主
> **不會**注入 `DSC_SINGLE_TURN`，循環不再設上限，模型可一路 `bench_next` 逐個完成。
> 下面的示例正是用 `"" |` 把空行管道進 stdin 來關閉單輪模式（空行會被 `runStdinLoop`
> 跳過，不會產生多餘回合）。

```bash
# PowerShell 示例：臨時工作區 + 審批自動拒絕 + 完整 agentic 循環
$env:DSC_WORKSPACE_ROOT = "C:\Users\me\AppData\Local\Temp\dsc-bench-run-artifacts"
$env:DSC_APPROVAL      = "never"
"" | .\dsc.exe -input "加載 tool-agentic-bench 插件，逐項完成所有 bench 測試並彙總報告"
```

> 若改用 `-headless` 或直接 `-input "…"`（不接管道），只會跑 1 輪即退出，無法完成整輪評測。

- 產物隨用例而定：各用例 `reply.txt` 與彙總 `report.json` 都在該臨時工作區內。
- 大模型本地跑得慢不是問題：bench 本身不設固定短超時，判分為純端態判定，不受速度
  影響；真正的時間預算由宿主對工具調用的「活躍續命」超時機制兜底，只有長時間無產出
  才判超時。
- 全自動無人值守須把審批策略設為 `never`，避免中途卡在人工審批。`-headless`/`-input`
  不經 TUI、無法敲斜杆命令，因此要用**環境變量** `DSC_APPROVAL=never`（或 config.yaml
  裏 preset 綁定）在啓動前設好——`/approval` 斜杆命令只對交互式 TUI 會話生效。

### 方式二：TUI 內設審批後，用自然語言讓模型跑

用户在交互式 TUI 會話裏先手動把審批設為自動拒絕，再下達自然語言指令讓模型自行加載
插件並完成測試。

1. 啓動 DSC 進入 TUI：

   ```text
   /approval never        # 審批自動拒絕（on/off 為 ask/never 別名），避免中途問人
   /load                   # 若需確認工具目錄刷新，或直接用下面自然語言交給模型加載
   ```

   （也可先把 `tool-agentic-bench` 加進 config.yaml，跳過運行時加載。）

2. 然後向模型下達自然語言指令，例如：

   ```text
   請加載 tool-agentic-bench 插件，按它內置的用例逐一跑完 agentic-bench 測試，
   完成後用 bench_report 輸出計分彙總。
   ```

   模型會自行調用 `bench_start → bench_next →（用真實工具完成任務）→
   bench_submit → … → bench_report`，TUI 裏每個步驟都有結構化視圖呈現。

3. 查看報告：每次 `bench_report` 返回計分表格；同時彙總 JSON 已落盤到
   `<benchRoot>/bench-out/report.json`。

兩種方式建議都把 `DSC_WORKSPACE_ROOT` 指向臨時目錄，保證產物不混入真實工作區。

## 擴展

新增用例只需在 `cases.go` 的 `benchCases` 增刪條目，填寫 `Kind` / `Matcher` /
`Expected` / `Tol`（期望值與規則不進入任何下發模型的文本，防作弊由
`TestNoAnswerLeakInTask` 守衞）。