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

  let navEl;

  onMount(() => { initStorageInfo(); });

  function isActive(href) {
    const p = $page.url.pathname;
    if (href === '/') return p === '/';
    return p.startsWith(href);
  }

  // 移动端导航横向滚动：路由切换后把 active 项滚入可视区
  $effect(() => {
    $page.url.pathname;
    const el = navEl?.querySelector('.nav-link.active');
    if (!el || !navEl) return;
    const pad = 14;
    if (el.offsetLeft < navEl.scrollLeft + pad) {
      navEl.scrollTo({ left: Math.max(0, el.offsetLeft - pad), behavior: 'smooth' });
    } else if (el.offsetLeft + el.offsetWidth > navEl.scrollLeft + navEl.clientWidth - pad) {
      navEl.scrollTo({ left: el.offsetLeft + el.offsetWidth - navEl.clientWidth + pad, behavior: 'smooth' });
    }
  });
</script>

<div class="app">
  <header class="topbar">
    <div class="bar-inner">
      <a class="brand" href="/">
        <span class="seal">幕</span>
        <span class="brand-name">幕间</span>
      </a>
      <nav class="nav" bind:this={navEl}>
        {#each nav as item}
          <a href={item.href} class="nav-link" class:active={isActive(item.href)}>
            {item.label}
          </a>
        {/each}
      </nav>
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
    /* offsetLeft 需以 nav 为定位上下文，供 active 项滚动计算使用 */
    position: relative;
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
    /* 两行布局：品牌一行，导航独占第二行通栏横向滑动 */
    .bar-inner {
      flex-wrap: wrap;
      height: auto;
      padding: 9px 14px 0;
      gap: 10px;
      row-gap: 0;
    }
    .brand { padding-bottom: 2px; }
    .nav {
      flex: 1 1 100%;
      margin: 0 -14px;
      padding: 4px 14px 8px;
      /* 两端渐隐提示可滑动 */
      -webkit-mask-image: linear-gradient(to right, transparent, #000 18px, #000 calc(100% - 28px), transparent);
      mask-image: linear-gradient(to right, transparent, #000 18px, #000 calc(100% - 28px), transparent);
    }
    .content { padding: 16px 14px 36px; }
  }
</style>
