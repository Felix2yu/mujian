<script>
  import { page } from '$app/stores';
  import { theme } from '$lib/stores.js';
  import { initStorageInfo } from '$lib/api.js';
  import { onMount } from 'svelte';
  import '$lib/app.css';

  const nav = [
    { href: '/', label: '记录', icon: '◎' },
    { href: '/analytics', label: '分析', icon: '◫' },
    { href: '/covers', label: '封面', icon: '▦' },
    { href: '/categories', label: '分类', icon: '❏' },
    { href: '/import', label: '导入', icon: '⇪' },
    { href: '/settings', label: '设置', icon: '⚙' }
  ];

  onMount(() => { initStorageInfo(); });

  function isActive(href) {
    const p = $page.url.pathname;
    if (href === '/') return p === '/';
    return p.startsWith(href);
  }

  function cycleTheme() {
    const order = ['auto', 'light', 'dark'];
    const cur = $theme || 'auto';
    theme.set(order[(order.indexOf(cur) + 1) % order.length]);
  }
</script>

<div class="app">
  <header class="topbar">
    <div class="bar-inner">
      <a class="brand" href="/">
        <span class="seal">幕</span>
        <span class="brand-name">幕间</span>
      </a>
      <nav class="nav">
        {#each nav as item}
          <a href={item.href} class="nav-link" class:active={isActive(item.href)}>
            <span class="ni">{item.icon}</span>{item.label}
          </a>
        {/each}
      </nav>
      <button class="theme-btn" on:click={cycleTheme} title="切换主题（自动 / 亮色 / 暗色）" aria-label="切换主题">
        {#if $theme === 'dark'}🌙{:else if $theme === 'light'}☀️{:else}◐{/if}
      </button>
    </div>
  </header>
  <main class="content">
    <slot />
  </main>
  <footer class="foot">
    <span class="muted tiny">幕间 · 现场演出记录</span>
  </footer>
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
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 30px;
    height: 30px;
    border-radius: 8px;
    background: linear-gradient(155deg, var(--accent), var(--accent-strong));
    color: #fff;
    font-family: var(--font-serif);
    font-weight: 700;
    font-size: 16px;
    box-shadow: 0 2px 6px -1px var(--accent);
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
    gap: 2px;
    overflow-x: auto;
    scrollbar-width: none;
  }
  .nav::-webkit-scrollbar { display: none; }
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
  .ni { font-size: 12px; opacity: 0.85; }

  .theme-btn {
    flex: 0 0 auto;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    border: 1px solid var(--border);
    background: var(--surface);
    font-size: 15px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: all var(--t-fast) var(--ease);
  }
  .theme-btn:hover { border-color: var(--accent); transform: rotate(18deg); }

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

  @media (max-width: 640px) {
    .bar-inner { padding: 0 14px; gap: 10px; height: 54px; }
    .brand-name { display: none; }
    .content { padding: 16px 14px 36px; }
  }
</style>
