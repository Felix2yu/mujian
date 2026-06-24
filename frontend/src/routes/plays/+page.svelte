<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let plays = $state([]);
  let loading = $state(true);

  onMount(async () => {
    try {
      plays = await api.listPlays();
    } catch (e) {
      console.error('Failed to load plays:', e);
    } finally {
      loading = false;
    }
  });
</script>

<div class="plays-page">
  <div class="page-header">
    <div class="header-left">
      <h1>剧目</h1>
      <span class="count-badge">{plays.length}</span>
    </div>
  </div>

  {#if loading}
    <div class="loading"><div class="spinner"></div>加载中...</div>
  {:else if plays.length === 0}
    <div class="empty">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
      <p>暂无剧目数据</p>
      <p class="empty-hint">在演出中添加剧目信息后，剧目将自动出现在这里</p>
    </div>
  {:else}
    <div class="results-info">共 <strong>{plays.length}</strong> 部剧目</div>
    <div class="play-list">
      <div class="list-header">
        <span class="col-rank">#</span>
        <span class="col-name">剧名</span>
        <span class="col-scenes">折子</span>
        <span class="col-count">演出场次</span>
      </div>
      {#each plays as play, i}
        <a href="/plays/{encodeURIComponent(play.name)}" class="list-row">
          <span class="col-rank">{i + 1}</span>
          <span class="col-name">{play.name}</span>
          <span class="col-scenes">{play.scene_count > 0 ? play.scene_count + ' 折' : '-'}</span>
          <span class="col-count">{play.show_count} 场</span>
        </a>
      {/each}
    </div>
  {/if}
</div>

<style>
  .plays-page { display: flex; flex-direction: column; gap: 16px; }
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

  .results-info {
    margin-bottom: 16px;
    font-size: 14px;
    color: var(--text-secondary);
    font-weight: 500;
  }

  .results-info strong { color: var(--accent); }

  .play-list {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
  }

  .list-header {
    display: grid;
    grid-template-columns: 40px 1fr 80px 100px;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-bottom: 1px solid var(--border);
    background: var(--bg-surface);
  }

  .list-row {
    display: grid;
    grid-template-columns: 40px 1fr 80px 100px;
    align-items: center;
    gap: 12px;
    padding: 14px 16px;
    text-decoration: none;
    color: inherit;
    transition: background 0.15s;
    border-bottom: 1px solid var(--border);
  }

  .list-row:last-child {
    border-bottom: none;
  }

  .list-row:hover {
    background: var(--bg-card-hover);
  }

  .col-rank {
    font-size: 13px;
    font-weight: 700;
    color: var(--text-muted);
    text-align: center;
  }

  .list-row:hover .col-rank {
    color: var(--accent);
  }

  .col-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .col-scenes {
    font-size: 13px;
    color: var(--accent);
    font-weight: 500;
    text-align: center;
  }

  .col-count {
    font-size: 13px;
    color: var(--text-secondary);
    text-align: right;
    font-weight: 500;
  }

  @media (max-width: 480px) {
    .list-header {
      grid-template-columns: 32px 1fr 60px 70px;
      padding: 10px 12px;
      gap: 8px;
    }

    .list-row {
      grid-template-columns: 32px 1fr 60px 70px;
      padding: 12px;
      gap: 8px;
    }
  }
</style>
