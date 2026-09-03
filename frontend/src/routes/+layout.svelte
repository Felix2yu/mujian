<script>
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { api, initStorageInfo, getAuthToken, verifyAuthToken } from '$lib/api.js';
  import { initTheme } from '$lib/stores.js';
  import { onMount } from 'svelte';
  import { fade, fly } from 'svelte/transition';
  import { cubicOut } from 'svelte/easing';
  import '$lib/app.css';

  let { children } = $props();

  const nav = [
    { href: '/', label: '记录' },
    { href: '/calendar', label: '日历' },
    { href: '/dramas', label: '剧目' },
    { href: '/artists', label: '演员' },
    { href: '/map', label: '地图' },
    { href: '/analytics', label: '分析' },
    { href: '/covers', label: '封面' },
    { href: '/categories', label: '分类' },
    { href: '/import', label: '数据' },
    { href: '/settings', label: '设置' }
  ];

  // 移动端侧栏不列「日历」：日历改为悬浮快捷入口，避免与侧栏菜单重复。
  const drawerNav = nav.filter(item => item.href !== '/calendar');

  let drawerOpen = $state(false);

  // PWA 更新：检测到新 SW 就绪时提示用户刷新
  let updateReady = $state(false);
  let swReg = null;

  // Web Vitals 采集：每次页面加载上报一次（fire-and-forget sendBeacon），
  // 失败静默——遥测绝不能影响正常使用。
  function reportWebVitals() {
    try {
      if (typeof navigator.sendBeacon !== 'function') return;
      const nav = performance.getEntriesByType('navigation')[0];
      const v = { route: $page.url.pathname, ttfb_ms: nav ? Math.round(nav.responseStart) : 0 };
      const paint = performance.getEntriesByType('paint') || [];
      for (const p of paint) {
        if (p.name === 'first-contentful-paint') v.fcp_ms = Math.round(p.startTime);
      }
      let cls = 0;
      try {
        new PerformanceObserver((l) => {
          for (const e of l.getEntries()) if (!e.hadRecentInput) cls += e.value;
        }).observe({ type: 'layout-shift', buffered: true });
      } catch {}
      try {
        new PerformanceObserver((l) => {
          const es = l.getEntries();
          if (es.length) v.lcp_ms = Math.round(es[es.length - 1].startTime);
        }).observe({ type: 'largest-contentful-paint', buffered: true });
      } catch {}
      let longTask = 0;
      try {
        new PerformanceObserver((l) => {
          for (const e of l.getEntries()) longTask += e.duration;
        }).observe({ type: 'longtask', buffered: true });
      } catch {}

      // 给 LCP/CLS/长任务 一点完成时间，再随页面卸货前发出。
      setTimeout(() => {
        v.cls = cls.toFixed(3);
        v.longtask_ms = Math.round(longTask);
        navigator.sendBeacon('/api/metrics/client', JSON.stringify(v));
      }, 1500);
    } catch {}
  }

  // ---------- 全站访问令牌门 ----------
  // 服务端启用鉴权（MJ_AUTH_TOKEN）后，任何页面都会 401；与其让每个页面
  // 静默显示空列表，不如在最外层拦下：启动时探测一次，未通过则整站只渲染
  // 令牌输入页，校验通过才放行渲染应用。会话中途令牌失效（服务端更换）
  // 由 api.js 广播的 mujian:unauthorized 事件重新拉起本门。
  let authState = $state('checking'); // checking | ok | locked
  let tokenInput = $state('');
  let authError = $state('');
  let verifying = $state(false);

  async function checkAuth() {
    try {
      // GET /api/settings 免鉴权，仅用它的 auth_required 判断是否需要令牌。
      const s = await api.getSettings();
      if (!s || !s.auth_required) {
        authState = 'ok';
        return;
      }
      const saved = getAuthToken();
      if (saved && (await verifyAuthToken(saved))) {
        authState = 'ok';
        return;
      }
      authState = 'locked';
    } catch (e) {
      // 探测失败（网络异常等）时不锁死应用，各页面自行展示错误。
      authState = 'ok';
    }
  }

  async function submitToken(e) {
    e.preventDefault();
    const t = tokenInput.trim();
    if (!t) {
      authError = '请输入访问令牌';
      return;
    }
    verifying = true;
    authError = '';
    if (await verifyAuthToken(t)) {
      try {
        localStorage.setItem('mujian:auth_token', t);
      } catch (err) { /* ignore */ }
      tokenInput = '';
      authState = 'ok';
    } else {
      authError = '令牌不正确，或网络异常，请重试';
    }
    verifying = false;
  }

  $effect(() => {
    const on401 = () => {
      if (authState === 'ok') authState = 'locked';
    };
    window.addEventListener('mujian:unauthorized', on401);
    return () => window.removeEventListener('mujian:unauthorized', on401);
  });

  onMount(() => {
    checkAuth();
    initTheme();
    initStorageInfo();
    reportWebVitals();

    if ('serviceWorker' in navigator && !/^localhost$|^127\.0\.0\.1$|^\[::1\]$/.test(location.hostname)) {
      navigator.serviceWorker.register('/sw.js').then((reg) => {
        swReg = reg;
        reg.addEventListener('updatefound', () => {
          const installing = reg.installing;
          installing?.addEventListener('statechange', () => {
            if (installing.state === 'installed' && navigator.serviceWorker.controller) {
              updateReady = true;
            }
          });
        });
        // 打开页面时若已有一个等待中的新 SW（上一个标签页装好了），直接提示
        if (reg.waiting && navigator.serviceWorker.controller) updateReady = true;
      });
      let reloaded = false;
      navigator.serviceWorker.addEventListener('controllerchange', () => {
        if (!reloaded) {
          reloaded = true;
          location.reload();
        }
      });
    }
  });

  function applyUpdate() {
    if (swReg?.waiting) swReg.waiting.postMessage({ type: 'SKIP_WAITING' });
    else location.reload();
  }

  function isActive(href) {
    const p = $page.url.pathname;
    if (href === '/') return p === '/';
    return p.startsWith(href);
  }

  // 路由变化后自动收起侧栏
  $effect(() => {
    $page.url.pathname;
    drawerOpen = false;
  });

  // 侧栏打开时锁定页面滚动，Esc 关闭
  $effect(() => {
    if (!drawerOpen) return;
    document.body.style.overflow = 'hidden';
    const onKey = (e) => { if (e.key === 'Escape') drawerOpen = false; };
    window.addEventListener('keydown', onKey);
    return () => {
      document.body.style.overflow = '';
      window.removeEventListener('keydown', onKey);
    };
  });

  // 侧栏打开后 trap focus（Tab / Shift+Tab 不出侧栏）
  $effect(() => {
    if (!drawerOpen) return;
    const trapSelector = 'button, a, [tabindex]:not([tabindex="-1"])';
    const focusable = () => Array.from(document.querySelectorAll(`.drawer ${trapSelector}`)).filter(
      (el) => !el.hasAttribute('disabled') && el.offsetParent !== null
    );
    const nodes = focusable();
    if (nodes.length === 0) return;
    const first = nodes[0], last = nodes[nodes.length - 1];
    // 打开时先聚焦第一个可交互元素（关闭按钮）
    // first.focus({ preventScroll: true });  // 让按钮/链接通过过渡接管
    const onKey = (e) => {
      if (e.key !== 'Tab') return;
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || !nodes.includes(active)) {
          e.preventDefault(); last.focus({ preventScroll: true });
        }
      } else {
        if (active === last) {
          e.preventDefault(); first.focus({ preventScroll: true });
        }
      }
    };
    document.addEventListener('keydown', onKey, true);
    // 过渡结束后把焦点送入 drawer
    const t = setTimeout(() => first.focus({ preventScroll: true }), 180);
    return () => {
      document.removeEventListener('keydown', onKey, true);
      clearTimeout(t);
    };
  });

  // ============ 全局封面灯箱：事件委托 + 统一灯箱 UI ============
  // 任何带 .coverable 类的元素点击都会弹出原图。支持：
  //   - Leaflet 弹窗里 innerHTML 注入的 img （无法绑 Svelte 事件）
  //   - Svelte 组件里的普通 img / button.coverable
  //   原图 URL 从优先级读取：data-full  > src
  let globalLightbox = $state(false);
  let globalLightboxSrc = $state('');

  $effect(() => {
    const onClick = (e) => {
      const el = e.target.closest('.coverable');
      if (!el) return;
      e.preventDefault();
      e.stopPropagation();
      const full = el.dataset.full || (el.tagName === 'IMG' ? el.src : el.querySelector('img')?.src) || '';
      if (full) { globalLightboxSrc = full; globalLightbox = true; }
    };
    const onKey = (e) => { if (e.key === 'Escape') { globalLightbox = false; globalLightboxSrc = ''; } };
    document.addEventListener('click', onClick, true); // 捕获阶段，绕过 popup 等内部 stopPropagation
    window.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('click', onClick, true);
      window.removeEventListener('keydown', onKey);
    };
  });
