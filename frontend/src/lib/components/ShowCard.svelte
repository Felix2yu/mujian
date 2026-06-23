<script>
  export let show;
  export let compact = false;

  function formatDate(dateStr) {
    const d = new Date(dateStr);
    return `${d.getMonth() + 1}月${d.getDate()}日`;
  }

  function formatDateTime(dateStr) {
    const d = new Date(dateStr);
    const h = String(d.getHours()).padStart(2, '0');
    const m = String(d.getMinutes()).padStart(2, '0');
    return `${d.getMonth() + 1}月${d.getDate()}日 ${h}:${m}`;
  }

  function statusLabel(status) {
    const labels = { planned: '计划中', watched: '已观看', cancelled: '已取消' };
    return labels[status] || status;
  }

  function statusColor(status) {
    const colors = { planned: '#4A90D9', watched: '#27AE60', cancelled: '#E74C3C' };
    return colors[status] || '#999';
  }
</script>

<a href="/shows/{show.id}" class="show-card" class:compact>
  <div class="card-header">
    <span class="status" style="background: {statusColor(show.status)}">{statusLabel(show.status)}</span>
    {#if show.category_name}
      <span class="category">{show.category_name}</span>
    {/if}
  </div>

  <h3 class="card-title">{show.name}</h3>

  <div class="card-meta">
    <span class="date">{formatDateTime(show.date)}</span>
    {#if show.venue}
      <span class="venue">📍 {show.venue}</span>
    {/if}
  </div>

  {#if show.rating}
    <div class="rating">
      {#each Array(5) as _, i}
        <span class="star" class:filled={i < show.rating}>★</span>
      {/each}
    </div>
  {/if}

  {#if show.company}
    <div class="company">{show.company}</div>
  {/if}
</a>

<style>
  .show-card {
    display: block;
    padding: 16px;
    background: #fff;
    border-radius: 8px;
    border: 1px solid #eee;
    transition: box-shadow 0.2s, border-color 0.2s;
    margin-bottom: 8px;
  }

  .show-card:hover {
    box-shadow: 0 4px 12px rgba(0,0,0,0.1);
    border-color: #ddd;
  }

  .show-card.compact {
    padding: 12px;
    margin-bottom: 8px;
  }

  .show-card.compact:last-child {
    margin-bottom: 0;
  }

  .card-header {
    display: flex;
    gap: 8px;
    margin-bottom: 8px;
  }

  .status {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 10px;
    color: #fff;
    font-weight: 500;
  }

  .category {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 10px;
    background: #f0f0f0;
    color: #666;
  }

  .card-title {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 8px;
    color: #333;
  }

  .compact .card-title {
    font-size: 14px;
    margin-bottom: 4px;
  }

  .card-meta {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 13px;
    color: #666;
  }

  .rating {
    margin-top: 8px;
  }

  .star {
    color: #ddd;
    font-size: 14px;
  }

  .star.filled {
    color: #f39c12;
  }

  .company {
    margin-top: 8px;
    font-size: 13px;
    color: #666;
  }
</style>
