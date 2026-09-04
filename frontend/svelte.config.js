import adapter from '@sveltejs/adapter-static';

// dev 模式下 SvelteKit dev server (5173) 代理到 Go 后端 (8080)。
// 前端代码里 API_BASE = ''，所有 fetch 都是相对路径 /api/*，Vite proxy
// 在这里接管转发；生产构建由 Go embed 直接 serve，不走 Vite。
const devProxy = {
	'/api': { target: 'http://localhost:8080', changeOrigin: true },
	'/mcp': { target: 'http://localhost:8080', changeOrigin: true },
	'/uploads': { target: 'http://localhost:8080', changeOrigin: true },
	'/healthz': { target: 'http://localhost:8080', changeOrigin: true }
};

export default {
	vite: {
		server: {
			proxy: devProxy
		}
	},
	kit: {
		adapter: adapter({
			pages: 'dist',
			assets: 'dist',
			fallback: 'index.html',
			precompress: false,
			strict: true
		}),
		paths: {
			base: ''
		}
	}
};
