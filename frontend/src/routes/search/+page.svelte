<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let query = $state('');
  let field = $state('');
  let shows = $state([]);
  let loading = $state(false);
  let searched = $state(false);

  onMount(() => {
    const q = $page.url.searchParams.get('q');
    const f = $page.url.searchParams.get('field');
    if (q) {
      query = q;
      if (f) {
        field = f;
        doFieldSearch();
      } else {
        doSearch();
      }
    }
  });

  async function doSearch() {
    const q = query.trim();
    if (!q) { shows = []; searched = false; return; }
    loading = true; searched = true;
    try {
      shows = await api.searchShows(q);
    } catch (e) {
      console.error('Search failed:', e);
    } finally {
      loading = false;
    }
  }

  async function doFieldSearch() {
    if (!query.trim() || !field) return;
    loading = true; searched = true;
    try {
      shows = await api.getByField(field, query.trim());
    } catch (e) {
      console.error('Search failed:', e);
    } finally {
      loading = false;
    }
  }

  function handleSubmit(e) {
    e.preventDefault();
    field = '';
    doSearch();
  }
</script>

<div class="search-page">
  <h1>搜索演出</h1>

  <form class="search-bar" onsubmit={handleSubmit}>
    <div class="search-input-wrapper">
      <svg class="search-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
      <input
        type="text"
        bind:value={query}
        placeholder="输入关键词搜索（支持空格分词，如：茶馆 话剧）"
        autofocus
      />
    </div>
    <button type="submit" class="primary-btn">搜索</button>
  </form>

  <div class="search-hint">
    <span class="hint-label">支持搜索：</span>
    <span class="hint-tag">名称</span>
    <span class="hint-tag">场地</span>
    <span class="hint-tag">剧团</span>
    <span class="hint-tag">阵容</span>
    <span class="hint-tag">同行</span>
    <span class="hint-tag">分类</span>
    <span class="hint-tag">剧目</span>
    <span class="hint-tag">剧评</span>
    <span class="hint-tag">备注</span>
    <span class="hint-tag">座位</span>
  </div>

  {#if loading}
    <div class="loading">
      <div class="spinner"></div>
      <span>搜索中...</span>
    </div>
  {:else if searched && shows.length === 0}
    <div class="empty">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
      <p>未找到匹配的演出</p>
      <p class="empty-hint">试试其他关键词</p>
    </div>
  {:else if searched}
    <div class="results-info">
      找到 <strong>{shows.length}</strong> 场匹配的演出
    </div>
    <div class="results-list">
      {#each shows as show (show.id)}
        <ShowCard {show} />
      {/each}
    </div>
  {:else}
    <div class="placeholder">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
      <p>输入关键词开始搜索</p>
    </div>
  {/if}
</div>

<style>
  .search-page { max-width: 800px; margin: 0 auto; }
  h1 { font-size: 24px; font-weight: 700; margin-bottom: 24px; letter-spacing: -0.02em; }

  .search-bar { display: flex; gap: 10px; margin-bottom: 16px; }

  .search-input-wrapper {
    position: relative;
    flex: 1;
    display: flex;
    align-items: center;
  }

  .search-icon {
    position: absolute;
    left: 14px;
    color: var(--text-muted);
    pointer-events: none;
  }

  .search-input-wrapper input {
    width: 100%;
    padding: 14px 16px 14px 44px;
    font-size: 15px;
    border-radius: var(--radius-md);
    border: 1.5px solid var(--border);
    transition: all 0.2s;
    background: var(--bg-input);
    color: var(--text-primary);
  }

  .search-input-wrapper input:hover {
    border-color: var(--border-hover);
  }

  .search-input-wrapper input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
    outline: none;
  }

  .primary-btn {
    padding: 14px 36px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-md);
    font-size: 15px;
    font-weight: 500;
    transition: all 0.2s;
    white-space: nowrap;
  }

  .primary-btn:hover {
    background: var(--accent-light);
    transform: translateY(-1px);
  }

  .search-hint {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    font-size: 12px;
    margin-bottom: 32px;
    align-items: center;
  }

  .hint-label {
    color: var(--text-muted);
  }

  .hint-tag {
    padding: 4px 10px;
    background: var(--bg-surface);
    border-radius: 20px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .loading, .empty, .placeholder {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-secondary);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .spinner {
    width: 20px;
    height: 20px;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin { to { transform: rotate(360deg); } }

  .empty svg, .placeholder svg {
    opacity: 0.3;
  }

  .empty p, .placeholder p {
    font-size: 15px;
  }

  .empty-hint {
    font-size: 13px;
    color: var(--text-muted) !important;
    margin-top: 4px;
  }

  .placeholder p {
    color: var(--text-muted);
  }

  .results-info {
    margin-bottom: 16px;
    font-size: 14px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .results-info strong {
    color: var(--accent);
  }

  .results-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
</style>
