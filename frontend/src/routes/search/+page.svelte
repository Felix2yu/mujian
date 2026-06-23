<script>
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let query = '';
  let field = '';
  let shows = [];
  let loading = false;
  let searched = false;

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

  <form class="search-bar" on:submit={handleSubmit}>
    <input
      type="text"
      bind:value={query}
      placeholder="输入关键词搜索（支持空格分词，如：茶馆 话剧）"
      autofocus
    />
    <button type="submit" class="btn-search">搜索</button>
  </form>

  <div class="search-hint">
    <span>支持搜索：</span>
    <span>名称</span><span>场地</span><span>剧团</span><span>阵容</span>
    <span>同行</span><span>分类</span><span>剧目</span><span>剧评</span><span>备注</span><span>座位</span>
  </div>

  {#if loading}
    <div class="loading">
      <div class="spinner"></div>
      <span>搜索中...</span>
    </div>
  {:else if searched && shows.length === 0}
    <div class="empty">
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
      <p>输入关键词开始搜索</p>
    </div>
  {/if}
</div>

<style>
  .search-page { max-width: 800px; margin: 0 auto; }
  h1 { font-size: 24px; font-weight: 700; margin-bottom: 24px; }
  .search-bar { display: flex; gap: 8px; margin-bottom: 12px; }
  .search-bar input { flex: 1; padding: 12px 16px; font-size: 16px; border-radius: 8px; border: 2px solid #ddd; transition: border-color 0.2s; }
  .search-bar input:focus { border-color: #4A90D9; outline: none; }
  .btn-search { padding: 12px 32px; background: #4A90D9; color: #fff; border-radius: 8px; font-size: 16px; font-weight: 500; transition: background 0.2s; }
  .btn-search:hover { background: #3a7bc8; }
  .search-hint { display: flex; flex-wrap: wrap; gap: 8px; font-size: 12px; color: #999; margin-bottom: 32px; }
  .search-hint span:not(:first-child) { padding: 2px 8px; background: #f0f0f0; border-radius: 4px; }
  .loading, .empty, .placeholder { text-align: center; padding: 60px 20px; color: #666; }
  .loading { display: flex; align-items: center; justify-content: center; gap: 12px; }
  .spinner {
    width: 24px; height: 24px; border: 3px solid #ddd; border-top-color: #4A90D9;
    border-radius: 50%; animation: spin 0.8s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  .empty-hint { font-size: 13px; color: #999; margin-top: 8px; }
  .placeholder p { color: #999; }
  .results-info { margin-bottom: 16px; font-size: 14px; color: #666; }
  .results-list { display: flex; flex-direction: column; gap: 12px; }
</style>
