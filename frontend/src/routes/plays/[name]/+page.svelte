<script>
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let plays = $state([]);
  let shows = $state([]);
  let loading = $state(true);
  let selectedScene = $state(null);
  let sceneSorts = $state({});
  let draggingIdx = $state(-1);
  let dragOverIdx = $state(-1);

  let playName = $derived(decodeURIComponent($page.params.name));

  let scenes = $derived.by(() => {
    const sceneMap = new Map();
    for (const s of shows) {
      if (!s.setlist) continue;
      const lines = s.setlist.split('\n').map(l => l.trim()).filter(Boolean);
      for (const line of lines) {
        const parts = line.split(/[,，]/).map(p => p.trim()).filter(Boolean);
        for (const part of parts) {
          const idx = part.indexOf('•');
          if (idx !== -1) {
            const play = part.substring(0, idx).trim();
            if (play !== playName) continue;
            const sceneNames = part.substring(idx + 1).split('•').map(sc => sc.trim()).filter(Boolean);
            for (const sc of sceneNames) {
              if (!sceneMap.has(sc)) sceneMap.set(sc, { name: sc, count: 0 });
              sceneMap.get(sc).count++;
            }
          }
        }
      }
    }
    const sorted = sceneSorts[playName];
    if (sorted && Array.isArray(sorted)) {
      const sortedSet = new Set(sorted);
      const result = sorted.filter(s => sceneMap.has(s)).map(s => sceneMap.get(s));
      sceneMap.forEach((v, k) => { if (!sortedSet.has(k)) result.push(v); });
      return result;
    }
    return [...sceneMap.values()].sort((a, b) => b.count - a.count);
  });

  let filteredShows = $derived.by(() => {
    if (!selectedScene) return shows;
    return shows.filter(s => {
      if (!s.setlist) return false;
      const parts = s.setlist.split(/[,，\n]/).map(p => p.trim()).filter(Boolean);
      for (const part of parts) {
        const idx = part.indexOf('•');
        if (idx === -1) continue;
        const play = part.substring(0, idx).trim();
        if (play !== playName) continue;
        const sc = part.substring(idx + 1).split('•').map(x => x.trim()).filter(Boolean);
        if (sc.includes(selectedScene)) return true;
      }
      return false;
    });
  });

  onMount(async () => {
    try {
      const [showsData, sorts] = await Promise.all([
        api.getPlayShows(playName),
        api.getSceneSorts()
      ]);
      shows = showsData;
      const map = {};
      sorts.forEach(s => { try { map[s.play] = JSON.parse(s.scenes); } catch {} });
      sceneSorts = map;
    } catch (e) {
      console.error('Failed to load play:', e);
    } finally {
      loading = false;
    }
  });

  function selectScene(scene) {
    selectedScene = selectedScene === scene ? null : scene;
  }

  function handleDragStart(e, idx) {
    draggingIdx = idx;
    e.dataTransfer.effectAllowed = 'move';
  }

  function handleDragOver(e, idx) {
    e.preventDefault();
    dragOverIdx = idx;
  }

  function handleDragEnd() {
    if (draggingIdx >= 0 && dragOverIdx >= 0 && draggingIdx !== dragOverIdx) {
      const currentScenes = scenes;
      const newOrder = currentScenes.map(s => s.name);
      const [moved] = newOrder.splice(draggingIdx, 1);
      newOrder.splice(dragOverIdx, 0, moved);
      sceneSorts = { ...sceneSorts, [playName]: newOrder };
      api.updateSceneSort(playName, JSON.stringify(newOrder)).catch(() => {});
    }
    draggingIdx = -1;
    dragOverIdx = -1;
  }
</script>

