<script>
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { theme } from '$lib/stores';
  import { onMount } from 'svelte';
  import { requestPermission, startReminderCheck, checkUpcomingShows } from '$lib/notifications';

  let searchQuery = $state('');
  let searchResults = $state([]);
  let showSearch = $state(false);
  let deferredPrompt = $state(null);
  let showInstall = $state(false);
  let mobileMenuOpen = $state(false);
  let hasUpcoming = $state(false);

  onMount(async () => {
    try {
      const settingsRes = await api.getSettings();
      if (settingsRes.theme) {
        theme.set(settingsRes.theme);
      }
    } catch (e) {
      console.error('Failed to load data:', e);
    }

    window.addEventListener('beforeinstallprompt', (e) => {
      e.preventDefault();
      deferredPrompt = e;
      showInstall = true;
    });

    requestPermission().then(granted => {
      if (granted) {
        startReminderCheck(() => api.listAllShows());
      }
    });

    try {
      const allShows = await api.listAllShows();
      hasUpcoming = checkUpcomingShows(allShows).length > 0;
    } catch {}
  });

  async function installApp() {
    if (!deferredPrompt) return;
    deferredPrompt.prompt();
    const { outcome } = await deferredPrompt.userChoice;
    if (outcome === 'accepted') {
      showInstall = false;
    }
    deferredPrompt = null;
  }

  async function handleSearch() {
    if (!searchQuery.trim()) {
      searchResults = [];
      return;
    }
    try {
      searchResults = await api.searchShows(searchQuery);
      showSearch = true;
    } catch (e) {
      console.error('Search failed:', e);
    }
  }

  function closeSearch() {
    showSearch = false;
    searchQuery = '';
    searchResults = [];
  }

  let currentPath = $derived($page.url.pathname);
</script>

