// 与 Go 端 /api/* 的通信契约

export interface HealthResult {
	ok: boolean;
	admin: string;
	agent: string;
	message: string;
}

export interface PluginItem {
	name?: string;
	type?: string;
	state?: string;
}

export interface DebuggerSnapshot {
	session_id?: string;
	turn_count?: number;
	plan_active?: boolean;
	last_prompt_tokens?: number;
	error?: string;
}

export interface ChatResult {
	reply?: string;
	bridge?: string;
	error?: string;
}

export async function apiGet<T>(path: string): Promise<T> {
	const r = await fetch(path);
	if (!r.ok) throw new Error(`HTTP ${r.status}`);
	return (await r.json()) as T;
}

// apiPlugins 调 /api/plugins。宿主 /plugins/list 返回 { status, plugins: [...] }，
// 这里解包成裸数组，供界面直接渲染。
export async function apiPlugins(): Promise<PluginItem[]> {
	const r = await fetch('/api/plugins');
	if (!r.ok) throw new Error(`HTTP ${r.status}`);
	const data = (await r.json()) as { plugins?: PluginItem[] };
	return data.plugins ?? [];
}

// apiDebugger 调 /api/debugger。宿主 /debugger/agent 返回 { status, agent, snapshot: {...} }，
// 这里解包成扁平快照结构，供界面直接渲染。
export async function apiDebugger(): Promise<DebuggerSnapshot> {
	const r = await fetch('/api/debugger');
	if (!r.ok) throw new Error(`HTTP ${r.status}`);
	const data = (await r.json()) as { snapshot?: DebuggerSnapshot; error?: string };
	if (data.error) throw new Error(data.error);
	return data.snapshot ?? {};
}

export async function apiChat(content: string): Promise<ChatResult> {
	const r = await fetch('/api/chat', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ content })
	});
	return (await r.json()) as ChatResult;
}