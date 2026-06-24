<script>
  let { show, compact = false } = $props();

  function formatDateTime(dateStr) {
    const d = new Date(dateStr);
    const h = String(d.getHours()).padStart(2, '0');
    const m = String(d.getMinutes()).padStart(2, '0');
    return `${d.getFullYear()}.${d.getMonth() + 1}.${d.getDate()} ${h}:${m}`;
  }

  function statusLabel(status) {
    return { normal: '正常', cancelled: '已取消', pending_tickets: '待开票', no_show: '未赴约' }[status] || status;
  }

  function statusColor(status) {
    return { normal: '#10b981', cancelled: '#ef4444', pending_tickets: '#f59e0b', no_show: '#94a3b8' }[status] || '#94a3b8';
  }
</script>

<a href="/shows/{show.id}" class="show-card" class:compact>
  <div class="card-body">
    <div class="card-content">
      <div class="card-title-row">
        <span class="status" style="background: {statusColor(show.status)}">{statusLabel(show.status)}</span>
        {#if show.category_name}
          <span class="category">{show.category_name}</span>
        {/if}
        {#if show.rating}
          <span class="rating-badge">
            {#each Array(5) as _, i}
              <span class="star-mini" class:filled={i < show.rating}>★</span>
            {/each}
          </span>
        {/if}
      </div>

      <h3 class="card-title">{show.name}</h3>

      <div class="card-meta">
        <span class="meta-item">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>
          <span>{formatDateTime(show.date)}</span>
        </span>
        {#if show.venue}
          <span class="meta-item">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg>
            <span class="venue-text">{show.venue}</span>
          </span>
        {/if}
      </div>

      {#if !compact}
        <div class="card-extra">
          {#if show.company}
            <span class="extra-item">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 14s1.5 2 4 2 4-2 4-2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
              <span>{show.company}</span>
            </span>
          {/if}
          {#if show.cast}
            <span class="extra-item">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
              <span class="cast-text">{show.cast.replace(/[,，]/g, ' ')}</span>
            </span>
          {/if}
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
    padding: 16px 18px;
    background: var(--bg-card);
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    transition: all 0.2s ease;
    text-decoration: none;
    color: inherit;
  }

  .show-card:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow-md);
    border-color: var(--border-hover);
  }

  .show-card:active {
    transform: translateY(0);
  }

  .show-card.compact {
    padding: 12px 14px;
  }

  .card-body {
    display: flex;
    gap: 14px;
    align-items: flex-start;
  }

  .card-content {
    flex: 1;
    min-width: 0;
  }

  .card-poster {
    width: 76px;
    height: 102px;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
    position: relative;
    background: var(--bg-surface);
  }

  .card-poster img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .card-title-row {
    display: flex;
    gap: 6px;
    margin-bottom: 4px;
    align-items: center;
    flex-wrap: wrap;
  }

  .status {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 4px;
    color: #fff;
    font-weight: 600;
    letter-spacing: 0.02em;
    flex-shrink: 0;
  }

  .category {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 4px;
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-weight: 500;
    flex-shrink: 0;
  }

  .rating-badge {
    display: flex;
    gap: 1px;
    margin-left: auto;
  }

  .star-mini {
    font-size: 12px;
    color: var(--border);
    line-height: 1;
  }

  .star-mini.filled {
    color: var(--warning);
  }

  .card-title {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 8px;
    color: var(--text-primary);
    line-height: 1.3;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    letter-spacing: -0.01em;
  }

  .compact .card-title {
    font-size: 14px;
    margin-bottom: 4px;
  }

  .card-meta {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    font-size: 12px;
    color: var(--text-muted);
  }

  .meta-item {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta-item svg {
    opacity: 0.5;
    flex-shrink: 0;
  }

  .venue-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .card-extra {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--border);
  }

  .extra-item {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .extra-item svg {
    opacity: 0.5;
    flex-shrink: 0;
  }

  .cast-text {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (max-width: 768px) {
    .card-poster {
      width: 64px;
      height: 86px;
    }

    .card-meta {
      gap: 3px 10px;
    }

    .card-extra {
      gap: 3px 10px;
    }
  }

  @media (max-width: 480px) {
    .show-card {
      padding: 12px 14px;
    }

    .card-poster {
      width: 56px;
      height: 76px;
    }

    .card-title {
      font-size: 14px;
    }

    .card-meta {
      font-size: 11px;
    }

    .card-extra {
      display: none;
    }
  }
</style>