<div class="app">
  <nav class="navbar">
    <div class="nav-inner">
      <div class="nav-brand">
        <a href="/">幕间</a>
      </div>

      <div class="nav-links" class:open={mobileMenuOpen}>
        <a href="/" class:active={currentPath === '/'} onclick={() => mobileMenuOpen = false}>日历</a>
        <a href="/shows" class:active={currentPath.startsWith('/shows') && !currentPath.includes('/import') && !currentPath.includes('/new')} onclick={() => mobileMenuOpen = false}>演出</a>
        <a href="/analytics" class:active={currentPath === '/analytics'} onclick={() => mobileMenuOpen = false}>数据分析</a>
        <a href="/shows/new" class:active={currentPath === '/shows/new'} onclick={() => mobileMenuOpen = false}>添加演出</a>
      </div>

      <div class="nav-search">
        <form onsubmit={(e) => { e.preventDefault(); handleSearch(); }}>
          <div class="search-wrapper">
            <svg class="search-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
            <input
              type="text"
              placeholder="搜索演出..."
              bind:value={searchQuery}
              onblur={() => setTimeout(closeSearch, 200)}
            />
          </div>
        </form>
        {#if showSearch && searchResults.length > 0}
          <div class="search-results">
            {#each searchResults.slice(0, 5) as show}
              <a href="/shows/{show.id}" class="search-item">
                <span class="search-name">{show.name}</span>
                <span class="search-venue">{show.venue}</span>
              </a>
            {/each}
            {#if searchResults.length > 5}
              <a href="/search?q={encodeURIComponent(searchQuery)}" class="search-more">
                查看全部 {searchResults.length} 条结果 →
              </a>
            {/if}
          </div>
        {/if}
      </div>

      <div class="nav-right">
        {#if showInstall}
          <button class="icon-btn install-btn" onclick={installApp} title="安装应用">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
          </button>
        {/if}
        <button class="icon-btn notify-btn" class:has-upcoming={hasUpcoming} onclick={requestPermission} title="通知">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>
        </button>
        <a href="/settings" class="icon-btn" class:active={currentPath === '/settings'} title="设置">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M12 1v2m0 18v2M4.22 4.22l1.42 1.42m12.72 12.72 1.42 1.42M1 12h2m18 0h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>
        </a>

        <button class="mobile-menu-btn" onclick={() => mobileMenuOpen = !mobileMenuOpen}>
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            {#if mobileMenuOpen}
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            {:else}
              <line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="18" x2="21" y2="18"/>
            {/if}
          </svg>
        </button>
      </div>
    </div>
  </nav>

  <main>
    <slot />
  </main>
</div>

<style>
  :global(:root) {
    --bg-body: #f8f9fc;
    --bg-card: #ffffff;
    --bg-card-hover: #f8f9fc;
    --bg-surface: #f1f3f9;
    --bg-surface-hover: #e5e8f0;
    --bg-input: #ffffff;
    --border: #e2e5f0;
    --border-hover: #d0d5e4;
    --text-primary: #1a1d2e;
    --text-secondary: #5a6178;
    --text-muted: #8b91a8;
    --accent: #6366f1;
    --accent-light: #818cf8;
    --accent-bg: #eef2ff;
    --danger-bg: #fef2f2;
    --danger-text: #dc2626;
    --danger-bg-hover: #fee2e2;
    --success: #10b981;
    --success-bg: #ecfdf5;
    --warning: #f59e0b;
    --warning-bg: #fffbeb;
    --shadow-sm: 0 1px 2px rgba(0,0,0,0.04);
    --shadow-md: 0 4px 12px rgba(0,0,0,0.06);
    --shadow-lg: 0 8px 30px rgba(0,0,0,0.08);
    --radius-sm: 8px;
    --radius-md: 12px;
    --radius-lg: 16px;
    --radius-xl: 20px;
  }

  :global(.dark) {
    --bg-body: #0f1117;
    --bg-card: #1a1d2e;
    --bg-card-hover: #222639;
    --bg-surface: #222639;
    --bg-surface-hover: #2a2e44;
    --bg-input: #1a1d2e;
    --border: #2a2e44;
    --border-hover: #3a3f5c;
    --text-primary: #e8eaf0;
    --text-secondary: #9ca3bf;
    --text-muted: #6b7394;
    --accent: #818cf8;
    --accent-light: #a5b4fc;
    --accent-bg: rgba(99,102,241,0.15);
    --danger-bg: rgba(220,38,38,0.15);
    --danger-text: #f87171;
    --danger-bg-hover: rgba(220,38,38,0.25);
    --success: #34d399;
    --success-bg: rgba(16,185,129,0.15);
    --warning: #fbbf24;
    --warning-bg: rgba(245,158,11,0.15);
    --shadow-sm: 0 1px 2px rgba(0,0,0,0.2);
    --shadow-md: 0 4px 12px rgba(0,0,0,0.3);
    --shadow-lg: 0 8px 30px rgba(0,0,0,0.4);
  }

  :global(*) {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'SF Pro Display', 'SF Pro Text', 'Segoe UI', Roboto, sans-serif;
    background: var(--bg-body);
    color: var(--text-primary);
    line-height: 1.6;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
    transition: background 0.3s, color 0.3s;
  }

  :global(a) {
    color: inherit;
    text-decoration: none;
  }

  :global(button) {
    cursor: pointer;
    border: none;
    background: none;
    font: inherit;
    color: inherit;
  }

  :global(input, select, textarea) {
    font: inherit;
    border-radius: var(--radius-sm);
    padding: 10px 14px;
    background: var(--bg-input);
    color: var(--text-primary);
    border: 1.5px solid var(--border);
    transition: all 0.2s ease;
  }

  :global(input:focus, select:focus, textarea:focus) {
    outline: none;
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
  }

  :global(.dark select option) {
    background: var(--bg-input);
    color: var(--text-primary);
  }

  :global(h1) { font-weight: 700; letter-spacing: -0.02em; }
  :global(h2) { font-weight: 600; letter-spacing: -0.01em; }
  :global(h3) { font-weight: 600; }

  .app {
    min-height: 100vh;
  }

  .navbar {
    background: rgba(255,255,255,0.8);
    backdrop-filter: blur(20px) saturate(180%);
    -webkit-backdrop-filter: blur(20px) saturate(180%);
    border-bottom: 1px solid var(--border);
    padding: 0 32px;
    position: sticky;
    top: 0;
    z-index: 100;
    transition: background 0.3s;
  }

  .nav-inner {
    max-width: 1400px;
    margin: 0 auto;
    display: flex;
    align-items: center;
    gap: 32px;
    height: 64px;
  }

  :global(.dark) .navbar {
    background: rgba(15,17,23,0.85);
  }

  .nav-brand a {
    font-size: 22px;
    font-weight: 800;
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
    letter-spacing: -0.02em;
  }

  .nav-links {
    display: flex;
    gap: 4px;
  }

  .nav-links a {
    padding: 8px 16px;
    border-radius: var(--radius-sm);
    transition: all 0.2s ease;
    white-space: nowrap;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-secondary);
  }

  .nav-links a:hover {
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  .nav-links a.active {
    background: var(--accent);
    color: #fff;
  }

  .nav-search {
    position: relative;
    flex: 1;
    max-width: 320px;
  }

  .search-wrapper {
    position: relative;
    display: flex;
    align-items: center;
  }

  .search-icon {
    position: absolute;
    left: 12px;
    color: var(--text-muted);
    pointer-events: none;
  }

  .nav-search input {
    width: 100%;
    padding: 8px 14px 8px 36px;
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    border: 1.5px solid transparent;
    font-size: 13px;
    transition: all 0.2s ease;
  }

  .nav-search input:hover {
    border-color: var(--border-hover);
  }

  .nav-search input:focus {
    background: var(--bg-input);
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
  }

  .search-results {
    position: absolute;
    top: calc(100% + 8px);
    left: 0;
    right: 0;
    background: var(--bg-card);
    box-shadow: var(--shadow-lg);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    max-height: 320px;
    overflow-y: auto;
    z-index: 200;
    min-width: 280px;
  }

  .search-item {
    display: flex;
    flex-direction: column;
    padding: 12px 16px;
    transition: background 0.15s;
  }

  .search-item:hover {
    background: var(--bg-surface);
  }

  .search-name {
    font-weight: 500;
    font-size: 14px;
  }

  .search-venue {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 2px;
  }

  .search-more {
    display: block;
    padding: 12px 16px;
    text-align: center;
    font-size: 13px;
    color: var(--accent);
    font-weight: 500;
    border-top: 1px solid var(--border);
    transition: background 0.15s;
  }

  .search-more:hover {
    background: var(--bg-surface);
  }

  .nav-right {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-left: auto;
  }

  .icon-btn {
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.2s ease;
    color: var(--text-secondary);
  }

  .icon-btn:hover {
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  .notify-btn {
    position: relative;
  }

  .notify-btn.has-upcoming::after {
    content: '';
    position: absolute;
    top: 6px;
    right: 6px;
    width: 8px;
    height: 8px;
    background: #ef4444;
    border-radius: 50%;
    border: 2px solid var(--bg-card);
  }

  .icon-btn.active {
    background: var(--accent-bg);
    color: var(--accent);
  }

  .install-btn {
    width: auto;
    padding: 6px 12px;
    gap: 6px;
    background: var(--accent-bg);
    color: var(--accent);
    font-size: 12px;
    font-weight: 500;
  }

  .install-btn:hover {
    background: var(--accent);
    color: #fff;
  }

  .mobile-menu-btn {
    display: none;
  }

  main {
    max-width: 1400px;
    margin: 0 auto;
    padding: 32px;
  }

  @media (max-width: 1024px) {
    main {
      padding: 24px;
    }
  }

  @media (max-width: 768px) {
    .navbar {
      padding: 0 16px;
    }

    .nav-inner {
      gap: 12px;
    }

    .nav-links {
      display: none;
      position: absolute;
      top: 64px;
      left: 0;
      right: 0;
      background: var(--bg-card);
      border-bottom: 1px solid var(--border);
      flex-direction: column;
      padding: 12px;
      gap: 4px;
      box-shadow: var(--shadow-lg);
    }

    .nav-links.open {
      display: flex;
    }

    .nav-links a {
      padding: 12px 16px;
      border-radius: var(--radius-sm);
    }

    .nav-search {
      display: none;
    }

    .mobile-menu-btn {
      display: flex;
      width: 36px;
      height: 36px;
      border-radius: var(--radius-sm);
      align-items: center;
      justify-content: center;
      color: var(--text-secondary);
      transition: all 0.2s;
    }

    .mobile-menu-btn:hover {
      background: var(--bg-surface);
    }

    main {
      padding: 16px;
    }
  }
</style>
