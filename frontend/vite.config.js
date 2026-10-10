import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// dev 模式下 SvelteKit dev server (5173) 代理到 Go 后端 (8080)。
// 前端代码里 API_BASE = ''，所有 fetch 都是相对路径 /api/*，Vite proxy
// 在这里接管转发；生产构建由 Go embed 直接 serve，不走 Vite。
const devProxy = {
	'/api': { target: 'http://localhost:8080', changeOrigin: true },
	'/mcp': { target: 'http://localhost:8080', changeOrigin: true },
	'/uploads': { target: 'http://localhost:8080', changeOrigin: true },
	'/healthz': { target: 'http://localhost:8080', changeOrigin: true }
};

// SvelteKit 3 起不再读取 svelte.config.js，配置一律通过 sveltekit() 插件传入。
// 只有 SvelteKit 自己的键（adapter / paths …）能放进插件；`vite` 不是 Kit 配置键，
// 插件会报 "invalid plugin option `vite`" 并丢弃它，所以 dev proxy 只能写在
// Vite 自己的顶层 server.proxy 里——写两处不会合并，反而会让 /mcp、/healthz
// 这两条只在插件里声明的路径在 dev 下 404。
export default defineConfig({
	plugins: [
		sveltekit({
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
		})
	],
	server: {
		proxy: devProxy
	}
});
