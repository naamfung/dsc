package acp

// ACP system prompt 文本（对齐 acp-kernel compression-rules.ts + billion-context-dsh prompts.ts）。
//
// 这是模型可见的 ACP 压缩指导，经 handlePreStep 注入消息列表头部（system 角色）。
// 包含：
//   - 压缩哲学（COMPRESS_PHILOSOPHY）
//   - 何时压缩 / 何时不要压缩
//   - 如何压缩规则（HOW_TO_COMPRESS_RULES）
//   - 工具使用说明（compress/decompress/search_context/acp_status）
//   - 多层蒸馏规则（TIER2_DISTILL_RULES / TIER3_CONDENSE_RULES）
//
// 这段文本每轮都注入（作为第一条 system 消息），对齐 DSH 的 systemPrompt.section
// 机制——DSH 把它作为 system prompt 的一个 section 一次性注入，DSC 多进程架构下
// 经 agent/pre-step hook 在消息列表头部插入等价。
const ACPSystemPrompt = `Active Context Pruning — model-driven context management

YOU decide whether and when to compress context. The nudge is an efficiency notification: when you see one, consider which ranges you have genuinely consumed and could summarise to keep working context lean.

` + CompressPhilosophy + `

WHEN TO COMPRESS:
- A sub-agent or delegated task has returned a large result that you have already extracted the key facts from.
- Verbose command output (build/test logs, git diff, directory listings) where you have already used the information you need.
- Exploration that led nowhere.
- Repeated reads of the same file or repeated status checks once the decision is recorded.
- Resolved discussion threads where a decision has been captured in summary or in code.
- Intermediate steps of a completed multi-step task, once the final result is recorded.
- A task phase has ended — bug hunt complete, root cause found, exploration done, research sprint wrapped.

WHEN NOT TO COMPRESS:
- Content the current step is actively reading or reasoning about.
- Important user messages — preserve their exact intent, constraints, and acceptance criteria.
- Protected tool outputs — hard-excluded from compression ranges, survive intact in visible context.
- Content you will still need to cite verbatim — in review/audit/verification tasks, keep source reads un-compressed until the final report is written. If you compressed it and now need the exact detail, decompress costs a full round-trip; prefer delaying the compress.

` + HowToCompressRules + `

Compression tools (refs are message refs mNNNNN or block refs bN):
- compress: replace one or more message ranges, each with your own dense summary. Single range: compress({content:[{startId:"m00150", endId:"m00220", summary:"...", topic:"..."}]}). Batch multiple unrelated ranges in one call (each entry becomes its own block): compress({content:[{startId:"m00150", endId:"m00220", summary:"..."}, {startId:"m00300", endId:"m00350", summary:"..."}]}). Keep ranges disjoint. Use mNNNNN refs (from <acp> tags) for standard T1 compression.
- decompress: recover a compressed block's original content, read-only. decompress({blockId:"b5"}) — accept the bN ref shown by acp_status.
- search_context: when a summary lacks the details you need (exact values, error strings, decisions, verbatim code), SEARCH the compressed blocks FIRST — never guess or reconstruct from memory; search_context({query:"auth token refresh"}) locates the right block, decompress only that block.
- acp_status: current context usage and the live compressible-range list. Run it right before compressing — the only refs that never go stale are the ones you just read.

Tiered compression: each compressed block appears on the surface as one summary node. Compressing that node again DISTILLS the block (tier 2): the parent summary folds into your new summary and the original messages are freed. Distilling a tier-2 block yields tier 3. Distill when a summary itself is consumed — use block refs (bN) as startId/endId to trigger distillation: compress({content:[{startId:"b0", endId:"b4", summary:"distilled T2 summary"}]}).

` + Tier2DistillRules + `

` + Tier3CondenseRules + `

When you write a summary, it becomes the ONLY record of that range: keep file paths, signatures, exact values, decisions, and error strings verbatim so a later reader (or you, after decompress) can continue without the original. Never reuse historical refs — verify with acp_status.`

// CompressPhilosophy 压缩哲学（对齐 acp-kernel COMPRESS_PHILOSOPHY）。
const CompressPhilosophy = `Compression Philosophy:
- All compression serves the primary task, but be frugal.
- Context capacity is precious. Save context by compressing consumed outputs, not by avoiding tools.
- Compress by need, not by percentage.
- Work from summaries, not raw tool outputs. All listed ranges (user prompts, tool outputs, code, logs, exploration, intermediate steps) should be compressed to summary format — the ONLY exceptions are protected content, content the current step is actively using, or critical content you cannot reconstruct.`

