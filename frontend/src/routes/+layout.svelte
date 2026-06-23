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
      <a href="/shows" class:active={currentPath.startsWith('/shows') && !currentPath.includes('/import') && !currentPath.includes('/new')}>演出列表</a>
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
  :global(:root) {
    --bg-body: #f5f5f5;
    --bg-card: #fff;
    --bg-card-hover: #fafafa;
    --bg-surface: #f0f0f0;
    --bg-surface-hover: #e0e0e0;
    --bg-input: #fff;
    --border: #eee;
    --border-hover: #ddd;
    --text-primary: #333;
    --text-secondary: #666;
    --text-muted: #999;
    --accent: #4A90D9;
    --danger-bg: #fee;
    --danger-text: #c00;
    --danger-bg-hover: #fdd;
    --success: #27AE60;
    --warning: #f39c12;
  }

  :global(.dark) {
    --bg-body: #1a1a1a;
    --bg-card: #2a2a2a;
    --bg-card-hover: #333;
    --bg-surface: #333;
    --bg-surface-hover: #444;
    --bg-input: #2a2a2a;
    --border: #333;
    --border-hover: #444;
    --text-primary: #e0e0e0;
    --text-secondary: #aaa;
    --text-muted: #777;
    --danger-bg: #3a2020;
    --danger-text: #f66;
    --danger-bg-hover: #4a2020;
  }

  :global(*) {
    margin: 0;
    padding: 0;
    box-sizing: border-box;
  }

  :global(body) {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    background: var(--bg-body);
    color: var(--text-primary);
    line-height: 1.6;
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
  }

  :global(input, select, textarea) {
    font: inherit;
    border-radius: 6px;
    padding: 8px 12px;
    background: var(--bg-input);
    color: var(--text-primary);
    transition: background 0.2s, color 0.2s;
  }

  :global(input:focus, select:focus, textarea:focus) {
    outline: none;
    outline: 2px solid var(--accent);
    outline-offset: -1px;
  }

  :global(.dark select option) {
    background: var(--bg-input);
    color: var(--text-primary);
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
  }

  :global(.dark) .search-item {
    color: var(--text-primary);
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
  }

  :global(.dark) .search-more {
    color: #4A90D9;
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
  :global(.dark .show-form),
  :global(.dark .calendar-section),
  :global(.dark .sidebar-section),
  :global(.dark .stats-bar),
  :global(.dark .stat-card),
  :global(.dark .chart-card),
  :global(.dark .list-card),
  :global(.dark .batch-bar),
  :global(.dark .batch-panel),
  :global(.dark .select-all),
  :global(.dark .card),
  :global(.dark .s3-form) {
    background: var(--bg-card);
  }

  :global(.dark .tabs),
  :global(.dark .calendar-grid) {
    background: #1e1e1e;
  }

  :global(.dark .tab.active) {
    background: #2a2a2a;
    color: #e0e0e0;
    box-shadow: 0 1px 3px rgba(0,0,0,0.3);
  }

  :global(.dark .action-btn),
  :global(.dark .batch-btn),
  :global(.dark .batch-action),
  :global(.dark .edit-btn),
  :global(.dark .btn-restore),
  :global(.dark .btn-import-more),
  :global(.dark .filter-toggle),
  :global(.dark .nav-settings) {
    background: #333;
    color: #ccc;
  }

  :global(.dark .action-btn:hover),
  :global(.dark .batch-btn:hover),
  :global(.dark .batch-action:hover:not(:disabled)),
  :global(.dark .edit-btn:hover),
  :global(.dark .btn-restore:hover),
  :global(.dark .btn-import-more:hover),
  :global(.dark .filter-toggle:hover) {
    background: #444;
  }

  :global(.dark .batch-action.danger),
  :global(.dark .delete-btn),
  :global(.dark .clear-btn) {
    background: #3a2020;
    color: #f66;
  }

  :global(.dark .batch-action.danger:hover),
  :global(.dark .delete-btn:hover),
  :global(.dark .clear-btn:hover) {
    background: #4a2020;
  }

  :global(.dark .category) {
    background: #333;
    color: #999;
  }

  :global(.dark .category:hover) {
    background: #444;
  }

  :global(.dark .info-label),
  :global(.dark .form-section label) {
    color: #999;
  }

  :global(.dark .info-value),
  :global(.dark h1),
  :global(.dark h2),
  :global(.dark h3) {
    color: #e0e0e0;
  }

  :global(.dark .text-content),
  :global(.dark .card-info) {
    color: #aaa;
  }

  :global(.dark .error) {
    background: #3a2020;
    color: #f66;
  }

  :global(.dark .empty),
  :global(.dark .loading),
  :global(.dark .result-count),
  :global(.dark .restore-status) {
    color: #999;
  }

  :global(.dark .spinner) {
    border-color: #444;
    border-top-color: #4A90D9;
  }

  :global(.dark .show-card) {
    background: #2a2a2a;
    border-color: #333;
  }

  :global(.dark .show-card:hover) {
    border-color: #444;
  }

  :global(.dark .tag) {
    background: #1a3a5a;
    color: #ccc;
  }

  :global(.dark .tag-remove) {
    color: #777;
  }

  :global(.dark .tag-remove:hover) {
    color: #f66;
  }

  :global(.dark .tag-field) {
    color: #e0e0e0;
  }

  :global(.dark .dropzone) {
    border-color: #444;
  }

  :global(.dark .dropzone:hover),
  :global(.dark .dropzone.dragover) {
    background: #1a2a3a;
  }

  :global(.dark .col-group strong) {
    color: #e0e0e0;
  }

  :global(.dark .col-group li) {
    color: #999;
  }

  :global(.dark .instructions p) {
    color: #999;
  }

  :global(.dark .result-stats) {
    color: #999;
  }

  :global(.dark .nav-links a:hover) {
    background: #333;
  }

  :global(.dark .nav-search input) {
    background: #333;
    color: #e0e0e0;
  }

  :global(.dark .nav-search input:focus) {
    background: #2a2a2a;
  }

  :global(.dark .search-results) {
    background: #2a2a2a;
    border-color: #333;
  }

  :global(.dark .search-item) {
    border-bottom-color: #333;
    color: #e0e0e0;
  }

  :global(.dark .search-item:hover) {
    background: #333;
  }

  :global(.dark .search-item .search-venue) {
    color: #999;
  }

  :global(.dark .search-more) {
    border-top-color: #333;
    color: #4A90D9;
  }

  :global(.dark .search-more:hover) {
    background: #333;
  }

  :global(.dark .nav-settings:hover) {
    background: #333;
  }

  :global(.dark .nav-settings.active) {
    background: #4A90D9;
    color: #fff;
  }

  :global(.dark .day-cell.empty) {
    background: #1a1a1a;
  }

  :global(.dark .day-cell:not(.empty):hover) {
    background: #2a2a2a;
  }

  :global(.dark .poster-cell) {
    background: #333;
  }

  :global(.dark .popup-item:hover) {
    background: #333;
  }

  :global(.dark .popup-item) {
    border-bottom-color: #333;
  }

  :global(.dark .popup-list) {
    border-top: 1px solid #333;
  }

  @media (max-width: 768px) {
    .navbar {
      padding: 0 12px;
      gap: 12px;
    }

    .nav-links {
      gap: 4px;
    }

    .nav-links a {
      padding: 6px 10px;
      font-size: 13px;
    }

    .nav-search input {
      width: 120px;
      font-size: 13px;
    }

    .nav-stats {
      display: none;
    }

    main {
      padding: 16px;
    }
  }

  @media (max-width: 480px) {
    .nav-links a {
      padding: 6px 8px;
      font-size: 12px;
    }

    .nav-search input {
      width: 100px;
    }
  }
</style>
