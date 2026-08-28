<script lang="ts">
	import { onMount } from 'svelte';
	import { apiChat, apiDebugger, apiPlugins } from '$lib/api';
	import type { ChatResult, DebuggerSnapshot, HealthResult, PluginItem } from '$lib/api';

	// ---- 状态 ----
	let health: HealthResult | null = null;
	let plugins: PluginItem[] = [];
	let snap: DebuggerSnapshot | null = null;
	let connected = false;

	interface ChatMsg {
		who: string;
		cls: string;
		body: string;
	}
	let messages: ChatMsg[] = [];
	let draft = '';
	let sending = false;

	function addSystem(text: string) {
		messages = [...messages, { who: '系统', cls: 'assistant', body: text }];
	}

	async function refresh() {
		try {
			health = await apiGet<HealthResult>('/api/health');
			connected = !!health.ok;
		} catch (e) {
			connected = false;
			addSystem(`/api/health 失败：${String(e)}`);
		}

		try {
			plugins = await apiPlugins();
		} catch (e) {
			plugins = [];
			addSystem(`/api/plugins 失败：${String(e)}`);
		}

		try {
			const s = await apiDebugger();
			snap = s && s.error ? null : s;
			if (s && s.error) addSystem(`快照不可用：${s.error}`);
		} catch (e) {
			snap = null;
		}
	}

	async function send() {
		const text = draft.trim();
		if (!text || sending) return;
		draft = '';
		sending = true;
		messages = [...messages, { who: '你', cls: 'user', body: text }];
		try {
			const res: ChatResult = await apiChat(text);
			if (res.error) {
				messages = [...messages, { who: 'agent', cls: 'assistant', body: `（对话桥接未接入宿主）${res.error}` }];
			} else if (res.bridge === 'pending') {
				addSystem(res.reply ?? '');
			} else {
				messages = [...messages, { who: 'agent', cls: 'assistant', body: res.reply ?? '（无回复）' }];
			}
		} catch (e) {
			addSystem(`发送失败：${String(e)}`);
		} finally {
			sending = false;
		}
	}

	onMount(() => {
		void refresh();
		const timer = setInterval(() => void refresh(), 4000);
		return () => clearInterval(timer);
	});
</script>

<svelte:head>
	<title>DSC Harness</title>
</svelte:head>

<div class="app">
	<header>
		<span class="brand">◆ DSC Harness</span>
		<span class="badge {connected ? 'ok' : 'err'}">{connected ? '● connected' : '○ offline'}</span>
		<span class="meta">agent: {health?.agent ?? 'agent-react-loop'}</span>
		<span class="spacer"></span>
		<span class="meta">admin: {health?.admin ?? '-'}</span>
	</header>

	<div class="body">
		<aside>
			<h3>插件</h3>
			{#if plugins.length === 0}
				<div class="item dim">（无 / 未加载）</div>
			{:else}
				{#each plugins as p}
					{@const name = p.name ?? '(unnamed)'}
					{@const st = (p.state ?? '?').toLowerCase()}
					{@const live = st === 'ready' || st === 'running'}
					<div class="item">
						<div class="name">{name}</div>
						<div class="state {live ? 'live' : 'stop'}">{st} · {p.type ?? ''}</div>
					</div>
				{/each}
			{/if}
		</aside>

		<main>
			<div class="chat" style="height: calc(100vh - 46px - 128px);">
				{#each messages as m}
					<div class="msg {m.cls}">
						<div class="who">{m.who}</div>
						<div class="b">{m.body}</div>
					</div>
				{/each}
				{#if snap}
					<div class="msg tool">
						<div class="who">snapshot</div>
						<div class="b">会话 {snap.session_id} ｜ turn {snap.turn_count} ｜ plan {snap.plan_active} ｜ tokens {snap.last_prompt_tokens}</div>
					</div>
				{/if}
			</div>

			<div class="composer">
				<textarea
					placeholder="发送给 DSC agent…（Enter 发送，Shift+Enter 换行）"
					bind:value={draft}
					onkeydown={(e) => {
						if (e.key === 'Enter' && !e.shiftKey) {
							e.preventDefault();
							void send();
						}
					}}
				></textarea>
				<div class="row">
					<button onclick={() => void send()} disabled={sending}>{sending ? '发送中…' : '发送'}</button>
					<span class="meta">{connected ? '' : '宿主 admin 未连接'}</span>
				</div>
			</div>
		</main>
	</div>
</div>

<style>
	.app {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
	}
	header {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		height: 44px;
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 0 16px;
		background: var(--panel);
		border-bottom: 1px solid var(--border);
		z-index: 10;
	}
	.brand {
		font-weight: 600;
		color: var(--accent);
	}
	.meta {
		font-size: 12px;
		color: var(--text-dim);
	}
	.spacer {
		flex: 1;
	}
	.badge {
		font-size: 12px;
		padding: 2px 8px;
		border-radius: 6px;
		background: var(--panel-2);
	}
	.badge.ok {
		color: var(--ok);
	}
	.badge.err {
		color: var(--err);
	}
	.body {
		display: flex;
		margin-top: 44px;
		flex: 1;
		min-height: 0;
	}
	aside {
		width: 220px;
		flex-shrink: 0;
		background: var(--panel);
		border-right: 1px solid var(--border);
		padding: 12px;
		overflow-y: auto;
	}
	aside h3 {
		font-size: 11px;
		text-transform: uppercase;
		color: var(--text-dim);
		margin: 12px 0 8px;
	}
	.item {
		font-size: 13px;
		padding: 6px 8px;
		border-radius: 6px;
		margin-bottom: 2px;
	}
	.item:hover {
		background: var(--panel-2);
	}
	.item .name {
		font-family: var(--mono, monospace);
	}
	.item .state {
		font-size: 11px;
		color: var(--text-dim);
	}
	.item .state.live {
		color: var(--ok);
	}
	.item .state.stop {
		color: var(--err);
	}
	.item.dim {
		color: var(--text-dim);
		margin-top: 4px;
	}
	main {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.chat {
		flex: 1;
		padding: 24px 32px;
		overflow-y: auto;
		background: var(--chat-bg);
	}
	.composer {
		padding: 12px 32px 20px;
		border-top: 1px solid var(--border);
		background: var(--panel);
	}
	textarea {
		width: 100%;
		resize: none;
		min-height: 64px;
		border-radius: 10px;
		border: 1px solid var(--border);
		background: var(--panel-2);
		color: var(--text);
		padding: 12px;
		font-size: 14px;
		outline: none;
		font-family: inherit;
	}
	textarea:focus {
		border-color: var(--accent);
	}
	.row {
		display: flex;
		align-items: center;
		gap: 8px;
		margin-top: 8px;
	}
	button {
		border: none;
		border-radius: 8px;
		padding: 8px 18px;
		font-size: 14px;
		background: var(--accent);
		color: #fff;
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.msg {
		margin-bottom: 22px;
		max-width: 860px;
	}
	.msg .who {
		font-size: 12px;
		font-weight: 600;
		margin-bottom: 4px;
		color: var(--text-dim);
	}
	.msg.user .who {
		color: var(--accent);
	}
	.msg .b {
		white-space: pre-wrap;
		font-size: 14px;
		line-height: 1.6;
	}
	.msg.tool .b {
		font-family: var(--mono, monospace);
		font-size: 12px;
		color: var(--text-dim);
	}
	.msg.tool .who {
		color: var(--warn);
	}
</style>