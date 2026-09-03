<script>
  import { coverUrl, formatCurrency } from '$lib/api.js';
  import { STATUS_LABELS } from '$lib/statusPrefs.js';
  let { record, selectionMode = false, selected = false } = $props();
  let coverFailed = $state(false);

  // 封面角标：一场演出涉及多个剧种时显示为「拼盘」
  const catBadge = $derived(
    record.categoryNames?.length > 1 ? '拼盘' : record.categoryNames?.[0] || record.categoryName || ''
  );
  const statusText = $derived(STATUS_LABELS[record.active_status] ?? record.active_status);

  // 剧团：最多展示 3 个，超出以 +N 收起，避免多剧团把卡片撑高。
  const troupeTags = $derived(
    (record.company || '').split(/[,，]/).map((s) => s.trim()).filter(Boolean)
  );
  const shownTroupes = $derived(troupeTags.slice(0, 3));
  const extraTroupes = $derived(troupeTags.length - shownTroupes.length);

  function stars(n) {
    return '★'.repeat(n) + '☆'.repeat(Math.max(0, 5 - n));
  }
</script>

<a class="card card-hover rec" href={`/records/${record.id}`}>
  <div class="cover">
    {#if record.coverThumb && !coverFailed}
      <img class="coverable" src={coverUrl(record.coverThumb)} data-full={coverUrl(record.coverFile)} alt={record.name} loading="lazy" onerror={() => (coverFailed = true)} />
    {:else if record.coverFile && !coverFailed}
      <img class="coverable" src={coverUrl(record.coverFile)} alt={record.name} loading="lazy" onerror={() => (coverFailed = true)} />
    {:else}
      <div class="no-cover"><span>{(record.name || '?').slice(0, 1)}</span></div>
    {/if}
    {#if catBadge}<span class="cat-badge">{catBadge}</span>{/if}
    <span class="status-badge s{record.active_status}">{statusText}</span>
    {#if record.rating}
      <span class="rate-badge">★ {record.rating}</span>
    {/if}
    {#if selectionMode}
      <span class="record-check" class:checked={selected} aria-hidden="true">
        {#if selected}✓{/if}
      </span>
    {/if}
  </div>
  <div class="info">
    <div class="title" title={record.name}>{record.name}</div>
    <div class="meta">
      {#if record.dateText}<span>{record.dateText.split(' ')[0]}</span>{/if}
      {#if record.city}<span class="dot">·</span><span>{record.city}</span>{/if}
    </div>
    <div class="artists">
      {#if record.artist_names && record.artist_names.length}
        {#each record.artist_names.slice(0, 3) as name, i}
          {#if record.artist_ids?.[i]}
            <a class="artist-link" href={`/artists/${record.artist_ids[i]}`}>{name}</a>
          {:else}
            <span class="artist-link">{name}</span>
          {/if}{i < Math.min(record.artist_names.length, 3) - 1 ? ' / ' : ''}
        {/each}{record.artist_names.length > 3 ? ' 等' : ''}
      {/if}
    </div>
    <div class="troupes" title={troupeTags.join('、')}>
      {#each shownTroupes as t}
        <span class="troupe-tag">{t}</span>
      {/each}
      {#if extraTroupes > 0}<span class="troupe-tag more">+{extraTroupes}</span>{/if}
    </div>
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
    position: absolute;
    inset: 0;
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
    position: absolute;
    inset: 0;
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
  /* 海报左下角：演出状态（0 正常 / 1 想看 / 2 已取消 / 3 未赴约） */
  .status-badge {
    position: absolute;
    left: 8px;
    bottom: 8px;
    padding: 2px 9px;
    border-radius: 999px;
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.03em;
    color: #fff;
    background: rgba(20, 17, 15, 0.62);
    -webkit-backdrop-filter: blur(6px);
    backdrop-filter: blur(6px);
  }
  .status-badge.s1 { background: rgba(146, 106, 24, 0.82); }
  .status-badge.s2 { background: rgba(90, 90, 96, 0.82); text-decoration: line-through; }
  .status-badge.s3 { background: rgba(93, 62, 110, 0.82); }
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
  /* 批量选择框：嵌入封面右下角，避开左上角标(剧种)与右上角标(评分) */
  .record-check {
    position: absolute;
    right: 8px;
    bottom: 8px;
    z-index: 10;
    width: 22px;
    height: 22px;
    border-radius: 50%;
    background: rgba(20, 17, 15, 0.55);
    border: 2px solid #fff;
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
    display: flex;
    align-items: center;
    justify-content: center;
    color: #fff;
    font-size: 13px;
    font-weight: 700;
    line-height: 1;
    transition: background var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .record-check.checked {
    background: var(--accent);
    border-color: var(--accent);
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
  .meta { font-size: 12.5px; color: var(--text-muted); display: flex; gap: 5px; align-items: center; min-height: 1.45em; }
  .dot { opacity: 0.5; }
  .artists {
    font-size: 12.5px;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-height: 1.45em;
  }
  .artist-link { color: var(--text-2); text-decoration: none; }
  .artist-link:hover { color: var(--accent); text-decoration: underline; }
  .troupes { display: flex; flex-wrap: wrap; gap: 4px; min-height: 1.45em; }
  .troupe-tag {
    font-size: 11px;
    color: var(--text-2);
    background: var(--surface-3);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 1px 8px;
    white-space: nowrap;
    line-height: 1.6;
  }
  .troupe-tag.more {
    color: var(--text-muted);
    background: transparent;
    border-style: dashed;
    cursor: default;
  }
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
