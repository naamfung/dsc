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

export async function apiChat(content: string): Promise<ChatResult> {
	const r = await fetch('/api/chat', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ content })
	});
	return (await r.json()) as ChatResult;
}