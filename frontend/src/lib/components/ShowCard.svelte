<script>
  export let show;
  export let compact = false;

  function formatDateTime(dateStr) {
    const d = new Date(dateStr);
    const h = String(d.getHours()).padStart(2, '0');
    const m = String(d.getMinutes()).padStart(2, '0');
    return `${d.getFullYear()}.${d.getMonth() + 1}.${d.getDate()} ${h}:${m}`;
  }

  function statusLabel(status) {
    return { planned: '计划中', watched: '已观看', cancelled: '已取消' }[status] || status;
  }

  function statusColor(status) {
    return { planned: '#4A90D9', watched: '#27AE60', cancelled: '#E74C3C' }[status] || '#999';
  }
</script>

<a href="/shows/{show.id}" class="show-card" class:compact>
  <div class="card-body">
    <div class="card-content">
      <div class="card-header">
        <span class="status" style="background: {statusColor(show.status)}">{statusLabel(show.status)}</span>
        {#if show.category_name}
          <span class="category">{show.category_name}</span>
        {/if}
      </div>

      <h3 class="card-title">{show.name}</h3>

      <div class="card-info">
        <span class="date">📅 {formatDateTime(show.date)}</span>
        {#if show.venue}<span class="venue">📍 {show.venue}</span>{/if}
        {#if show.company}<span class="company">🎭 {show.company}</span>{/if}
        {#if show.cast}<span class="cast">👤 {show.cast.replace(/[,，]/g, ' ')}</span>{/if}
      </div>

      {#if show.rating}
        <div class="rating">
          {#each Array(5) as _, i}
            <span class="star" class:filled={i < show.rating}>★</span>
          {/each}
        </div>
      {/if}
    </div>

    {#if show.poster_url && !compact}
      <div class="card-poster">
        <img src={show.poster_url} alt={show.name} />
      </div>
    {/if}
  </div>
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
  .show-card:hover { box-shadow: 0 4px 12px rgba(0,0,0,0.1); border-color: #ddd; }
  .show-card.compact { padding: 12px; margin-bottom: 8px; }
  .show-card.compact:last-child { margin-bottom: 0; }
  .card-body { display: flex; gap: 16px; }
  .card-content { flex: 1; min-width: 0; }
  .card-poster { max-width: 120px; flex-shrink: 0; border-radius: 6px; overflow: hidden; align-self: center; }
  .card-poster img { width: 100%; display: block; border-radius: 6px; }
  .card-header { display: flex; gap: 8px; margin-bottom: 8px; }
  .status { font-size: 11px; padding: 2px 8px; border-radius: 10px; color: #fff; font-weight: 500; }
  .category { font-size: 11px; padding: 2px 8px; border-radius: 10px; background: #f0f0f0; color: #666; }
  .card-title { font-size: 16px; font-weight: 600; margin-bottom: 8px; color: #333; }
  .compact .card-title { font-size: 14px; margin-bottom: 4px; }
  .card-info { display: flex; flex-wrap: wrap; gap: 4px 14px; font-size: 13px; color: #666; }
  .card-info span { white-space: nowrap; }
  .rating { margin-top: 6px; }
  .star { color: #ddd; font-size: 14px; }
  .star.filled { color: #f39c12; }

  :global(.dark) .show-card {
    background: #2a2a2a;
    border-color: #333;
  }

  :global(.dark) .show-card:hover {
    border-color: #444;
  }

  :global(.dark) .category {
    background: #333;
    color: #999;
  }

  :global(.dark) .card-title {
    color: #e0e0e0;
  }

  :global(.dark) .card-info {
    color: #999;
  }

  :global(.dark) .star {
    color: #555;
  }
</style>
