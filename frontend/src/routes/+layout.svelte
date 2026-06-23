<script>
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { theme } from '$lib/stores';
  import { onMount } from 'svelte';

  let stats = null;
  let searchQuery = '';
  let searchResults = [];
  let showSearch = false;
  let mobileMenuOpen = false;

  onMount(async () => {
    try {
      const [statsRes, settingsRes] = await Promise.all([api.getStats(), api.getSettings()]);
      stats = statsRes;
      if (settingsRes.theme) {
        theme.set(settingsRes.theme);
      }
    } catch (e) {
      console.error('Failed to load data:', e);
    }
  });

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

  function closeMobileMenu() {
    mobileMenuOpen = false;
  }

  function toggleMobileMenu() {
    mobileMenuOpen = !mobileMenuOpen;
  }

  $: currentPath = $page.url.pathname;
</script>

<div class="app">
  <nav class="navbar">
    <div class="nav-brand">
      <a href="/" on:click={closeMobileMenu}>幕间</a>
    </div>

    <button class="hamburger" class:open={mobileMenuOpen} on:click={toggleMobileMenu} aria-label="菜单">
      <span></span>
      <span></span>
      <span></span>
    </button>

    <div class="nav-menu" class:open={mobileMenuOpen}>
      <div class="nav-links">
        <a href="/" class:active={currentPath === '/'} on:click={closeMobileMenu}>日历</a>
        <a href="/shows" class:active={currentPath.startsWith('/shows') && !currentPath.includes('/import') && !currentPath.includes('/new')} on:click={closeMobileMenu}>演出列表</a>
        <a href="/dashboard" class:active={currentPath === '/dashboard'} on:click={closeMobileMenu}>看板</a>
        <a href="/shows/new" class:active={currentPath === '/shows/new'} on:click={closeMobileMenu}>添加演出</a>
      </div>
      <div class="nav-search">
        <form on:submit|preventDefault={handleSearch}>
          <input
            type="text"
            placeholder="搜索演出..."
            bind:value={searchQuery}
            on:blur={() => setTimeout(closeSearch, 200)}
          />
        </form>
        {#if showSearch && searchResults.length > 0}
          <div class="search-results">
            {#each searchResults.slice(0, 5) as show}
              <a href="/shows/{show.id}" class="search-item" on:mousedown={closeSearch}>
                <span class="search-name">{show.name}</span>
                <span class="search-venue">{show.venue}</span>
              </a>
            {/each}
            {#if searchResults.length > 5}
              <a href="/search?q={encodeURIComponent(searchQuery)}" class="search-more" on:mousedown={closeSearch}>
                查看全部 {searchResults.length} 条结果 →
              </a>
            {/if}
          </div>
        {/if}
      </div>
      <div class="nav-right-mobile">
        {#if stats}
          <div class="nav-stats">
            <span>{stats.total_shows} 场演出</span>
            <span>{stats.total_hours.toFixed(0)} 小时</span>
          </div>
        {/if}
        <a href="/settings" class:active={currentPath === '/settings'} on:click={closeMobileMenu}>⚙ 设置</a>
      </div>
    </div>

    <div class="nav-right">
      {#if stats}
        <div class="nav-stats">
          <span>{stats.total_shows} 场演出</span>
          <span>{stats.total_hours.toFixed(0)} 小时</span>
        </div>
      {/if}
      <a href="/settings" class="nav-settings" class:active={currentPath === '/settings'}>⚙</a>
    </div>
  </nav>

  <main>
    <slot />
  </main>
</div>

<style>
  :global(*) {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: #f5f5f5;
    color: #333;
    line-height: 1.6;
    transition: background 0.3s, color 0.3s;
    -webkit-text-size-adjust: 100%;
  }

  :global(.dark body),
  :global(body.dark) {
    background: #1a1a1a;
    color: #e0e0e0;
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
  }

  :global(input, select, textarea) {
    font: inherit;
    border: 1px solid #ddd;
    border-radius: 6px;
    padding: 8px 12px;
    transition: border-color 0.2s, background 0.2s, color 0.2s;
  }

  :global(.dark input, .dark select, .dark textarea) {
    background: #2a2a2a;
    border-color: #444;
    color: #e0e0e0;
  }

  :global(input:focus, select:focus, textarea:focus) {
    outline: none;
    border-color: #4A90D9;
  }

  :global(.dark select option) {
    background: #2a2a2a;
    color: #e0e0e0;
  }

  .app {
    min-height: 100vh;
  }

  .navbar {
    background: #fff;
    box-shadow: 0 2px 8px rgba(0,0,0,0.1);
    padding: 0 24px;
    display: flex;
    align-items: center;
    gap: 32px;
    height: 60px;
    position: sticky;
    top: 0;
    z-index: 100;
    transition: background 0.3s;
  }

  :global(.dark) .navbar {
    background: #222;
    box-shadow: 0 2px 8px rgba(0,0,0,0.3);
  }

  .nav-brand a {
    font-size: 24px;
    font-weight: 700;
    color: #4A90D9;
  }

  .hamburger {
    display: none;
    flex-direction: column;
    justify-content: center;
    gap: 5px;
    width: 36px;
    height: 36px;
    padding: 6px;
    z-index: 110;
  }

  .hamburger span {
    display: block;
    width: 100%;
    height: 2px;
    background: #333;
    border-radius: 2px;
    transition: all 0.3s;
  }

  :global(.dark) .hamburger span {
    background: #e0e0e0;
  }

  .hamburger.open span:nth-child(1) {
    transform: rotate(45deg) translate(5px, 5px);
  }

  .hamburger.open span:nth-child(2) {
    opacity: 0;
  }

  .hamburger.open span:nth-child(3) {
    transform: rotate(-45deg) translate(5px, -5px);
  }

  .nav-menu {
    display: contents;
  }

  .nav-links {
    display: flex;
    gap: 8px;
  }

  .nav-links a {
    padding: 8px 16px;
    border-radius: 6px;
    transition: background 0.2s;
    white-space: nowrap;
  }

  .nav-links a:hover {
    background: #f0f0f0;
  }

  :global(.dark) .nav-links a:hover {
    background: #333;
  }

  .nav-links a.active {
    background: #4A90D9;
    color: #fff;
  }

  .nav-search {
    position: relative;
  }

  .nav-search input {
    width: 200px;
    padding: 6px 12px;
    border-radius: 20px;
    background: #f0f0f0;
    border: none;
  }

  :global(.dark) .nav-search input {
    background: #333;
  }

  .nav-search input:focus {
    background: #fff;
    box-shadow: 0 0 0 2px #4A90D9;
  }

  :global(.dark) .nav-search input:focus {
    background: #2a2a2a;
  }

  .search-results {
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    background: #fff;
    box-shadow: 0 4px 12px rgba(0,0,0,0.15);
    border-radius: 8px;
    margin-top: 8px;
    max-height: 300px;
    overflow-y: auto;
    z-index: 200;
    min-width: 250px;
  }

  :global(.dark) .search-results {
    background: #2a2a2a;
  }

  .search-item {
    display: flex;
    flex-direction: column;
    padding: 12px 16px;
    border-bottom: 1px solid #eee;
  }

  :global(.dark) .search-item {
    border-bottom-color: #444;
  }

  .search-item:last-child {
    border-bottom: none;
  }

  .search-item:hover {
    background: #f5f5f5;
  }

  :global(.dark) .search-item:hover {
    background: #333;
  }

  .search-name {
    font-weight: 500;
  }

  .search-venue {
    font-size: 12px;
    color: #666;
  }

  .search-more {
    display: block;
    padding: 12px 16px;
    text-align: center;
    font-size: 13px;
    color: #4A90D9;
    border-top: 1px solid #eee;
  }

  :global(.dark) .search-more {
    border-top-color: #444;
  }

  .search-more:hover {
    background: #f5f5f5;
  }

  :global(.dark) .search-more:hover {
    background: #333;
  }

  .nav-right {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-left: auto;
  }

  .nav-right-mobile {
    display: none;
  }

  .nav-stats {
    display: flex;
    gap: 16px;
    font-size: 13px;
    color: #666;
  }

  .nav-settings {
    font-size: 20px;
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    transition: background 0.2s;
  }

  .nav-settings:hover {
    background: #f0f0f0;
  }

  :global(.dark) .nav-settings:hover {
    background: #333;
  }

  .nav-settings.active {
    background: #4A90D9;
    color: #fff;
  }

  main {
    max-width: 1200px;
    margin: 0 auto;
    padding: 24px;
  }

  :global(.dark main) {
    background: transparent;
  }

  :global(.dark .section),
  :global(.dark .detail-card),
  :global(.dark .show-form),
  :global(.dark .calendar-section),
  :global(.dark .sidebar-section),
  :global(.dark .stats-bar),
  :global(.dark .stat-card),
  :global(.dark .chart-card),
  :global(.dark .list-card) {
    background: #2a2a2a;
    box-shadow: 0 2px 8px rgba(0,0,0,0.3);
  }

  @media (max-width: 768px) {
    .navbar {
      padding: 0 16px;
      gap: 0;
    }

    .hamburger {
      display: flex;
    }

    .nav-menu {
      position: fixed;
      top: 60px;
      left: 0;
      right: 0;
      bottom: 0;
      background: #fff;
      padding: 16px;
      display: flex;
      flex-direction: column;
      gap: 16px;
      transform: translateX(100%);
      transition: transform 0.3s ease;
      z-index: 105;
      overflow-y: auto;
    }

    :global(.dark) .nav-menu {
      background: #1a1a1a;
    }

    .nav-menu.open {
      transform: translateX(0);
    }

    .nav-links {
      flex-direction: column;
      gap: 4px;
    }

    .nav-links a {
      padding: 12px 16px;
      font-size: 16px;
    }

    .nav-search {
      width: 100%;
    }

    .nav-search input {
      width: 100%;
      padding: 10px 16px;
      font-size: 16px;
      border-radius: 8px;
    }

    .search-results {
      position: static;
      min-width: unset;
      margin-top: 8px;
      box-shadow: 0 2px 8px rgba(0,0,0,0.1);
    }

    .nav-right {
      display: none;
    }

    .nav-right-mobile {
      display: flex;
      flex-direction: column;
      gap: 12px;
      padding-top: 16px;
      border-top: 1px solid #eee;
    }

    :global(.dark) .nav-right-mobile {
      border-top-color: #444;
    }

    .nav-right-mobile .nav-stats {
      justify-content: center;
    }

    .nav-right-mobile a {
      padding: 12px 16px;
      border-radius: 8px;
      background: #f0f0f0;
      text-align: center;
      font-weight: 500;
    }

    :global(.dark) .nav-right-mobile a {
      background: #2a2a2a;
    }

    main {
      padding: 16px;
    }
  }
</style>
