# SvelteKit 3 迁移（bd04a90）横向排查报告

- 排查日期：2026-10-11
- 触发事件：`mujian` 提交 `bd04a90`（SvelteKit 3 迁移）导致「幕间」整站白屏
- 排查范围：本批依赖更新（2026-10-10）涉及的所有前端仓库
- 方法：依赖版本比对 + 配置键合法性核对 + 逐仓真实构建 + 无头 Chromium 运行时探针（console/pageerror 捕获 + DOM 节点计数判定白屏）

---

## 0. 根因结论（幕间白屏）

> **状态更新（2026-10-11 后续）**：同源缺陷在 huozhi 同步确认并修复，见 §2.1 与 §3。

| 项 | 内容 |
|---|---|
| 直接原因 | SvelteKit 3 **彻底移除** `$app/stores`。`bd04a90` 只改了 `$lib → #lib`，漏掉同版本被移除的 `$app/stores` |
| 崩溃点 | 8 个路由文件仍 `import { page } from '$app/stores'`；`+layout.svelte` 里 `$page` 在 `$derived` 中被求值 → 布局组件初始化即抛错 → 整站白屏 |
| 为何构建期未拦截 | Kit 3 保留了一个**抛错空壳**（`node_modules/@sveltejs/kit/src/runtime/app/stores.js`），`page.subscribe = removed`，只在运行时订阅才抛 `https://svelte.dev/e/kit/app_stores_removed`。构建、类型检查、后端日志均无任何线索 |
| 为何后端无日志 | 静态资源走 `slogLogger` 的 `/_app/` 跳过分支，客户端 JS 异常不经过服务端 |
| 已修复 | `38bf827`（改为 `$app/state` 的 `page` 响应式对象，`$page.x → page.x`），当前 HEAD 已验证正常 |

空壳源码实证（Kit 3.0.0）：

```js
// node_modules/@sveltejs/kit/src/runtime/app/stores.js
function removed() { e.app_stores_removed(); }
export const page = { subscribe: removed };
```

同一提交顺带修复的两处遗留：

1. `vite.config.js`：嵌在 `sveltekit({...})` 里的 `vite` 键被插件判为非法、整块静默丢弃 → dev 代理未生效。Kit 3 合法键只有 `files / serviceWorker / adapter / alias / appDir / compilerOptions / csp / experimental / moduleExtensions / outDir / paths / preprocess / prerender / typescript`，**没有 `vite`**。
2. `Makefile`：`cp -r` 缺少 `rm -rf backend/dist`，目标已存在时嵌套成 `backend/dist/dist`，`go:embed` 读到旧外壳。

---

## 1. 受影响范围总览

| 应用 | Kit 版本 | `$app/stores` 残留 | 白屏风险 | 判定 |
|---|---|---|---|---|
| huozhi（货殖） | ^3.0.0 | 0（已修） | 已修复（`71289cf6`） | ✅ 已修复 |
| mujian（幕间） | ^3.0.1 | 0（已修） | 已修复（`38bf827`） | ✅ 已排除 |
| qingye（青野集） | ^2.70.3 | 0 | 无 | ✅ 未升级 Kit |
| diarum（牵丝） | ^2.70.3 | 5 处（合法） | 无 | ✅ 未升级 Kit |
| qiansi（千丝） | 无 Kit | 0 | 无 | ✅ 不涉及 |
| shenshi（慎始） | React，无 Kit | 0 | 无 | ✅ 不涉及 |
| ntfy / tuqie / chaxin / bili-history / yuexi | 无 Svelte | 0 | 无 | ✅ 不涉及 |

---

## 2. 按应用分组详查

### 2.1 huozhi（货殖）—— ⚠️ 曾受影响，P0，**已修复**（`71289cf6`，已推送）

**是否受影响**：是。与幕间**同一根因、同一代码路径**。排查时确认存在，已按 `38bf827` 同一手法修复并推送。

**问题原因**：项目已在 `@sveltejs/kit@^3.0.0`（提交 `b3ddf6ab`，与幕间 `bd04a90` 同批），但 4 个文件仍从已被移除的 `$app/stores` 导入 `page`。

**代码位置**（均为 `$page` 自动订阅，触发 `subscribe → removed()`）：

| 文件 | 行 | 用法 | 触发条件 | 影响范围 |
|---|---|---|---|---|
| `frontend/src/routes/(auth)/+layout.svelte` | 3, 55, 157, **221** | `let currentPath = $derived($page?.url?.pathname \|\| '')` | 布局初始化即求值 | **所有需登录页面整站白屏** |
| `frontend/src/routes/(auth)/accounts/[id]/+page.svelte` | 2, 21 | `$derived(Number($page?.params?.id))` | 打开账户详情 | 该路由白屏 |
| `frontend/src/routes/(auth)/transactions/edit/[id]/+page.svelte` | 3, 6 | `$derived($page?.params?.id)` | 打开账单编辑 | 该路由白屏 |
| `frontend/src/routes/login/+page.svelte` | 2, **36** | `$page?.url?.searchParams.get('redirect')` | **登录成功后**，在 `try` 块内 | 异常被 `catch` 吞掉并显示为「登录失败」→ **凭据正确也永远登不进** |

