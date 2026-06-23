<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let data = null;
  let loading = true;

  onMount(async () => {
    try {
      data = await api.getDashboard();
    } catch (e) {
      console.error('Failed to load dashboard:', e);
    } finally {
      loading = false;
    }
  });

  function statusLabel(s) {
    return { planned: '计划中', watched: '已观看', cancelled: '已取消' }[s] || s;
  }

  function statusColor(s) {
    return { planned: '#4A90D9', watched: '#27AE60', cancelled: '#E74C3C' }[s] || '#999';
  }
</script>

<div class="dashboard">
  <h1>数据看板</h1>

  {#if loading}
    <div class="loading">加载中...</div>
  {:else if data}
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{data.total_shows}</div>
        <div class="stat-label">总演出场次</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{data.total_hours.toFixed(0)}</div>
        <div class="stat-label">总时长 (小时)</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{data.total_venues}</div>
        <div class="stat-label">去过场馆</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">¥{data.total_cost.toFixed(0)}</div>
        <div class="stat-label">总花费</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{data.avg_rating > 0 ? data.avg_rating.toFixed(1) : '-'}</div>
        <div class="stat-label">平均评分</div>
      </div>
    </div>

    <div class="lists-grid">
      <div class="list-card">
        <h3>演出状态</h3>
        <div class="status-list">
          {#each data.by_status as s}
            <div class="status-item">
              <span class="status-dot" style="background: {statusColor(s.status)}"></span>
              <span class="status-name">{statusLabel(s.status)}</span>
              <span class="status-count">{s.count} 场</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="list-card">
        <h3>分类分布</h3>
        {#if data.by_category?.length}
          {#each data.by_category as cat}
            <div class="status-item">
              <span class="status-dot" style="background: {cat.color}"></span>
              <span class="status-name">{cat.name}</span>
              <span class="status-count">{cat.count} 场</span>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无数据</p>
        {/if}
      </div>

      <div class="list-card">
        <h3>高分演出</h3>
        {#if data.top_rated?.length}
          {#each data.top_rated as show}
            <div class="mini-show">
              <a href="/shows/{show.id}">
                <span class="show-name">{show.name}</span>
                <span class="show-rating">{'★'.repeat(show.rating)}</span>
              </a>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无评分记录</p>
        {/if}
      </div>

      <div class="list-card">
        <h3>最近观看</h3>
        {#if data.recent_watched?.length}
          {#each data.recent_watched as show}
            <div class="mini-show">
              <a href="/shows/{show.id}">
                <span class="show-name">{show.name}</span>
                <span class="show-date">{show.date.slice(0, 10)}</span>
              </a>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无观看记录</p>
        {/if}
      </div>

      <div class="list-card">
        <h3>场馆排行</h3>
        {#if data.by_venue?.length}
          {#each data.by_venue as v}
            <div class="status-item">
              <span class="status-name">{v.name}</span>
              <span class="status-count">{v.count} 场</span>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无数据</p>
        {/if}
      </div>

      <div class="list-card">
        <h3>月度统计</h3>
        {#if data.by_month?.length}
          {#each data.by_month as m}
            <div class="status-item">
              <span class="status-name">{m.month.slice(5)}月</span>
              <span class="status-count">{m.count} 场</span>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无数据</p>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .dashboard {
    max-width: 1100px;
    margin: 0 auto;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 24px;
  }

  .loading {
    text-align: center;
    padding: 60px 20px;
    color: #666;
  }

  .stats-grid {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 16px;
    margin-bottom: 24px;
  }

  .stat-card {
    background: #fff;
    border-radius: 12px;
    padding: 20px;
    text-align: center;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .stat-value {
    font-size: 28px;
    font-weight: 700;
    color: #4A90D9;
  }

  .stat-label {
    font-size: 13px;
    color: #666;
    margin-top: 4px;
  }

  .lists-grid {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 20px;
  }

  .list-card {
    background: #fff;
    border-radius: 12px;
    padding: 20px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .list-card h3 {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 16px;
    color: #333;
  }

  .status-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .status-item {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }

  .status-name {
    flex: 1;
    font-size: 14px;
  }

  .status-count {
    font-size: 14px;
    color: #666;
  }

  .mini-show a {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
    border-bottom: 1px solid #f0f0f0;
  }

  .mini-show:last-child a {
    border-bottom: none;
  }

  .mini-show a:hover .show-name {
    color: #4A90D9;
  }

  .show-name {
    font-size: 14px;
    transition: color 0.2s;
  }

  .show-rating {
    color: #f39c12;
    font-size: 12px;
  }

  .show-date {
    font-size: 12px;
    color: #999;
  }

  .empty {
    color: #999;
    font-size: 13px;
    text-align: center;
    padding: 20px;
  }

  @media (max-width: 768px) {
    .dashboard {
      padding: 0;
    }

    h1 {
      font-size: 20px;
      margin-bottom: 16px;
    }

    .stats-grid {
      grid-template-columns: repeat(2, 1fr);
      gap: 10px;
    }

    .stat-card {
      padding: 14px 10px;
    }

    .stat-value {
      font-size: 22px;
    }

    .lists-grid {
      grid-template-columns: 1fr;
      gap: 12px;
    }

    .list-card {
      padding: 16px;
    }
  }

  @media (max-width: 480px) {
    .stats-grid {
      gap: 8px;
    }

    .stat-card {
      padding: 12px 8px;
    }

    .stat-value {
      font-size: 20px;
    }

    .stat-label {
      font-size: 11px;
    }
  }
</style>
