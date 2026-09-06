# agentic-bench（DSC 模型能力自动评分测试插件）

`tool-agentic-bench` 是 DSC 上的一组**模型能力评分测试台**（agentic-bench）工具：它把一组内置的运行时集成
测试用例暴露给模型，模型经真实工具逐个完成，插件进程自动判定每项、汇总计分并输出
报告。评测对象是**模型自身**；同时因 file 类用例需模型驱动文件工具落盘，若全用例
通过也相当于对 DSC 的工具运行时做了集成验证。

## 平台说明

- 纯 Go + `dsc-sdk`，零 CGO，目标平台集七端全部交叉编译通过
  （`linux/amd64`、`linux/arm64`、`linux/loong64`、`darwin/amd64`、
  `darwin/arm64`、`windows/amd64`、`freebsd/amd64`）。

## 工作原理与防作弊

- 每个用例只把**任务陈述**（Task）暴露给模型；期望答案与判定规则只存在于插件进程
  内部，绝不写入工具描述 / `Context` / `ListContext`。`bench_next` 只下发任务文字，
  `bench_submit` 失败时也仅回泛化原因、不透露期望值。插件还带一条
  `bench_start` 的 Context 引导模型真实完成任务、不得向 bench 索要答案，否则判 FAIL。
- file 类用例由插件**直接读取产物文件**判定（端态校验），无需模型回传内容，杜绝"猜答
  案"与"工具链路没真正走通却蒙混过关"。
- 幂等 + 自动推进：同用例重复 `bench_submit` 返回既有结果、不重复计分；**交卷后插件立即
  在响应里揭晓下一用例（`next`）**，已评分用例无法重试——这既保证每项仅计一次，也让
  agent 循环强制前进，杜绝「弱模型反复重交同一题」造成的死循环。

## 工具一覽

| 工具            | 用途                                                              |
| ---------------- | --------------------------------------------------------------- |
| `bench_start`    | 初始化一次评分：重置结果、创建产物目录，返回用例总数与各用例 id/标题；并启动**交卷预算**看门狗 |
| `bench_next`     | 取下一个未评分用例（含任务陈述）；全部完成返回 done:true        |
| `bench_submit`   | 提交某用例完成结果并评分（answer 带 answer，file 由插件读盘判定）；交卷后**自动推进到下一用例**（响应 next 直接给出下一题），已评分用例无法重试 |
| `bench_report`   | 自动计分汇总（总得分 + 成败比 + 逐用例状态表 + **总耗时**）并把 JSON 落盘（含总耗时与各用例单项耗时） |

> **交卷/超时预算**（都“设置才启用”）：
> `DSC_BENCH_TIMEOUT`=全局预算（秒），**不设 = 无限制**，到期把剩余未交卷用例标 skip 并
> 即时落盘报告；`DSC_BENCH_CASE_TIMEOUT`=每案例预算（秒），**默认关闭**，某题下发后超时
> 即标 skip 并自动推进到下一题。两者可并用：单题超时跳过、整场超时兜底。
>
> **预算 = 兜底，不冤枉及时交卷**：超时触发的 skip 只是“推进/保底”。若模型之后仍对该题
> 交卷，`bench_submit` 会按真实结果**重新评分**（覆盖 skip）——最终报告呈现“超时后补齐”的
> 正确得分；只有从头到尾没交的题才保留 skip。预算用 `>30s` 的宽松值通常不误伤正常慢题。

## 用例

内置 14 例，覆盖：计算/数论/精确分数推理、常识/文言史实/多语言书写、结构化输出、工具端态与多步文件链。