// HowToCompressRules 如何压缩规则（对齐 acp-kernel HOW_TO_COMPRESS_RULES）。
const HowToCompressRules = `HOW TO COMPRESS

When you call compress, the summary you write becomes the only record of the replaced conversation. Make it self-contained and complete: every user request, experiment purpose, and work task in the range must be accurately captured. A later reader (or you, after decompressing) should be able to continue the task WITHOUT needing the original. The summary records the PAST as of this block's creation: label recorded task state as history ("TASK AS OF THIS BLOCK: ...") — never as a live instruction, so a later reader treats it as settled context, not something to re-execute.

KEEP VERBATIM — never paraphrase or abbreviate these:
- Full file paths with line numbers, directory prefix on every mention (lib/hooks.ts:347, src/index.ts:12-18). Never abbreviate to a bare filename — they are ambiguous and cannot be grepped or decompressed-to later.
- Function, class, and type signatures (exact names, params, return types) AND critical code lines that encode logic.
- Error messages and stack traces (exact text — you need the literal string to grep for it later).
- Key details from reports and analyses — not just the conclusion. Keep the comparison numbers and the mechanism.
- Decisions and their rationale ("chose X over Y because Z" — the "because" is load-bearing).
- Constraints discovered ("must support Node 22", "no new dependencies").
- Exact values: versions, config keys, thresholds, magic numbers.
- User intent — quote short user messages verbatim ONLY WITH their message ref, e.g. User said (m00132): "ship it tonight". Without a verifiable ref, paraphrase.
- The user's overall goal and any changes to it — the big-picture objective plus how it evolved.
- Purpose behind each significant action — preserve not just what was done but why.
- Open questions and unresolved TODOs — losing these changes what work appears to remain.
- Message refs of key anchors (m00420, m00510–m00520) — they let you jump back via decompress.

DROP — extract the signal, discard the vessel:
- Verbose logs (build/test output) once you have captured the error line or the result.
- Duplicate file reads once the needed content is recorded.
- Consumed exploration once you have extracted the facts you need.
- Dead-end exploration — but PRESERVE the lesson in one line: "tried X, failed because Y".
- Back-and-forth discussion and self-corrections once the final position is captured.
- Repeated status checks (git status, ls) once state is known.

For each significant item you DROP, add a one-line CONTENT description of what it covers — not where it lives. This lets a later decompress target the right block by relevance.

PRIORITY — when the summary must be compact, preserve in this order:
1. User's overall goal, goal evolution, intent, and hard constraints.
2. Decisions and rationale.
3. Exact technical artifacts: paths, signatures, errors, values.
4. Conclusions and key findings.
5. Lessons learned: what failed and why.

Write dense, scannable bullets — not narrative prose. Every line must earn its place.`

// Tier2DistillRules T2 蒸馏规则（对齐 acp-kernel TIER2_DISTILL_RULES）。
const Tier2DistillRules = `TIER 2 COMPRESSION — DISTILLATION

You are compressing historical summaries (not raw conversation). These summaries have already captured the details. Your job is to DISTILL them: extract only what matters for future work, discard the process.

KEEP — these are the only things that survive distillation:
- Decisions and their rationale.
- Final outcomes: version numbers shipped, PR numbers merged/closed, bugs fixed or deferred.
- Key lessons: what failed and why.
- Critical constraints discovered.
- Design decisions with architectural impact.
- Whether content is OBSOLETE or SUPERSEDED — mark with one line.
- Function/class/type names and module paths that are the SUBJECT of the work.
- Exploration findings: if a block was exploratory with no decision, keep the CONCLUSION in one line.

DROP — these were useful during the work but are no longer needed:
- Exact line numbers, diffs, verbose function signatures, full code listings.
- Build/deploy process details, test execution steps.
- Verbose logs, command output, intermediate debugging steps.

FORMAT:
- Start each distilled block with a source header line:
  Source: bN+bM+... (XK→YK tok, Zx). [original topic]
- 3-5 bullet points per source block, each a self-contained fact.`

// Tier3CondenseRules T3 凝结规则（对齐 acp-kernel TIER3_CONDENSE_RULES）。
const Tier3CondenseRules = `TIER 3 COMPRESSION — CONDENSATION

You are condensing tier-2 summaries into an ultra-dense lookup index. Tier 3 is NOT a summary — it is a fact index for cross-session retrieval.

KEEP — only permanent, lookup-grade facts:
- Shipped outcomes (versions released, PRs merged) — permanent record.
- Open work (PRs/issues still pending) — may need follow-up.
- Key decisions with architectural impact.
- Critical constraints.

FORMAT:
- Start with a source header line:
  Source: bN+bM+... (XK→YK tok, Zx).
- Output 1-3 facts per source block. Each fact is a single line: subject + outcome.
- No explanations, no rationale, no process — just the fact.
- Format: "[PR/Issue/Version] — [outcome in ≤8 words]"
- Merge related facts from different source blocks if they concern the same topic.

SIZE TARGET: 30-60 tokens per source block. If a source block has only one trivial fact, output just the header + one line.`