**实证**（带 `hz_token` 绕过守卫，无头 Chromium 访问受保护路由，6/6 均抛错）：

```
/transactions   最终URL=/login  ❌ Error: https://svelte.dev/e/kit/app_stores_removed
/accounts/1     最终URL=/login  ❌ Error: https://svelte.dev/e/kit/app_stores_removed
/statistics     最终URL=/login  ❌ Error: https://svelte.dev/e/kit/app_stores_removed
/settings       最终URL=/login  ❌ Error: https://svelte.dev/e/kit/app_stores_removed
/categories     最终URL=/login  ❌ Error: https://svelte.dev/e/kit/app_stores_removed
```

> 注：探针因无真实后端被守卫重定向到 `/login`，故看到的是登录页而非白屏；但异常已真实抛出。若后端放行，`(auth)/+layout.svelte` 第 221 行会在布局初始化阶段崩溃，表现与幕间一致（整站白屏）。

**修复方式**：**代码适配，无需回退依赖**。按 `38bf827` 的同一手法迁移：

```svelte
- import { page } from '$app/stores';
+ import { page } from '$app/state';
  …
- let currentPath = $derived($page?.url?.pathname || '');
+ let currentPath = $derived(page?.url?.pathname || '');
```

Svelte 5 runes 下 `$derived` / `$effect` / 模板中的读取依旧被跟踪，路由高亮与侧栏行为不退化（幕间已验证）。

**顺带修复（P1）**：`frontend/tsconfig.json` 仍 `extends: "./.svelte-kit/tsconfig.json"`，Kit 3 要求改为 `"extends": "$app/tsconfig"`。旧路径不声明 `#lib` 子路径导入，导致全部 `#lib/*` 被判 `Cannot find module`，`svelte-check` 报 **241 错误**——等于类型门禁完全失效。改后 **0 error**。

**修复后验证**：

| 项 | 修前 | 修后 |
|---|---|---|
| 运行时 `app_stores_removed` | 6/6 路由抛出 | **0**（`/accounts` 渲染出 232 节点，`(auth)` 布局成功挂载） |
| `svelte-check` | 241 errors | **0 errors**（10 warnings，均为 UI 组件 `<slot>` 弃用，遗留项） |
| `pnpm build` | 通过 | 通过 |
| `pnpm test` | — | **175/175 通过** |

**附带发现（P2，既有问题，非 Kit 3 回归）**：Service Worker 从未进入产物。

- `vite-plugin-pwa@2.0.0` 把 `sw.js` / `workbox-*.js` 输出到 `frontend/dist/`，而 `adapter-static` 输出 `pages/assets: 'build'`，CI 只做 `cp -R frontend/build/. dist/` → `sw.js` 被遗漏。
- 运行时症状：`src/lib/pwa.ts` 调 `registerSW()` 拉取 `/sw.js`，服务端 SPA 回退返回 `index.html`，浏览器报 `The script has an unsupported MIME type ('text/html')`，离线能力静默失效。
- 该目录错配在迁移前就存在（迁移前 `svelte.config.js` 同样是 `pages: 'build'`），**不是 Kit 3 引入**。
- 对照：`diarum` 用 `@vite-pwa/sveltekit`（`SvelteKitPWA`，能识别 SvelteKit 的 outDir），`sw.js` 正确落在 `build/`；`mujian` 走自建 `scripts/build-sw.mjs`。
- 建议：改用 `@vite-pwa/sveltekit`，或显式指定 `swDest`/`outDir` 与 adapter 一致。

---

### 2.2 mujian（幕间）—— ✅ 已排除（已修复）

是否受影响：**曾受影响，当前 HEAD 已修复**。

排除理由：

- `38bf827` 已把 8 处 `$app/stores` 全量改为 `$app/state`；全仓 grep `$app/stores` 命中 **0**。
- 真实构建通过（`pnpm build` → `Wrote site to "dist"`，0 警告）。
- 无头 Chromium 运行时探针：`/`、`/records`、`/calendar`、`/analytics`、`/settings`、`/data`、`/map`、`/artists` 均正常渲染，**console / pageerror 零错误**。
- `/dramas` 出现 `TypeError: a is not a function or its return value is not iterable` 与「500 Internal Error」，经核为探针 stub API 返回 `{}` 所致——该页 `[dramas, categories] = await Promise.all([...])` 需数组。接真实后端即消失，**非 Kit 3 回归**。

---

### 2.3 qingye（青野集）—— ✅ 未受影响

排除理由：

- `@sveltejs/kit` 仍为 **^2.70.3**，本批只升了 `vite 8.3.1→8.3.3`、`vitest 5.0.2→5.0.3`，未触碰 major。`$app/stores` 在 Kit 2 仅为 deprecated、功能正常。
- 全仓 `$app/stores` 命中 0。
- 构建通过（`adapter-static` → `build`）。
- 运行时探针：`/`、`/plants`、`/tasks`、`/settings` 全部渲染正常（节点数 132/133/145/267），无 pageerror。

