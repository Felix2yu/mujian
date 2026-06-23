<script>
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import { theme } from '$lib/stores';
  import { onMount } from 'svelte';

  let stats = null;
  let searchQuery = '';
  let searchResults = [];
  let showSearch = false;

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

  $: currentPath = $page.url.pathname;
</script>

<div class="app">
  <nav class="navbar">
    <div class="nav-brand">
      <a href="/">幕间</a>
    </div>
    <div class="nav-links">
      <a href="/" class:active={currentPath === '/'}>日历</a>
      <a href="/shows" class:active={currentPath.startsWith('/shows')}>演出列表</a>
      <a href="/shows/new" class:active={currentPath === '/shows/new'}>添加演出</a>
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

  .nav-links {
    display: flex;
    gap: 8px;
  }

  .nav-links a {
    padding: 8px 16px;
    border-radius: 6px;
    transition: background 0.2s;
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
  :global(.dark .stats-bar) {
    background: #2a2a2a;
    box-shadow: 0 2px 8px rgba(0,0,0,0.3);
  }
</style>
