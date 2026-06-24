<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let actors = $state([]);
  let loading = $state(true);
  let searchQuery = $state('');

  let filteredActors = $derived(
    searchQuery
      ? actors.filter(a => a.name.toLowerCase().includes(searchQuery.toLowerCase()))
      : actors
  );

  onMount(async () => {
    try {
      actors = await api.listActors();
    } catch (e) {
      console.error('Failed to load actors:', e);
    } finally {
      loading = false;
    }
  });
</script>

<div class="cast-page">
  <div class="page-header">
    <div class="header-left">
      <h1>演员</h1>
      <span class="count-badge">{actors.length}</span>
    </div>
  </div>

  {#if loading}
    <div class="loading"><div class="spinner"></div>加载中...</div>
  {:else if actors.length === 0}
    <div class="empty">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
      <p>暂无演员数据</p>
      <p class="empty-hint">在演出中添加阵容信息后，演员将自动出现在这里</p>
    </div>
  {:else}
    <div class="toolbar">
      <div class="search-wrapper">
        <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
        <input type="text" placeholder="搜索演员..." bind:value={searchQuery} />
      </div>
    </div>

    {#if filteredActors.length === 0}
      <div class="empty">
        <p>未找到匹配的演员</p>
        <p class="empty-hint">试试其他关键词</p>
      </div>
    {:else}
      <div class="actor-grid">
        {#each filteredActors as actor}
          <a href="/cast/{encodeURIComponent(actor.name)}" class="actor-card">
            <div class="actor-avatar">
              {#if actor.photo_url}
                <img src={actor.photo_url} alt={actor.name} />
              {:else}
                <span>{actor.name.charAt(0)}</span>
              {/if}
            </div>
            <div class="actor-info">
              <span class="actor-name">{actor.name}</span>
              <span class="actor-count">{actor.show_count} 场演出</span>
            </div>
          </a>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .cast-page { display: flex; flex-direction: column; gap: 16px; }
  .page-header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; }
  .header-left { display: flex; align-items: center; gap: 10px; }
  .header-left h1 { font-size: 24px; font-weight: 700; letter-spacing: -0.02em; }
  .count-badge {
    font-size: 12px;
    font-weight: 600;
    padding: 3px 10px;
    border-radius: 20px;
    background: var(--accent-bg);
    color: var(--accent);
  }

  .loading {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    justify-content: center;
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

  .empty {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-secondary);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .empty svg { opacity: 0.3; }
  .empty p { font-size: 15px; }
  .empty-hint { font-size: 13px; color: var(--text-muted) !important; margin-top: 4px; }

  .toolbar {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-bottom: 20px;
  }

  .search-wrapper {
    position: relative;
    display: flex;
    align-items: center;
    flex: 1;
    max-width: 320px;
  }

  .search-icon {
    position: absolute;
    left: 12px;
    color: var(--text-muted);
    pointer-events: none;
  }

  .search-wrapper input {
    width: 100%;
    padding: 10px 14px 10px 36px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    border: 1px solid var(--border);
    background: var(--bg-input);
    color: var(--text-primary);
    transition: all 0.2s;
  }

  .search-wrapper input:hover {
    border-color: var(--border-hover);
  }

  .search-wrapper input:focus {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-bg);
    outline: none;
  }

  .actor-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
    gap: 12px;
  }

  .actor-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 24px 16px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    text-decoration: none;
    color: inherit;
    transition: all 0.2s;
    text-align: center;
  }

  .actor-card:hover {
    border-color: var(--accent);
    box-shadow: var(--shadow-md);
    transform: translateY(-2px);
  }

  .actor-avatar {
    width: 64px;
    height: 64px;
    border-radius: 50%;
    overflow: hidden;
    flex-shrink: 0;
  }

  .actor-avatar img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .actor-avatar span {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--accent-bg);
    color: var(--accent);
    font-size: 24px;
    font-weight: 700;
  }

  .actor-info {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .actor-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .actor-count {
    font-size: 12px;
    color: var(--text-muted);
  }

  @media (max-width: 480px) {
    .actor-grid {
      grid-template-columns: repeat(2, 1fr);
      gap: 10px;
    }

    .actor-card {
      padding: 20px 12px;
    }

    .actor-avatar {
      width: 56px;
      height: 56px;
    }

    .actor-avatar span {
      font-size: 20px;
    }
  }
</style>
