import js from '@eslint/js';
import prettier from 'eslint-config-prettier';
import svelte from 'eslint-plugin-svelte';
import { defineConfig } from 'eslint/config';
import globals from 'globals';

export default defineConfig(
  // 构建产物、依赖与静态资产不参与检查。dist/ 是 adapter-static 的真实输出目录，
  // 漏掉它会把压缩产物当源码扫出上千条假错误。
  // static/sw.js 是 scripts/build-sw.mjs 由模板生成的 PWA service worker，
  // 运行在 ServiceWorkerGlobalScope（clients 等全局不在 browser/node globals 内），
  // 属构建产物而非源码，不参与 lint。
  // scripts/ 同为构建期脚本（build-sw 生成器 + SW 模板），退出 eslint；
  // prettier 仍对它们格式化（见 .prettierignore 未排除 scripts/）。
  {
    ignores: ['dist/', 'node_modules/', '.svelte-kit/', 'static/', 'scripts/']
  },
  js.configs.recommended,
  svelte.configs.recommended,
  prettier,
  svelte.configs.prettier,
  {
    languageOptions: { globals: { ...globals.browser, ...globals.node } },
    rules: {
      // 纯 JS 项目（无 TS 源码），no-undef 靠 globals 已覆盖；
      // Svelte 模板里的 $state/$derived 等 rune 由插件识别，不受影响。
      // 存量放宽：原生 Map/Set/Date 在非响应式场景（纯计算、局部缓存、
      // 不参与模板渲染的临时容器）是合法写法，一次性改为 SvelteMap/SvelteSet/
      // SvelteDate 属重写级重构。降为 warn 保留可见性，新代码用 rune 版。
      'svelte/prefer-svelte-reactivity': 'warn',
      // `_` 是「有意丢弃」的约定写法（each 的占位变量、忽略的回调参数）
      'no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrors: 'none' }
      ]
    }
  },
  {
    files: ['**/*.svelte'],
    languageOptions: {
      parserOptions: {
        extraFileExtensions: ['.svelte']
      }
    }
  }
);
