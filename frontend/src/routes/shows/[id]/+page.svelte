<script>
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let show = $state(null);
  let loading = $state(true);
  let error = $state('');
  let expandedPlays = $state(null);
  let sceneSorts = $state({});
  let lightboxIdx = $state(-1);
  let allShows = $state([]);

  let lightboxShow = $derived(lightboxIdx >= 0 ? allShows[lightboxIdx] : null);

  let id = $derived($page.params.id);

  function parseSetlist(setlist) {
    if (!setlist) return [];
    const rawLines = setlist.split('\n').map(s => s.trim()).filter(Boolean);
    const lines = [];
    for (const raw of rawLines) {
      const parts = raw.split(/[,，]/).map(s => s.trim()).filter(Boolean);
      if (parts.length > 1) {
        lines.push(...parts);
      } else {
        lines.push(raw);
      }
    }
    const playMap = new Map();
    for (const line of lines) {
      const idx = line.indexOf('•');
      if (idx === -1) {
        if (!playMap.has(line)) playMap.set(line, []);
      } else {
        const play = line.substring(0, idx).trim();
        const scenes = line.substring(idx + 1).split('•').map(s => s.trim()).filter(Boolean);
        if (!playMap.has(play)) playMap.set(play, []);
        for (const s of scenes) {
          if (!playMap.get(play).includes(s)) playMap.get(play).push(s);
        }
      }
    }
    return [...playMap.entries()].map(([play, scenes]) => ({ play, scenes }));
  }

  function sortScenes(play, scenes) {
    const sorted = sceneSorts[play];
    if (!sorted || !Array.isArray(sorted)) return scenes;
    const sortedSet = new Set(sorted);
    const result = sorted.filter(s => scenes.includes(s));
    scenes.forEach(s => { if (!sortedSet.has(s)) result.push(s); });
    return result;
  }

  function isPlayExpanded(play) {
    if (expandedPlays === null) return true;
    return expandedPlays.has(play);
  }

  function togglePlay(play) {
    if (expandedPlays === null) {
      const plays = parseSetlist(show?.setlist).map(p => p.play);
      expandedPlays = new Set(plays);
    }
    const s = new Set(expandedPlays);
    if (s.has(play)) s.delete(play); else s.add(play);
    expandedPlays = s;
  }

  onMount(async () => {
    try {
      const [showData, sorts, shows] = await Promise.all([api.getShow(id), api.getSceneSorts(), api.listAllShows()]);
      show = showData;
      allShows = (shows || []).filter(s => s.poster_url);
      const map = {};
      sorts.forEach(s => { try { map[s.play] = JSON.parse(s.scenes); } catch {} });
      sceneSorts = map;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  function openLightbox(showId) {
    const idx = allShows.findIndex(s => s.id == showId);
    if (idx >= 0) lightboxIdx = idx;
  }

  function closeLightbox() {
    lightboxIdx = -1;
  }

  function prevPoster() {
    if (allShows.length === 0) return;
    lightboxIdx = (lightboxIdx - 1 + allShows.length) % allShows.length;
  }

  function nextPoster() {
    if (allShows.length === 0) return;
    lightboxIdx = (lightboxIdx + 1) % allShows.length;
  }

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
    return { normal: '正常', cancelled: '已取消', pending_tickets: '待开票', no_show: '未赴约' }[status] || status;
  }

  function statusColor(status) {
    return { normal: '#10b981', cancelled: '#ef4444', pending_tickets: '#f59e0b', no_show: '#94a3b8' }[status] || '#94a3b8';
  }

  function formatCost(val) {
    return val == null ? '-' : `¥${val.toFixed(2)}`;
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

  let totalCost = $derived((show?.ticket_cost || 0) + (show?.other_cost || 0));
</script>

<div class="show-detail">
  {#if loading}
    <div class="loading">
      <div class="spinner"></div>
      加载中...
    </div>
  {:else if error}
    <div class="error">{error}</div>
  {:else if show}
    <a href="/shows" class="back-link">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      返回
    </a>

    <div class="detail-card">
      <div class="detail-layout">
        <div class="detail-main">
          <div class="detail-header">
            <div class="header-info">
              <h1>{show.name}</h1>
              <div class="meta-row">
                <span class="status" style="background: {statusColor(show.status)}">{statusLabel(show.status)}</span>
                {#if show.category_name}
                  <a href="/search?q={encodeURIComponent(show.category_name)}" class="category">{show.category_name}</a>
                {/if}
                {#if show.rating}
                  <span class="rating">
                    {#each Array(5) as _, i}<span class:filled={i < show.rating}>★</span>{/each}
                  </span>
                {/if}
              </div>
            </div>
            <div class="header-actions">
              <a href="/shows/{show.id}/edit" class="edit-btn">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
                编辑
              </a>
              <button class="delete-btn" onclick={deleteShow}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
                删除
              </button>
            </div>
          </div>

      <div class="info-grid">
        <div class="info-item">
          <span class="info-label">日期</span>
          <span class="info-value">{formatDate(show.date)}</span>
        </div>
        <div class="info-item">
          <span class="info-label">时间</span>
          <span class="info-value">{formatTime(show.date)}</span>
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
        {#if show.company}
          <div class="info-item">
            <span class="info-label">剧团</span>
            <span class="info-value">{show.company}</span>
          </div>
        {/if}
        {#if show.cast}
          <div class="info-item">
            <span class="info-label">阵容</span>
            <div class="cast-list">
              {#each show.cast.split(/[,，]/) as actor}
                <a href="/cast/{encodeURIComponent(actor.trim())}" class="linkable">{actor.trim()}</a>
              {/each}
            </div>
          </div>
        {/if}
        {#if show.seat}
          <div class="info-item">
            <span class="info-label">座位</span>
            <span class="info-value">{show.seat}</span>
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
          <div class="setlist">
            {#each parseSetlist(show.setlist) as item}
              <div class="setlist-item">
                {#if item.scenes.length > 0}
                  <button class="play-header" onclick={() => togglePlay(item.play)}>
                    <svg class="play-arrow" class:expanded={isPlayExpanded(item.play)} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
                    <a href="/plays/{encodeURIComponent(item.play)}" class="linkable" onclick={(e) => e.stopPropagation()}>{item.play}</a>
                    <span class="scene-count">{item.scenes.length}折</span>
                  </button>
                  {#if isPlayExpanded(item.play)}
                    <div class="scene-list">
                      {#each sortScenes(item.play, item.scenes) as scene}
                        <a href="/plays/{encodeURIComponent(item.play)}" class="scene-item linkable">{scene}</a>
                      {/each}
                    </div>
                  {/if}
                {:else}
                  <a href="/plays/{encodeURIComponent(item.play)}" class="linkable">{item.play}</a>
                {/if}
              </div>
            {/each}
          </div>
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

        {#if show.poster_url}
          <div class="detail-poster">
            <button class="poster-btn" onclick={() => openLightbox(show.id)}>
              <img src={show.poster_url} alt={show.name} loading="lazy" />
            </button>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if lightboxShow}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="lightbox" onclick={closeLightbox}>
    <button class="lightbox-close" onclick={closeLightbox}>
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
    </button>
    {#if allShows.length > 1}
      <button class="lightbox-nav lightbox-prev" onclick={(e) => { e.stopPropagation(); prevPoster(); }}>
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      </button>
      <button class="lightbox-nav lightbox-next" onclick={(e) => { e.stopPropagation(); nextPoster(); }}>
        <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
      </button>
    {/if}
    <div class="lightbox-content" onclick={(e) => e.stopPropagation()}>
      <img src={lightboxShow.poster_url} alt={lightboxShow.name} />
      <div class="lightbox-caption">{lightboxShow.name}</div>
      <div class="lightbox-counter">{lightboxIdx + 1} / {allShows.length}</div>
    </div>
  </div>
{/if}

<style>
  .show-detail { max-width: 1000px; margin: 0 auto; }
  .back-link {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 13px;
    color: var(--text-muted);
    text-decoration: none;
    font-weight: 500;
    margin-bottom: 16px;
    transition: color 0.15s;
  }
  .back-link:hover { color: var(--accent); }
  .loading, .error { text-align: center; padding: 60px 20px; color: var(--text-secondary); display: flex; align-items: center; justify-content: center; gap: 12px; }
  .spinner { width: 20px; height: 20px; border: 2px solid var(--border); border-top-color: var(--accent); border-radius: 50%; animation: spin 0.8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .error { color: var(--danger-text); background: var(--danger-bg); border-radius: var(--radius-md); }

  .detail-card {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 32px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .detail-layout {
    display: flex;
    gap: 28px;
  }

  .detail-main { flex: 1; min-width: 0; }

  .detail-poster {
    width: 200px;
    flex-shrink: 0;
  }

  .poster-btn {
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    cursor: pointer;
    transition: transform 0.2s, box-shadow 0.2s;
    display: block;
    padding: 0;
    background: none;
    border: none;
  }

  .poster-btn:hover {
    transform: scale(1.02);
    box-shadow: var(--shadow-md);
  }

  .poster-btn img {
    width: 100%;
    display: block;
    border-radius: var(--radius-md);
  }

  .lightbox {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.92);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    cursor: pointer;
    animation: fadeIn 0.2s ease;
  }

  @keyframes fadeIn { from { opacity: 0; } to { opacity: 1; } }

  .lightbox-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    cursor: default;
  }

  .lightbox-content img {
    max-width: 85vw;
    max-height: 80vh;
    border-radius: var(--radius-md);
    box-shadow: 0 8px 40px rgba(0,0,0,0.5);
  }

  .lightbox-caption {
    font-size: 15px;
    color: #fff;
    font-weight: 500;
  }

  .lightbox-counter {
    font-size: 13px;
    color: rgba(255,255,255,0.5);
  }

  .lightbox-close {
    position: absolute;
    top: 20px;
    right: 20px;
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: rgba(255,255,255,0.1);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    border: none;
    transition: background 0.2s;
    z-index: 10;
  }

  .lightbox-close:hover { background: rgba(255,255,255,0.25); }

  .lightbox-nav {
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
    width: 48px;
    height: 48px;
    border-radius: 50%;
    background: rgba(255,255,255,0.1);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    border: none;
    transition: background 0.2s;
    z-index: 10;
  }

  .lightbox-nav:hover { background: rgba(255,255,255,0.25); }
  .lightbox-prev { left: 20px; }
  .lightbox-next { right: 20px; }

  .detail-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: 24px;
    padding-bottom: 24px;
    border-bottom: 1px solid var(--border);
  }

  .header-info h1 {
    font-size: 28px;
    font-weight: 700;
    margin-bottom: 12px;
    letter-spacing: -0.02em;
  }

  .meta-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }

  .status {
    font-size: 12px;
    padding: 4px 12px;
    border-radius: 20px;
    color: #fff;
    font-weight: 600;
    letter-spacing: 0.02em;
  }

  .category {
    font-size: 12px;
    padding: 4px 12px;
    border-radius: 20px;
    background: var(--bg-surface);
    color: var(--text-secondary);
    text-decoration: none;
    font-weight: 500;
    transition: all 0.15s;
  }

  .category:hover { background: var(--bg-surface-hover); }

  .rating { font-size: 16px; color: var(--border); }
  .rating .filled { color: var(--warning); }

  .header-actions { display: flex; gap: 8px; }

  .edit-btn, .delete-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 16px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    transition: all 0.2s;
  }

  .edit-btn {
    background: var(--bg-surface);
    color: var(--text-primary);
    border: 1px solid var(--border);
  }

  .edit-btn:hover { background: var(--bg-surface-hover); }

  .delete-btn {
    background: var(--danger-bg);
    color: var(--danger-text);
    border: 1px solid transparent;
  }

  .delete-btn:hover { background: var(--danger-bg-hover); }

  .info-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 20px;
    margin-bottom: 28px;
    padding-bottom: 28px;
    border-bottom: 1px solid var(--border);
  }

  .info-item {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .info-label {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .info-value {
    font-size: 15px;
    color: var(--text-primary);
    font-weight: 500;
    line-height: 1.4;
  }

  .linkable { color: var(--accent); text-decoration: none; }
  .linkable:hover { text-decoration: underline; }

  .cast-list { display: flex; flex-wrap: wrap; gap: 6px; font-size: 15px; }

  .section { margin-bottom: 28px; }

  .section h3 {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 12px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .text-content {
    font-size: 15px;
    line-height: 1.8;
    color: var(--text-secondary);
    white-space: pre-wrap;
    padding: 16px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
  }

  .setlist {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 16px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
  }

  .setlist-item { }

  .play-header {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 10px;
    font-size: 15px;
    font-weight: 500;
    cursor: pointer;
    background: none;
    border: none;
    color: var(--text-primary);
    transition: all 0.15s;
    border-radius: var(--radius-sm);
    width: 100%;
    text-align: left;
  }

  .play-header:hover { background: var(--bg-card); color: var(--accent); }

  .play-arrow { transition: transform 0.2s; color: var(--text-muted); flex-shrink: 0; }
  .play-arrow.expanded { transform: rotate(90deg); }

  .scene-count {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
    margin-left: 4px;
    background: var(--bg-card);
    padding: 2px 8px;
    border-radius: 10px;
  }

  .play-header:hover .scene-count {
    background: var(--accent-bg);
    color: var(--accent);
  }

  .scene-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 4px 0 4px 30px;
  }

  .scene-item {
    font-size: 14px;
    padding: 6px 12px;
    border-radius: var(--radius-sm);
    transition: background 0.15s;
    display: block;
  }

  .scene-item:hover { background: var(--bg-card); }

  @media (max-width: 768px) {
    .show-detail { padding: 0; }
    .detail-card { padding: 20px 16px; }
    .detail-layout { flex-direction: column-reverse; gap: 20px; }
    .detail-poster { width: 100%; }
    .detail-header { flex-direction: column; gap: 16px; }
    .header-info h1 { font-size: 22px; }
    .header-actions { width: 100%; display: flex; gap: 8px; }
    .header-actions .edit-btn, .header-actions .delete-btn { flex: 1; justify-content: center; }
    .info-grid { grid-template-columns: 1fr; gap: 16px; }
  }

  @media (max-width: 480px) {
    .detail-card { padding: 16px 12px; }
    .header-info h1 { font-size: 20px; }
  }
</style>
