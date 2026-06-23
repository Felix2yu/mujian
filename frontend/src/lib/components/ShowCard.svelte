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
          <span class="meta-icon">📅</span>
          <span>{formatDateTime(show.date)}</span>
        </span>
        {#if show.venue}
          <span class="meta-item">
            <span class="meta-icon">📍</span>
            <span class="venue-text">{show.venue}</span>
          </span>
        {/if}
      </div>

      {#if !compact}
        <div class="card-extra">
          {#if show.company}
            <span class="extra-item">
              <span class="extra-icon">🎭</span>
              <span>{show.company}</span>
            </span>
          {/if}
          {#if show.cast}
            <span class="extra-item">
              <span class="extra-icon">👤</span>
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
    padding: 14px 16px;
    background: var(--bg-card);
    border-radius: 10px;
    border: 1px solid var(--border);
    transition: all 0.2s ease;
    margin-bottom: 8px;
    text-decoration: none;
    color: inherit;
  }

  .show-card:hover {
    box-shadow: 0 4px 16px rgba(0,0,0,0.08);
    border-color: var(--border-hover);
    transform: translateY(-1px);
  }

  .show-card:active {
    transform: translateY(0);
    box-shadow: 0 2px 8px rgba(0,0,0,0.06);
  }

  .show-card.compact {
    padding: 10px 12px;
    margin-bottom: 6px;
  }

  .show-card.compact:last-child {
    margin-bottom: 0;
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
    width: 80px;
    height: 107px;
    flex-shrink: 0;
    border-radius: 6px;
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

  .card-header {
    display: flex;
    gap: 6px;
    margin-bottom: 6px;
    align-items: center;
    flex-wrap: wrap;
  }

  .status {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 10px;
    color: #fff;
    font-weight: 500;
    letter-spacing: 0.3px;
  }

  .category {
    font-size: 11px;
    padding: 2px 8px;
    border-radius: 10px;
    background: var(--bg-surface);
    color: var(--text-secondary);
  }

  .rating-badge {
    display: flex;
    gap: 1px;
    margin-left: auto;
  }

  .star-mini {
    font-size: 12px;
    color: #ddd;
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
    gap: 3px;
    white-space: nowrap;
  }

  .meta-icon {
    font-size: 11px;
  }

  .venue-text {
    max-width: 120px;
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
    margin-top: 6px;
    padding-top: 6px;
    border-top: 1px solid var(--border);
  }

  .extra-item {
    display: flex;
    align-items: center;
    gap: 3px;
    white-space: nowrap;
  }

  .extra-icon {
    font-size: 11px;
  }

  .cast-text {
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  @media (max-width: 768px) {
    .card-poster {
      width: 70px;
      height: 93px;
    }

    .card-meta {
      gap: 3px 10px;
    }

    .venue-text {
      max-width: 100px;
    }

    .card-extra {
      gap: 3px 10px;
    }

    .cast-text {
      max-width: 120px;
    }
  }

  @media (max-width: 480px) {
    .show-card {
      padding: 12px;
    }

    .card-poster {
      width: 60px;
      height: 80px;
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
