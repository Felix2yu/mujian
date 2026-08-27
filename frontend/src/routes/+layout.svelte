<script>
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import { initStorageInfo } from '$lib/api.js';
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

  onMount(() => {
    initTheme();
    initStorageInfo();

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
</script>

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
    <aside class="drawer" transition:fly={{ x: 300, duration: 220, easing: cubicOut }} aria-label="站点导航">
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

  <main class="content">
    {@render children()}
  </main>
  <footer class="foot">
    <span class="muted tiny">幕间 · 现场演出记录</span>
  </footer>

  {#if updateReady}
    <button type="button" class="update-banner" onclick={applyUpdate} transition:fly={{ y: 60, duration: 220 }}>
      发现新版本，点击刷新
    </button>
  {/if}
</div>

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
    transition: all var(--t-fast) var(--ease);
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
    transition: all var(--t-fast) var(--ease);
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
    transition: all var(--t-fast) var(--ease);
  }
  .drawer-close:hover { background: var(--surface-3); color: var(--text); }

  .drawer-nav {
    flex: 1;
    overflow-y: auto;
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
    transition: all var(--t-fast) var(--ease);
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
    transition: all var(--t-fast) var(--ease);
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
</style>