| id | 类型 | 验证点 |
| -- | ---- | ------ |
| `arith_power`   | answer/num      | 整数幂运算数值 |
| `sqrt_2`        | answer/num      | 开平方精度（容差） |
| `capital_france`| answer/anyof  | 常识问答 + 联合国官方语言（任一种命中） |
| `ming_capital` | answer/anyof  | 文言史实问答（洪武开国 → 南京）官方语言命中 |
| `ming_capital_judgment` | answer/groups | 判断力（识别明朝国都歧义，须同时指出明初南京 + 迁都后北京） |
| `odd_sum`       | answer/num     | 1..99 奇数和（2500） |
| `fib_10`        | answer/num     | 斐波那契第 10 项（55） |
| `prime_below_20`| answer/num     | 数论质数（19） |
| `json_health`   | answer/regex   | 结构化 JSON 输出（计算 1..10 和入键） |
| `fraction_sum`  | answer/num     | 精确分数运算（lisp_eval：1/3+1/7=10/21 循环小数） |
| `file_reverse`  | file/ieq        | 文件工具落盘端态（单词逆序） |
| `file_sum`      | file/num        | 计算并落盘（运行时集成） |
| `file_multi`    | file/groups     | 多行/追加写入（hello + world） |
| `file_wc_lines` | file/num        | shell wc 统计行数并落盘 |

> 注：`json_health` 与 `file_multi` 是「格式合规」类——任务本身即要求的输出格式/内容，
> 无可保密的预期值，故对防作弊扫描标记 `NoLeak` 豁免；其余用例的期望值一律不下发模型。

产物按 `<benchRoot>/bench-out/<case_id>/reply.txt` 落盘；报告写
`<benchRoot>/bench-out/report.json`。

## 呈现样式示例（真实运行时输出）

以本地模型 `Agentic-Turbo-Coder`（llama.cpp Q8_0）经 `-input`（stdin 重定向关单轮、
`DSC_APPROVAL=never`、交卷预算 `DSC_BENCH_TIMEOUT=480`）无人值守跑一次完整 **14 用例**
的真实结果为例：

- `bench_report` 落盘的 `bench-out/report.json`（机器可读，原始输出；含**总耗时**
  `duration_ms` 与各用例**单项耗时** `duration_ms`）：

  ```json
  {
    "bench_root": "…/dsc-bench-artifacts6",
    "started_at": "2026-09-06T16:09:58.6415126+08:00",
    "duration_ms": 132447,
    "cases": [
      {"id":"arith_power","title":"整数幂运算","status":"pass","weight":1,"earned":1,"duration_ms":7517,"artifact_content":""},
      {"id":"sqrt_2","title":"开平方精度","status":"pass","weight":1,"earned":1,"duration_ms":5444,"artifact_content":""},
      {"id":"capital_france","title":"常识问答（联合国官方语言）","status":"pass","weight":1,"earned":1,"duration_ms":3211,"artifact_content":""},
      {"id":"ming_capital","title":"文言史实问答（明朝国都·洪武开国）","status":"pass","weight":1,"earned":1,"duration_ms":2459,"artifact_content":""},
      {"id":"ming_capital_judgment","title":"判断力（明朝国都的歧义）","status":"pass","weight":1,"earned":1,"duration_ms":2897,"artifact_content":""},
      {"id":"file_reverse","title":"文件工具写入（端态校验）","status":"pass","weight":1,"earned":1,"duration_ms":14153,"artifact_content":"fox brown quick the"},
      {"id":"file_sum","title":"计算并落盘（运行时集成）","status":"pass","weight":1,"earned":1,"duration_ms":16371,"artifact_content":"5050"},
      {"id":"odd_sum","title":"数列求和（奇数）","status":"pass","weight":1,"earned":1,"duration_ms":6269,"artifact_content":""},
      {"id":"fib_10","title":"递归/递推数列","status":"pass","weight":1,"earned":1,"duration_ms":2178,"artifact_content":""},
      {"id":"prime_below_20","title":"数论（质数）","status":"pass","weight":1,"earned":1,"duration_ms":1593,"artifact_content":""},
      {"id":"json_health","title":"结构化 JSON 输出","status":"pass","weight":1,"earned":1,"duration_ms":5519,"artifact_content":""},
      {"id":"fraction_sum","title":"精确分数运算（lisp_eval）","status":"fail","weight":1,"earned":0,"duration_ms":4914,"artifact_content":"","feedback":"数值不在容差范围内（偏差 0.0476095）"},
      {"id":"file_multi","title":"多次写盘（追加）","status":"pass","weight":1,"earned":1,"duration_ms":14702,"artifact_content":"hello\nworld"},
      {"id":"file_wc_lines","title":"命令统计并落盘（wc）","status":"fail","weight":1,"earned":0,"duration_ms":16293,"artifact_content":"3","feedback":"数值不在容差范围内（偏差 1）"}
    ],
    "harness": "dsc-tool-agentic-bench",
    "passed": 12, "failed": 2, "total": 14,
    "score": "85.71", "ratio": "12:2"
  }
  ```