<div class="play-detail">
  {#if loading}
    <div class="loading"><div class="spinner"></div>加载中...</div>
  {:else}
    <a href="/plays" class="back-link">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      返回剧目列表
    </a>

    <div class="detail-card">
      <div class="detail-header">
        <div class="header-info">
          <h1>{playName}</h1>
          <div class="meta-row">
            <span class="meta-badge">{shows.length} 场演出</span>
            {#if scenes.length > 0}
              <span class="meta-badge">{scenes.length} 个折子</span>
            {/if}
          </div>
        </div>
      </div>

      <div class="detail-layout">
        {#if scenes.length > 0}
          <aside class="scene-sidebar">
            <div class="sidebar-header">
              <h3>折子列表</h3>
              <button
                class="clear-btn"
                class:active={selectedScene !== null}
                onclick={() => selectedScene = null}
              >
                全部
              </button>
            </div>
            <nav class="scene-nav">
              {#each scenes as scene, i}
                <button
                  class="scene-nav-item"
                  class:active={selectedScene === scene.name}
                  class:dragging={draggingIdx === i}
                  class:drag-over={dragOverIdx === i}
                  draggable="true"
                  onclick={() => selectScene(scene.name)}
                  ondragstart={(e) => handleDragStart(e, i)}
                  ondragover={(e) => handleDragOver(e, i)}
                  ondragend={handleDragEnd}
                >
                  <span class="drag-handle">⠿</span>
                  <span class="scene-name">{scene.name}</span>
                  <span class="scene-count">{scene.count}</span>
                </button>
              {/each}
            </nav>
          </aside>
        {/if}

        <div class="play-content">
          {#if selectedScene}
            <div class="filter-bar">
              <div class="filter-info">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/></svg>
                <span>当前筛选：{selectedScene}</span>
              </div>
              <button class="clear-filter" onclick={() => selectedScene = null}>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
                清除
              </button>
            </div>
          {/if}

          {#if filteredShows.length === 0}
            <div class="empty-state">
              <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 15h8"/><circle cx="9" cy="9.5" r="0.5" fill="currentColor"/><circle cx="15" cy="9.5" r="0.5" fill="currentColor"/></svg>
              <p>暂无相关演出</p>
              <p class="empty-hint">{selectedScene ? '尝试选择其他折子' : '该剧目暂无演出记录'}</p>
            </div>
          {:else}
            <div class="shows-list">
              {#each filteredShows as show (show.id)}
                <ShowCard {show} />
              {/each}
            </div>
          {/if}
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .play-detail { max-width: 1000px; margin: 0 auto; }

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

  .detail-card {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 32px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .detail-header {
    margin-bottom: 28px;
    padding-bottom: 24px;
    border-bottom: 1px solid var(--border);
  }

  .header-info h1 {
    font-size: 28px;
    font-weight: 700;
    letter-spacing: -0.02em;
    margin-bottom: 12px;
  }

  .meta-row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .meta-badge {
    font-size: 13px;
    padding: 4px 12px;
    border-radius: 20px;
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-weight: 500;
  }

  .detail-layout {
    display: flex;
    gap: 28px;
  }

  .scene-sidebar {
    width: 220px;
    flex-shrink: 0;
  }

  .sidebar-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }

  .sidebar-header h3 {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .clear-btn {
    font-size: 12px;
    padding: 4px 10px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    transition: all 0.15s;
    font-weight: 500;
  }

  .clear-btn:hover { background: var(--bg-surface); }
  .clear-btn.active { background: var(--accent-bg); color: var(--accent); }

  .scene-nav {
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
    padding: 4px;
  }

  .scene-nav-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    cursor: pointer;
    transition: all 0.15s;
    text-align: left;
    width: 100%;
    background: transparent;
  }

  .scene-nav-item:hover { background: var(--bg-card); }
  .scene-nav-item.active {
    background: var(--bg-card);
    color: var(--accent);
    font-weight: 600;
    box-shadow: var(--shadow-sm);
  }
  .scene-nav-item.dragging { opacity: 0.5; }
  .scene-nav-item.drag-over {
    box-shadow: inset 0 2px 0 var(--accent);
  }

  .drag-handle {
    color: var(--text-muted);
    font-size: 14px;
    cursor: grab;
    line-height: 1;
  }

  .drag-handle:hover { color: var(--text-secondary); }

  .scene-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .scene-count {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--bg-card);
    padding: 2px 8px;
    border-radius: 10px;
    min-width: 24px;
    text-align: center;
  }

  .scene-nav-item.active .scene-count {
    background: var(--accent-bg);
    color: var(--accent);
  }

  .play-content {
    flex: 1;
    min-width: 0;
  }

  .filter-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    background: var(--accent-bg);
    border-radius: var(--radius-md);
    margin-bottom: 20px;
  }

  .filter-info {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    color: var(--accent);
    font-weight: 500;
  }

  .filter-info svg {
    opacity: 0.7;
  }

  .clear-filter {
    display: flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    color: var(--text-muted);
    padding: 4px 8px;
    border-radius: var(--radius-sm);
    transition: all 0.15s;
    font-weight: 500;
  }

  .clear-filter:hover {
    background: var(--bg-card);
    color: var(--text-primary);
  }

  .shows-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .empty-state {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-muted);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }

  .empty-state svg {
    opacity: 0.3;
  }

  .empty-state p {
    font-size: 15px;
    font-weight: 500;
  }

  .empty-hint {
    font-size: 13px;
    color: var(--text-muted) !important;
    font-weight: 400 !important;
  }

  @media (max-width: 768px) {
    .detail-card { padding: 20px 16px; }
    .detail-layout { flex-direction: column; gap: 20px; }
    .scene-sidebar { width: 100%; }
    .sidebar-header { margin-bottom: 8px; }
    .scene-nav { flex-direction: row; flex-wrap: wrap; gap: 4px; padding: 4px; }
    .scene-nav-item { flex: 0 0 auto; padding: 8px 12px; }
    .drag-handle { display: none; }
    .header-info h1 { font-size: 22px; }
  }
</style>
