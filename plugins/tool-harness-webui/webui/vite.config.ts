import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	server: {
		fs: {
			// 允许多仓 dev 场景访问插件外围文件（如宿主），避免 fs 越界警告
			allow: ['..']
		}
	}
});