> 汇总口径（单点制）：`score`（总得分）= 成功数/总案例×100 的**两位小数数值**（如
> `85.71`）；`ratio`（成败比）= **成功数:失败数**（如 `12:2`）。无冗余的 percent 字段
> ——得分本身就是那个数值。

- TUI 中 `bench_report` 以**表格视图**渲染为对齐列（状态 PASS/FAIL/SKIP 着绿/红/灰，
  含**耗时**列；徽标显示计数、得分与总耗时）：

  | Case                  | 任务                    | 状态 | 得分 | 耗时     |
  | --------------------- | ----------------------- | ---- | ---- | -------- |
  | arith_power           | 整数幂运算              | PASS | 1/1  | 7.5s  |
  | sqrt_2                | 开平方精度              | PASS | 1/1  | 5.4s  |
  | capital_france        | 常识问答（联合国官方语言） | PASS | 1/1  | 3.2s  |
  | ming_capital          | 文言史实问答（洪武开国）  | PASS | 1/1  | 2.5s  |
  | ming_capital_judgment | 判断力（明朝国都的歧义）  | PASS | 1/1  | 2.9s  |
  | file_reverse          | 文件工具写入（端态校验）  | PASS | 1/1  | 14.2s |
  | file_sum              | 计算并落盘（运行时集成）  | PASS | 1/1  | 16.4s |
  | odd_sum               | 数列求和（奇数）        | PASS | 1/1  | 6.3s  |
  | fib_10                | 递归/递推数列           | PASS | 1/1  | 2.2s  |
  | prime_below_20        | 数论（质数）            | PASS | 1/1  | 1.6s  |
  | json_health           | 结构化 JSON 输出        | PASS | 1/1  | 5.5s  |
  | fraction_sum          | 精确分数运算（lisp_eval）| FAIL | 0/1  | 4.9s  |
  | file_multi            | 多次写盘（追加）        | PASS | 1/1  | 14.7s |
  | file_wc_lines         | 命令统计并落盘（wc）    | FAIL | 0/1  | 16.3s |
  | **— 汇总 —**          | 通过 12/14 · 成败 12:2 | — | 85.71 | 2分12秒 |

  徽标：`12/14 PASS · 得分 85.71 · 2分12秒`（全过标绿，部分失败标红）

> 计时口径：**总耗时**从 `bench_start` 起算到本次 `bench_report`；**单项耗时**从该用例
> 首次 `bench_next` 下发到 `bench_submit`。均随报告写入 `report.json` 并展示于 `bench_report`
> 表格与徽标。

本示例同时演示了两种真实失败形态：`fraction_sum` 是模型把 1/3+1/7（=10/21≈0.4762）错算并
提交 0.5238（差 1/21）；`file_wc_lines` 是 `wc -l` 按 POSIX 数换行符、末行无换行故计 2（差
1）——说明评测既反映模型能力，也检验对工具语义（新行计数）的理解。

## 运行

1. 构建插件（`./build.sh` 已登记，或手动）：

   ```bash
   cd plugins/tool-agentic-bench && go build -o tool-agentic-bench.exe .
   ```

2. 让模型在会话中经 `load_dsc_plugin tool-agentic-bench`（或把 `tool-agentic-bench` 加入
   config.yaml）加载。