---

### 2.4 diarum（牵丝）—— ✅ 未受影响

排除理由：

- `@sveltejs/kit` 仍为 **^2.70.3**，本批只升 `vite-plugin-pwa 1.3.0→2.0.0`、`vite 8.3.1→8.3.3`。
- 虽有 5 处 `import { page } from '$app/stores'`，但在 Kit 2 下**合法且可用**，不构成白屏风险。
- PWA 产物落点正确：使用 `@vite-pwa/sveltekit`，`build/` 下已含 `sw.js` + `workbox-2da51cb1.js`，与 `//go:embed all:build` 一致。

待观察项（P3）：`typescript@^7.0.2` 与 `svelte-check@^4.7.6` 组合。Kit 3 侧已知 TS 7 不兼容（huozhi 为此降到 `~6.0.0`）；当前 Kit 2 未暴露问题，但若后续升 Kit 3 需一并处理。

---

### 2.5 qiansi（千丝）—— ✅ 未受影响

排除理由：不使用 SvelteKit（纯 Vite + Svelte 5），本批只升 `vite 8.3.2`、`@lucide/svelte 1.52.0`。`$app/stores` 命中 0。构建通过，`vite-plugin-pwa@1.3.0` 输出 `dist/sw.js`，与 `spaHandler` 的 `web/dist` 一致。

### 2.6 shenshi / ntfy / tuqie / chaxin / yuexi / bili-history / 其他 —— ✅ 不涉及

排除理由：非 Svelte 技术栈或无前端构建产物风险。本批仅 minor/patch 级更新，均已构建通过：

- `shenshi`：React + `@vitejs/plugin-react 6.1.1→6.1.2`，构建通过。
- `ntfy`：`vite 8.3.2`、`vitest 5.0.3`，构建通过，`build/sw.js` 正常生成。
- 其余仓库本批无前端依赖变更。

---

## 3. 修复建议（按优先级）

### P0 — huozhi 整站白屏 + 登录失效 —— ✅ 已完成（`71289cf6`）

| # | 动作 | 类别 | 状态 |
|---|---|---|---|
| 1 | 4 个文件 `$app/stores` → `$app/state` | **代码适配** | ✅ 已完成（含 `login/+page.svelte` 的登录后路径，否则修复后仍登不进） |
| 2 | `tsconfig.json` 改 `extends: "$app/tsconfig"` | **代码适配** | ✅ 已完成（241 → 0 error，恢复类型门禁） |

### P1 — 构建期可拦截性（待办）

| # | 动作 | 类别 | 说明 |
|---|---|---|---|
| 3 | 全仓禁写 `$app/stores` | **代码适配** | 加 ESLint `no-restricted-imports` 或在 `.github` 自检脚本加规则，防 Dependabot 再次触发同类迁移时漏改 |
| 4 | CI 增加运行时冒烟（非仅 `pnpm build`） | **代码适配** | Kit 3 的空壳让构建/类型检查全绿。建议把「构建产物 + 无头浏览器访问关键路由 + 断言 pageerror 为 0」纳入 reusable workflow，幕间与货殖这类问题才能自动拦住 |
| 5 | `pnpm check` 纳入 CI | **代码适配** | huozhi 有 `svelte-check` 但 CI 只跑 `test:coverage`。本次正是因此让 241 个类型错误潜伏至今 |

### P2 — huozhi Service Worker 缺失（既有问题，待办）

| # | 动作 | 类别 | 说明 |
|---|---|---|---|
| 6 | 改用 `@vite-pwa/sveltekit`，或对齐 `swDest`/`outDir` 与 adapter 的 `build/` | **代码适配** | 无需回退 `vite-plugin-pwa 2.0.0`，该版本本身无问题，错在 outDir 不匹配。运行时仍可见 `unsupported MIME type ('text/html')` |

### 关于「必须回退依赖版本」

**本批无一项需要回退。** Kit 3 移除 `$app/stores` 是既定破坏性变更，回退到 Kit 2 只会让 `svelte.config.js` 与 `#lib` 别名改造全部作废，且失去后续安全更新。4 个文件的迁移量小、已被幕间同一手法验证可行。

真正需要警惕的是：**Kit 3 对未适配代码只留运行时抛错空壳**，构建与类型检查全绿。因此防护重点应放在「运行时冒烟」而非「版本回退」。

---

## 4. 验证方式说明

| 手段 | 用途 |
|---|---|
| `pnpm build` 实跑 | muijan / huozhi / qingye / qiansi / shenshi / ntfy 全部通过 |
| 解析 `node_modules/@sveltejs/kit` | 确认 Kit 3 合法配置键与 `$app/stores` 抛错空壳 |
| Playwright + Chromium 探针 | 静态服务 + SPA 回退 + `localStorage` 注入 token；捕获 `console.error` / `pageerror` / 4xx，按 `body` 文本长度与节点数判定白屏 |
| 各仓 embed 目录核对 | 确认 `//go:embed all:dist|build` 均带 `all:` 前缀，`_app/` 不会被排除（此项全部合规） |
