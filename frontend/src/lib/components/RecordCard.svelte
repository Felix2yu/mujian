<script>
  import { coverUrl, formatCurrency } from '$lib/api.js';
  let { record } = $props();
  let coverFailed = $state(false);

  function stars(n) {
    return '★'.repeat(n) + '☆'.repeat(Math.max(0, 5 - n));
  }
</script>

<a class="card card-hover rec" href={`/records/${record.id}`}>
  <div class="cover">
    {#if record.coverThumb && !coverFailed}
      <img src={coverUrl(record.coverThumb)} alt={record.name} loading="lazy" onerror={() => (coverFailed = true)} />
    {:else if record.coverFile && !coverFailed}
      <img src={coverUrl(record.coverFile)} alt={record.name} loading="lazy" onerror={() => (coverFailed = true)} />
    {:else}
      <div class="no-cover"><span>{(record.name || '?').slice(0, 1)}</span></div>
    {/if}
    {#if record.categoryName}<span class="cat-badge">{record.categoryName}</span>{/if}
    {#if record.rating}
      <span class="rate-badge">★ {record.rating}</span>
    {/if}
  </div>
  <div class="info">
    <div class="title" title={record.name}>{record.name}</div>
    <div class="meta">
      {#if record.dateText}<span>{record.dateText.split(' ')[0]}</span>{/if}
      {#if record.city}<span class="dot">·</span><span>{record.city}</span>{/if}
    </div>
    {#if record.artist_names && record.artist_names.length}
      <div class="artists">
        {#each record.artist_names.slice(0, 3) as name, i}
          <a class="artist-link" href={`/artists/${record.artist_ids?.[i] || ''}`} onclick={(e) => { if (!record.artist_ids?.[i]) e.preventDefault(); }}>{name}</a>{i < Math.min(record.artist_names.length, 3) - 1 ? ' / ' : ''}
        {/each}{record.artist_names.length > 3 ? ' 等' : ''}
      </div>
    {/if}
    <div class="bottom">
      {#if record.address}<span class="tag venue" title={record.address}>{record.address}</span>{/if}
      {#if record.price}<span class="price">{formatCurrency(record.price, record.price_currency)}</span>{/if}
    </div>
  </div>
</a>

<style>
  .rec { display: flex; flex-direction: column; overflow: hidden; }
  .cover {
    position: relative;
    aspect-ratio: 3 / 4;
    background: var(--surface-3);
    overflow: hidden;
    min-width: 0;
  }
  .cover img {
    width: 100%;
    height: 100%;
    max-width: 100%;
    max-height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.5s var(--ease);
  }
  .rec:hover .cover img { transform: scale(1.05); }
  .no-cover {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    background: linear-gradient(160deg, var(--surface-3), var(--surface-2));
  }
  .no-cover span {
    font-family: var(--font-serif);
    font-size: 52px;
    color: var(--text-3);
    opacity: 0.6;
  }
  .cat-badge {
    position: absolute;
    top: 8px;
    left: 8px;
    padding: 3px 10px;
    border-radius: 999px;
    background: rgba(20, 17, 15, 0.72);
    -webkit-backdrop-filter: blur(6px);
    backdrop-filter: blur(6px);
    color: #fff;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.03em;
  }
  .rate-badge {
    position: absolute;
    top: 8px;
    right: 8px;
    padding: 3px 9px;
    border-radius: 999px;
    background: rgba(20, 17, 15, 0.72);
    -webkit-backdrop-filter: blur(6px);
    backdrop-filter: blur(6px);
    color: var(--gold);
    font-size: 11px;
    font-weight: 700;
  }
  .info { padding: 12px 14px 14px; display: flex; flex-direction: column; gap: 5px; flex: 1; }
  .title {
    font-family: var(--font-serif);
    font-weight: 600;
    font-size: 15.5px;
    line-height: 1.35;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    min-height: 2.7em;
  }
  .meta { font-size: 12.5px; color: var(--text-muted); display: flex; gap: 5px; align-items: center; }
  .dot { opacity: 0.5; }
  .artists {
    font-size: 12.5px;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .artist-link { color: var(--text-2); text-decoration: none; }
  .artist-link:hover { color: var(--accent); text-decoration: underline; }
  .bottom { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-top: auto; padding-top: 6px; }
  .bottom .venue {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .price { color: var(--accent); font-weight: 700; font-size: 14px; }
</style>
