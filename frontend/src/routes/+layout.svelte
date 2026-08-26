<script>
  import { page } from '$app/stores';
  import { initStorageInfo } from '$lib/api.js';
  import { onMount } from 'svelte';
  import '$lib/app.css';

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

  // 移动端顶栏只直显高频入口，其余收进「⋯」二次菜单
  const primaryNav = ['/', '/calendar', '/dramas', '/artists'];
  const secondaryNav = nav.filter((n) => !primaryNav.includes(n.href));

  let moreOpen = $state(false);

  onMount(() => { initStorageInfo(); });

  function isActive(href) {
    const p = $page.url.pathname;
    if (href === '/') return p === '/';
    return p.startsWith(href);
  }

  // 路由变化后收起「更多」面板
  $effect(() => {
    $page.url.pathname;
    moreOpen = false;
  });
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
          <a
            href={item.href}
            class="nav-link"
            class:active={isActive(item.href)}
            class:secondary-entry={!primaryNav.includes(item.href)}
          >
            {item.label}
          </a>
        {/each}
        <button
          type="button"
          class="nav-link more-btn"
          class:active={moreOpen}
          onclick={() => (moreOpen = !moreOpen)}
          aria-label="更多页面"
          aria-expanded={moreOpen}
        >
          <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" aria-hidden="true">
            <circle cx="5" cy="12" r="1.9" />
            <circle cx="12" cy="12" r="1.9" />
            <circle cx="19" cy="12" r="1.9" />
          </svg>
        </button>
      </nav>
    </div>
    {#if moreOpen}
      <button type="button" class="more-backdrop" onclick={() => (moreOpen = false)} aria-label="关闭菜单"></button>
      <div class="more-panel" role="menu">
        {#each secondaryNav as item (item.href)}
          <a href={item.href} class="more-item" class:active={isActive(item.href)} role="menuitem">
            {item.label}
          </a>
        {/each}
      </div>
    {/if}
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

  /* 「⋯」按钮与二次面板：仅移动端 */
  .more-btn { display: none; cursor: pointer; border: none; background: none; }
  .more-btn svg { display: block; }

  .more-backdrop {
    position: fixed;
    inset: 0;
    z-index: 49;
    border: none;
    background: transparent;
    cursor: default;
  }
  .more-panel {
    position: absolute;
    top: calc(100% + 8px);
    right: max(12px, calc((100% - 1120px) / 2 + 20px));
    z-index: 60;
    display: grid;
    grid-template-columns: repeat(2, minmax(104px, 1fr));
    gap: 2px;
    padding: 8px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    animation: panelIn 0.16s var(--ease) both;
    transform-origin: top right;
  }
  @keyframes panelIn {
    from { opacity: 0; transform: translateY(-6px) scale(0.97); }
    to { opacity: 1; transform: translateY(0) scale(1); }
  }
  .more-item {
    padding: 10px 16px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    color: var(--text-2);
    white-space: nowrap;
    transition: all var(--t-fast) var(--ease);
  }
  .more-item:hover { background: var(--surface-3); color: var(--text); }
  .more-item.active {
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 600;
  }

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

    /* 高频四项 + 「⋯」，一行放得下；低频入口收进二次面板 */
    .nav-link { padding: 6px 10px; font-size: 13.5px; }
    .nav-link.secondary-entry { display: none; }
    .more-btn { display: inline-flex; }
  }
</style>