bench 产物根取自宿主注入的 `DSC_WORKSPACE_ROOT`（对齐沙箱：相对路径写天然落在其内，
无需 full-access）。建议把工作目录指向一个干净的临时目录，避免污染真实工作区。

有两个运行方式，任选其一。

### 方式一：纯命令自动开启（无人值守 / CI）

不经过 TUI 交互，一切由命令行一次性完成，适合脚本与 CI。

> ⚠️ **避免「只跑 1 轮就退出」的关键**
> 宿主对 `-headless`（或 `-input` 且 **stdin 未重定向**）会注入 `DSC_SINGLE_TURN=1`，
> 把 agent 的 ReAct 循环强制为 **1 轮**——模型常刚调完 `bench_start` 就输出收尾而退出，
> 后面的用例全部没跑（这正是曾遇到的现象）。
> 完整跑完 agentic 循环的办法：让 **stdin 处于重定向状态**（管道/文件），此时宿主
> **不会**注入 `DSC_SINGLE_TURN`，循环不再设上限，模型可一路 `bench_next` 逐个完成。
> 下面的示例正是用 `"" |` 把空行管道进 stdin 来关闭单轮模式（空行会被 `runStdinLoop`
> 跳过，不会产生多余回合）。

```bash
# PowerShell 示例：临时工作区 + 审批自动拒绝 + 完整 agentic 循环
$env:DSC_WORKSPACE_ROOT = "C:\Users\me\AppData\Local\Temp\dsc-bench-run-artifacts"
$env:DSC_APPROVAL      = "never"
"" | .\dsc.exe -input "加载 tool-agentic-bench 插件，逐项完成所有 bench 测试并汇总报告"
```

> 若改用 `-headless` 或直接 `-input "…"`（不接管道），只会跑 1 轮即退出，无法完成整轮评测。

- 产物随用例而定：各用例 `reply.txt` 与汇总 `report.json` 都在该临时工作区内。
- 大模型本地跑得慢不是问题：bench 本身不设固定短超时，判分为纯端态判定，不受速度
  影响；真正的时间预算由宿主对工具调用的「活跃续命」超时机制兜底，只有长时间无产出
  才判超时。
- 全自动无人值守须把审批策略设为 `never`，避免中途卡在人工审批。`-headless`/`-input`
  不经 TUI、无法敲斜杆命令，因此要用**环境变量** `DSC_APPROVAL=never`（或 config.yaml
  里 preset 绑定）在启动前设好——`/approval` 斜杆命令只对交互式 TUI 会话生效。

### 方式二：TUI 内设审批后，用自然语言让模型跑

用户在交互式 TUI 会话里先手动把审批设为自动拒绝，再下达自然语言指令让模型自行加载
插件并完成测试。

1. 启动 DSC 进入 TUI：

   ```text
   /approval never        # 审批自动拒绝（on/off 为 ask/never 别名），避免中途问人
   /load                   # 若需确认工具目录刷新，或直接用下面自然语言交给模型加载
   ```

   （也可先把 `tool-agentic-bench` 加进 config.yaml，跳过运行时加载。）

2. 然后向模型下达自然语言指令，例如：

   ```text
   请加载 tool-agentic-bench 插件，按它内置的用例逐一跑完 agentic-bench 测试，
   完成后用 bench_report 输出计分汇总。
   ```

   模型会自行调用 `bench_start → bench_next →（用真实工具完成任务）→
   bench_submit → … → bench_report`，TUI 里每个步骤都有结构化视图呈现。

3. 查看报告：每次 `bench_report` 返回计分表格；同时汇总 JSON 已落盘到
   `<benchRoot>/bench-out/report.json`。

两种方式建议都把 `DSC_WORKSPACE_ROOT` 指向临时目录，保证产物不混入真实工作区。

## 扩展

新增用例只需在 `cases.go` 的 `benchCases` 增删条目，填写 `Kind` / `Matcher` /
`Expected` / `Tol`（期望值与规则不进入任何下发模型的文本，防作弊由
`TestNoAnswerLeakInTask` 守卫）。