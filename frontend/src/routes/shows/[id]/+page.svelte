<script>
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let show = null;
  let loading = true;
  let error = '';

  $: id = $page.params.id;

  onMount(async () => {
    try {
      show = await api.getShow(id);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function formatDate(dateStr) {
    const d = new Date(dateStr);
    return d.toLocaleDateString('zh-CN', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' });
  }

  function formatTime(dateStr) {
    const d = new Date(dateStr);
    return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
  }

  function formatDuration(mins) {
    if (!mins) return '-';
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (h === 0) return `${m}分钟`;
    if (m === 0) return `${h}小时`;
    return `${h}小时${m}分钟`;
  }

  function statusLabel(status) {
    const labels = { planned: '计划中', watched: '已观看', cancelled: '已取消' };
    return labels[status] || status;
  }

  function statusColor(status) {
    const colors = { planned: '#4A90D9', watched: '#27AE60', cancelled: '#E74C3C' };
    return colors[status] || '#999';
  }

  function formatCost(val) {
    if (val == null) return '-';
    return `¥${val.toFixed(2)}`;
  }

  async function deleteShow() {
    if (!confirm('确定删除这场演出吗？')) return;
    try {
      await api.deleteShow(id);
      goto('/shows');
    } catch (e) {
      alert('删除失败: ' + e.message);
    }
  }

  $: totalCost = (show?.ticket_cost || 0) + (show?.other_cost || 0);
</script>

<div class="show-detail">
  {#if loading}
    <div class="loading">加载中...</div>
  {:else if error}
    <div class="error">{error}</div>
  {:else if show}
    {#if show.poster_url}
      <div class="poster">
        <img src={show.poster_url} alt={show.name} />
      </div>
    {/if}

    <div class="detail-card">
      <div class="detail-header">
        <div class="header-info">
          <h1>{show.name}</h1>
          <div class="meta-row">
            <span class="status" style="background: {statusColor(show.status)}">{statusLabel(show.status)}</span>
            {#if show.category_name}
              <span class="category">{show.category_name}</span>
            {/if}
            {#if show.rating}
              <span class="rating">
                {#each Array(5) as _, i}
                  <span class:filled={i < show.rating}>★</span>
                {/each}
              </span>
            {/if}
          </div>
        </div>
        <div class="header-actions">
          <a href="/shows/{show.id}/edit" class="edit-btn">编辑</a>
          <button class="delete-btn" on:click={deleteShow}>删除</button>
        </div>
      </div>

      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">时间</span>
          <span class="info-value">{formatDate(show.date)} {formatTime(show.date)}</span>
        </div>

        {#if show.venue}
          <div class="info-item">
            <span class="info-label">场地</span>
            <span class="info-value">{show.venue}</span>
          </div>
        {/if}

        {#if show.duration}
          <div class="info-item">
            <span class="info-label">时长</span>
            <span class="info-value">{formatDuration(show.duration)}</span>
          </div>
        {/if}

        {#if show.seat}
          <div class="info-item">
            <span class="info-label">座位</span>
            <span class="info-value">{show.seat}</span>
          </div>
        {/if}

        {#if show.company}
          <div class="info-item">
            <span class="info-label">剧团</span>
            <span class="info-value">{show.company}</span>
          </div>
        {/if}

        {#if show.cast}
          <div class="info-item">
            <span class="info-label">阵容</span>
            <span class="info-value">{show.cast}</span>
          </div>
        {/if}

        {#if show.friends}
          <div class="info-item">
            <span class="info-label">同行</span>
            <span class="info-value">{show.friends}</span>
          </div>
        {/if}

        {#if show.ticket_cost != null || show.other_cost != null}
          <div class="info-item">
            <span class="info-label">花费</span>
            <span class="info-value">
              {#if show.ticket_cost != null}门票 {formatCost(show.ticket_cost)}{/if}
              {#if show.other_cost != null} 其他 {formatCost(show.other_cost)}{/if}
              {#if totalCost > 0}<strong>合计 {formatCost(totalCost)}</strong>{/if}
            </span>
          </div>
        {/if}
      </div>

      {#if show.setlist}
        <div class="section">
          <h3>剧目</h3>
          <div class="text-content">{show.setlist}</div>
        </div>
      {/if}

      {#if show.review}
        <div class="section">
          <h3>剧评</h3>
          <div class="text-content">{show.review}</div>
        </div>
      {/if}

      {#if show.notes}
        <div class="section">
          <h3>备注</h3>
          <div class="text-content">{show.notes}</div>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .show-detail {
    max-width: 800px;
    margin: 0 auto;
  }

  .loading, .error {
    text-align: center;
    padding: 60px 20px;
    color: #666;
  }

  .error {
    color: #c00;
    background: #fee;
    border-radius: 8px;
  }

  .poster {
    margin-bottom: 24px;
    border-radius: 12px;
    overflow: hidden;
    box-shadow: 0 4px 16px rgba(0,0,0,0.15);
  }

  .poster img {
    width: 100%;
    display: block;
  }

  .detail-card {
    background: #fff;
    border-radius: 12px;
    padding: 32px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .detail-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 24px;
  }

  .header-info h1 {
    font-size: 28px;
    font-weight: 700;
    margin-bottom: 12px;
  }

  .meta-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .status {
    font-size: 12px;
    padding: 4px 12px;
    border-radius: 12px;
    color: #fff;
    font-weight: 500;
  }

  .category {
    font-size: 12px;
    padding: 4px 12px;
    border-radius: 12px;
    background: #f0f0f0;
    color: #666;
  }

  .rating {
    font-size: 16px;
    color: #ddd;
  }

  .rating .filled {
    color: #f39c12;
  }

  .header-actions {
    display: flex;
    gap: 8px;
  }

  .edit-btn, .delete-btn {
    padding: 8px 16px;
    border-radius: 8px;
    font-size: 14px;
  }

  .edit-btn {
    background: #f0f0f0;
    color: #333;
  }

  .edit-btn:hover {
    background: #e0e0e0;
  }

  .delete-btn {
    background: #fee;
    color: #c00;
  }

  .delete-btn:hover {
    background: #fdd;
  }

  .info-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
    margin-bottom: 24px;
    padding-bottom: 24px;
    border-bottom: 1px solid #eee;
  }

  .info-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .info-label {
    font-size: 12px;
    color: #999;
    text-transform: uppercase;
  }

  .info-value {
    font-size: 15px;
    color: #333;
  }

  .section {
    margin-bottom: 24px;
  }

  .section h3 {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 12px;
    color: #333;
  }

  .text-content {
    font-size: 15px;
    line-height: 1.8;
    color: #555;
    white-space: pre-wrap;
  }

  @media (max-width: 600px) {
    .detail-card {
      padding: 20px;
    }

    .info-grid {
      grid-template-columns: 1fr;
    }

    .detail-header {
      flex-direction: column;
      gap: 16px;
    }
  }
</style>