</script>

<a class="skip-link" href="#main">跳到主要内容</a>

<div class="app">
  <header class="topbar">
    <div class="bar-inner">
      <a class="brand" href="/">
        <img class="seal" src="/favicon.svg" alt="幕间" />
        <span class="brand-name">幕间</span>
      </a>
      <nav class="nav">
        {#each nav as item}
          <a href={item.href} class="nav-link" class:active={isActive(item.href)}>
            {item.label}
          </a>
        {/each}
      </nav>
      <!-- 移动端日历快捷入口：顶栏内、汉堡按钮左侧，不进侧栏菜单。
           再次点击（已在日历页）则退出日历返回首页。 -->
      <button
        type="button"
        class="topbar-cal"
        class:active={isActive('/calendar')}
        aria-label={isActive('/calendar') ? '退出日历，返回首页' : '打开日历'}
        aria-pressed={isActive('/calendar')}
        onclick={() => goto(isActive('/calendar') ? '/' : '/calendar')}
      >
        <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <rect x="3" y="4" width="18" height="18" rx="2" />
          <path d="M3 9h18M8 2v4M16 2v4M8 14h2M14 14h2M8 18h2M14 18h2" />
        </svg>
      </button>
      <button
        type="button"
        class="menu-btn"
        onclick={() => (drawerOpen = true)}
        aria-label="打开菜单"
        aria-expanded={drawerOpen}
      >
        <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <path d="M4 7h16M4 12h16M4 17h16" />
        </svg>
      </button>
    </div>
  </header>

  {#if drawerOpen}
    <button
      type="button"
      class="drawer-backdrop"
      onclick={() => (drawerOpen = false)}
      aria-label="关闭菜单"
      transition:fade={{ duration: 140 }}
    ></button>
    <aside class="drawer" transition:fly={{ x: 300, duration: 220, easing: cubicOut }} aria-label="站点导航" role="dialog" aria-modal="true">
      <div class="drawer-head">
        <img class="seal" src="/favicon.svg" alt="幕间" />
        <span class="drawer-title">幕间</span>
        <button type="button" class="drawer-close" onclick={() => (drawerOpen = false)} aria-label="关闭菜单">
          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
            <path d="M6 6l12 12M18 6L6 18" />
          </svg>
        </button>
      </div>
      <nav class="drawer-nav">
        {#each drawerNav as item (item.href)}
          <a href={item.href} class="drawer-link" class:active={isActive(item.href)}>
            {item.label}
            {#if isActive(item.href)}<span class="dot" aria-hidden="true"></span>{/if}
          </a>
        {/each}
      </nav>
      <div class="drawer-foot muted tiny">幕间 · 现场演出记录</div>
    </aside>
  {/if}

  <main id="main" class="content">
    {#if authState === 'ok'}
      {@render children()}
    {:else if authState === 'checking'}
      <div class="auth-loading muted">加载中…</div>
    {/if}
  </main>
  <footer class="foot">
    <span class="muted tiny">幕间 · 现场演出记录</span>
  </footer>

  {#if authState === 'locked'}
    <div class="auth-gate" role="dialog" aria-modal="true" aria-label="访问验证">
      <form class="auth-card" onsubmit={submitToken}>
        <img class="auth-seal" src="/favicon.svg" alt="" />
        <h1 class="auth-title">幕间</h1>
        <p class="auth-sub">此服务已开启访问验证，请输入访问令牌继续</p>
        <input
          class="input"
          type="password"
          bind:value={tokenInput}
          placeholder="访问令牌"
          autocomplete="current-password"
          disabled={verifying}
        />
        {#if authError}<div class="auth-error" role="alert">{authError}</div>{/if}
        <button class="btn primary" type="submit" disabled={verifying}>
          {verifying ? '验证中…' : '进入'}
        </button>
      </form>
    </div>
  {/if}

  {#if updateReady}
    <button type="button" class="update-banner" onclick={applyUpdate} transition:fly={{ y: 60, duration: 220 }} role="status" aria-live="polite">
      发现新版本，点击刷新
    </button>
  {/if}
</div>

{#if globalLightbox && globalLightboxSrc}
  <button type="button" class="global-lightbox" onclick={() => { globalLightbox = false; globalLightboxSrc = ''; }} aria-label="关闭大图">
    <img src={globalLightboxSrc} alt="" />
  </button>
{/if}

<style>
  .app { min-height: 100vh; display: flex; flex-direction: column; }

  .topbar {
    position: sticky;
    top: 0;
    z-index: 50;
    background: color-mix(in srgb, var(--bg-elevated) 82%, transparent);
    -webkit-backdrop-filter: blur(14px) saturate(1.4);
    backdrop-filter: blur(14px) saturate(1.4);
    border-bottom: 1px solid var(--border);
  }
  .bar-inner {
    max-width: 1120px;
    margin: 0 auto;
    padding: 0 20px;
    height: 58px;
    display: flex;
    align-items: center;
    gap: 18px;
  }

  .brand { display: flex; align-items: center; gap: 9px; flex: 0 0 auto; }
  .seal {
    width: 30px;
    height: 30px;
    border-radius: 8px;
    overflow: hidden;
    border: 1px solid var(--border);
    flex-shrink: 0;
    transition: transform var(--t-med) var(--ease);
  }
  .brand:hover .seal { transform: rotate(-6deg) scale(1.05); }
  .brand-name {
    font-family: var(--font-serif);
    font-size: 19px;
    font-weight: 700;
    letter-spacing: 0.06em;
  }

  .nav {
    flex: 1;
    display: flex;
    justify-content: flex-end;
    gap: 2px;
  }
  .nav-link {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 13px;
    border-radius: 999px;
    font-size: 14px;
    color: var(--text-muted);
    white-space: nowrap;
    transition: color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .nav-link:hover { color: var(--text); background: var(--surface-3); }
  .nav-link.active {
    color: var(--accent);
    background: var(--accent-soft);
    font-weight: 600;
  }

  /* 汉堡按钮：仅移动端 */
  .menu-btn {
    display: none;
    width: 38px;
    height: 38px;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    box-shadow: var(--shadow-xs);
    transition: color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .menu-btn:active { transform: scale(0.95); }
  .menu-btn svg { display: block; }

  /* ============ 移动端侧栏 ============ */
  .drawer-backdrop {
    position: fixed;
    inset: 0;
    z-index: 60;
    border: none;
    padding: 0;
    background: rgba(0, 0, 0, 0.42);
    cursor: default;
    overscroll-behavior: contain;
  }
  .drawer {
    position: fixed;
    top: 0;
    bottom: 0;
    right: 0;
    z-index: 61;
    width: min(78vw, 300px);
    display: flex;
    flex-direction: column;
    background: var(--bg-elevated);
    border-left: 1px solid var(--border);
    box-shadow: var(--shadow-lg);
    padding-bottom: calc(16px + env(safe-area-inset-bottom, 0px));
    overscroll-behavior: contain;
  }
  .drawer-head {
    display: flex;
    align-items: center;
    gap: 9px;
    padding: 16px 16px 12px;
    border-bottom: 1px solid var(--border);
  }
  .drawer-title {
    font-family: var(--font-serif);
    font-size: 19px;
    font-weight: 700;
    letter-spacing: 0.06em;
    flex: 1;
  }
  .drawer-close {
    border: none;
    background: none;
    color: var(--text-muted);
    width: 34px;
    height: 34px;
    border-radius: 50%;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .drawer-close:hover { background: var(--surface-3); color: var(--text); }

  .drawer-nav {
    flex: 1;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 10px 12px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .drawer-link {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 12px 14px;
    border-radius: var(--radius-sm);
    font-size: 15px;
    color: var(--text-2);
    transition: color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .drawer-link:hover { background: var(--surface-3); color: var(--text); }
  .drawer-link.active {
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }
  .drawer-link .dot {
    margin-left: auto;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--accent);
  }
  .drawer-foot { padding: 10px 18px 0; text-align: center; }

  .content {
    flex: 1;
    width: 100%;
    max-width: 1120px;
    margin: 0 auto;
    padding: 22px 20px 40px;
    box-sizing: border-box;
  }
  .foot {
    padding: 18px 20px 26px;
    text-align: center;
  }

  /* PWA 更新提示横幅 */
  .update-banner {
    position: fixed;
    left: 50%;
    bottom: calc(20px + env(safe-area-inset-bottom, 0px));
    transform: translateX(-50%);
    z-index: 70;
    border: none;
    padding: 11px 22px;
    border-radius: 999px;
    background: var(--accent-strong);
    color: #fff;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    box-shadow: var(--shadow-lg);
    transition: filter var(--t-fast) var(--ease);
  }
  .update-banner:hover { filter: brightness(1.08); }

  /* 移动端日历快捷入口：顶栏按钮，位于汉堡左侧 */
  .topbar-cal {
    display: none;
    margin-left: auto;
    width: 38px;
    height: 38px;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    background: var(--surface);
    color: var(--text);
    cursor: pointer;
    box-shadow: var(--shadow-xs);
    transition: color var(--t-fast) var(--ease), background var(--t-fast) var(--ease), border-color var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .topbar-cal:active { transform: scale(0.95); }
  .topbar-cal svg { display: block; }
  .topbar-cal.active {
    color: var(--accent);
    border-color: var(--accent);
    background: var(--accent-soft);
  }

  @media (max-width: 640px) {
    .bar-inner { padding: 0 14px; gap: 10px; height: 54px; }
    .content { padding: 16px 14px 36px; }
    /* 顶栏只留品牌 + 日历 + 汉堡，全部入口进侧栏 */
    .nav { display: none; }
    .menu-btn { display: inline-flex; }
    /* 移动端显示顶栏日历入口（在汉堡左侧） */
    .topbar-cal { display: inline-flex; }
  }

  /* ---------- 全站访问令牌门 ---------- */
  .auth-loading { padding: 48px 0; text-align: center; }

  .auth-gate {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    background: var(--bg);
  }

  .auth-card {
    width: min(360px, 100%);
    display: flex;
    flex-direction: column;
    gap: 12px;
    text-align: center;
    padding: 36px 30px;
    border-radius: var(--radius-lg, 16px);
    background: var(--surface);
    border: 1px solid var(--border);
    box-shadow: 0 16px 48px rgb(0 0 0 / 0.14);
  }

  .auth-seal { width: 44px; height: 44px; margin: 0 auto; }
  .auth-title { margin: 0; font-size: 22px; letter-spacing: 0.08em; }
  .auth-sub { margin: 0 0 6px; color: var(--text-2); font-size: 13px; }
  .auth-card .input { text-align: center; }
  .auth-error { color: var(--danger); font-size: 13px; }

  /* 全局灯箱：被 .coverable 类元素点击触发 */
  .global-lightbox {
    position: fixed;
    inset: 0;
    z-index: 9999;
    border: none;
    padding: 24px;
    margin: 0;
    background: rgba(0, 0, 0, 0.88);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    cursor: zoom-out;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .global-lightbox img {
    max-width: min(92vw, 720px);
    max-height: 88vh;
    width: auto;
    height: auto;
    border-radius: var(--radius);
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.6);
    user-select: none;
    -webkit-user-drag: none;
  }
  .coverable { cursor: zoom-in; }
  .coverable img { cursor: zoom-in; }
</style>
