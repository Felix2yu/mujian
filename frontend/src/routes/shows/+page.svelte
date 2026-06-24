<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let shows = $state([]);
  let loading = $state(true);
  let activeTab = $state('normal');
  let categoryFilter = $state('');
  let ratingFilter = $state('');
  let searchQuery = $state('');
  let selectedIds = $state(new Set());
  let batchMode = $state(false);
  let showBatchPanel = $state(false);
  let categories = $state([]);
  let batchCategory = $state('');
  let batchRating = $state('');
  let batchStatus = $state('');
  let batchSaving = $state(false);
  let filtersExpanded = $state(false);
  let isMobile = $state(false);
  let showCancelled = $state(false);
  let showNoShow = $state(false);

  function checkMobile() { isMobile = window.innerWidth <= 768; }

  let hasActiveFilters = $derived(searchQuery || categoryFilter || ratingFilter || showCancelled || showNoShow);

  let filteredShows = $derived(shows.filter(s => {
    const now = new Date();

    if (activeTab === 'normal') {
      const showDate = new Date(s.date);
      if (showDate < now) return false;
      if (s.status === 'cancelled' && !showCancelled) return false;
      if (s.status === 'no_show' && !showNoShow) return false;
      if (s.status !== 'normal' && s.status !== 'pending_tickets' && s.status !== 'cancelled' && s.status !== 'no_show') return false;
    } else {
      if (s.status === 'cancelled' && !showCancelled) return false;
      if (s.status === 'no_show' && !showNoShow) return false;
    }

    if (categoryFilter && s.category_name !== categoryFilter) return false;
    if (ratingFilter) {
      const r = parseInt(ratingFilter);
      if (r === 0 && s.rating != null) return false;
      if (r > 0 && s.rating !== r) return false;
    }
    if (searchQuery) {
      const q = searchQuery.toLowerCase();
      const text = `${s.name} ${s.venue} ${s.company} ${s.cast} ${s.friends} ${s.notes} ${s.review}`.toLowerCase();
      if (!text.includes(q)) return false;
    }
    return true;
  }).sort((a, b) => {
    if (activeTab === 'normal') {
      return new Date(a.date) - new Date(b.date);
    }
    return new Date(b.date) - new Date(a.date);
  }));

  let allSelected = $derived(filteredShows.length > 0 && filteredShows.every(s => selectedIds.has(s.id)));

  onMount(async () => {
    checkMobile();
    window.addEventListener('resize', checkMobile);
    try {
      const [allShows, cats] = await Promise.all([
        api.listAllShows(),
        api.listCategories()
      ]);
      shows = allShows;
      categories = cats || [];
    } catch (e) {
      console.error('Failed to load shows:', e);
    } finally {
      loading = false;
    }
  });

  function clearFilters() {
    searchQuery = '';
    categoryFilter = '';
    ratingFilter = '';
    showCancelled = false;
    showNoShow = false;
  }

  function toggleSelectAll() {
    if (allSelected) { selectedIds.clear(); }
    else { filteredShows.forEach(s => selectedIds.add(s.id)); }
    selectedIds = selectedIds;
  }

  function toggleSelect(id) {
    if (selectedIds.has(id)) { selectedIds.delete(id); }
    else { selectedIds.add(id); }
    selectedIds = selectedIds;
  }

  function toggleBatchMode() {
    batchMode = !batchMode;
    if (!batchMode) { selectedIds.clear(); selectedIds = selectedIds; showBatchPanel = false; }
  }

  async function applyBatchUpdate() {
    if (selectedIds.size === 0) return;
    const data = {};
    if (batchCategory !== '') data.category_id = parseInt(batchCategory) || null;
    if (batchRating !== '') data.rating = parseInt(batchRating) || null;
    if (batchStatus !== '') data.status = batchStatus;
    if (Object.keys(data).length === 0) return;
    batchSaving = true;
    try {
      await api.batchUpdate([...selectedIds], data);
      selectedIds.clear(); selectedIds = selectedIds; showBatchPanel = false;
      batchCategory = ''; batchRating = ''; batchStatus = '';
      shows = await api.listAllShows();
    } catch (e) {
      alert('批量更新失败: ' + e.message);
    } finally {
      batchSaving = false;
    }
  }

  async function applyBatchDelete() {
    if (selectedIds.size === 0 || !confirm(`确定删除选中的 ${selectedIds.size} 场演出吗？`)) return;
    batchSaving = true;
    try {
      await api.batchDelete([...selectedIds]);
      selectedIds.clear(); selectedIds = selectedIds; showBatchPanel = false;
      shows = await api.listAllShows();
    } catch (e) {
      alert('批量删除失败: ' + e.message);
    } finally {
      batchSaving = false;
    }
  }

  async function deleteShow(id) {
    if (!confirm('确定删除？')) return;
    try {
      await api.deleteShow(id);
      shows = shows.filter(s => s.id !== id);
    } catch (e) {
      alert('删除失败: ' + e.message);
    }
  }
</script>

<div class="shows-page">
  <div class="page-header">
    <div class="header-left">
      <h1>演出</h1>
      <span class="count-badge">{shows.length}</span>
    </div>
    <div class="header-right">
      <button class="action-btn" class:active={batchMode} onclick={toggleBatchMode}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
        {batchMode ? '退出' : '批量'}
      </button>
      <a href="/shows/import" class="action-btn" title="导入">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>
        导入
      </a>
      <a href={api.getExportUrl()} class="action-btn" download title="导出">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>
        导出
      </a>
      <a href="/shows/new" class="primary-btn">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
        添加
      </a>
    </div>
  </div>

  <div class="tabs">
    <button class="tab" class:active={activeTab === 'normal'} onclick={() => { activeTab = 'normal'; clearFilters(); }}>
      待看列表
      <span class="tab-count">{shows.filter(s => new Date(s.date) >= new Date() && (s.status === 'normal' || s.status === 'pending_tickets')).length}</span>
    </button>
    <button class="tab" class:active={activeTab === 'all'} onclick={() => { activeTab = 'all'; clearFilters(); }}>
      全部列表
      <span class="tab-count">{shows.length}</span>
    </button>
  </div>

  {#if batchMode}
    <div class="batch-bar">
      <span class="batch-info">已选 <strong>{selectedIds.size}</strong> 场</span>
      <label class="select-all-label">
        <input type="checkbox" checked={allSelected} onchange={toggleSelectAll} />
        全选
      </label>
      <div class="batch-actions">
        <button class="batch-action" onclick={applyBatchUpdate} disabled={batchSaving || selectedIds.size === 0}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 11 12 14 22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
          批量更新
        </button>
        <button class="batch-action danger" onclick={applyBatchDelete} disabled={batchSaving || selectedIds.size === 0}>
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
          批量删除
        </button>
      </div>
    </div>

    {#if showBatchPanel}
      <div class="batch-panel">
        <h3>批量更新</h3>
        <div class="batch-form">
          <div class="form-group">
            <label for="batch-cat">分类</label>
            <select id="batch-cat" bind:value={batchCategory}>
              <option value="">不修改</option>
              {#each categories as cat}<option value={cat.id}>{cat.name}</option>{/each}
            </select>
          </div>
          <div class="form-group">
            <label for="batch-rating">评分</label>
            <select id="batch-rating" bind:value={batchRating}>
              <option value="">不修改</option>
              <option value="5">★★★★★ 5</option>
              <option value="4">★★★★ 4</option>
              <option value="3">★★★ 3</option>
              <option value="2">★★ 2</option>
              <option value="1">★ 1</option>
              <option value="0">清除评分</option>
            </select>
          </div>
          <div class="form-group">
            <label for="batch-status">状态</label>
            <select id="batch-status" bind:value={batchStatus}>
              <option value="">不修改</option>
              <option value="normal">正常</option>
              <option value="cancelled">已取消</option>
              <option value="pending_tickets">待开票</option>
              <option value="no_show">未赴约</option>
            </select>
          </div>
          <button class="primary-btn" onclick={applyBatchUpdate} disabled={batchSaving}>
            {batchSaving ? '保存中...' : '应用'}
          </button>
        </div>
      </div>
    {/if}
  {/if}

  <div class="toolbar">
    <button class="filter-toggle" onclick={() => filtersExpanded = !filtersExpanded}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/></svg>
      筛选
      {#if hasActiveFilters}<span class="filter-badge"></span>{/if}
    </button>

    <div class="filters-panel" class:expanded={filtersExpanded || !isMobile}>
      <div class="filters">
        <div class="search-wrapper">
          <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
          <input type="text" class="search-input" placeholder="搜索..." bind:value={searchQuery} />
        </div>
        <select bind:value={categoryFilter}>
          <option value="">全部分类</option>
          {#each categories as cat}
            <option value={cat.name}>{cat.name}</option>
          {/each}
        </select>
        <select bind:value={ratingFilter}>
          <option value="">全部评分</option>
          <option value="5">★★★★★</option>
          <option value="4">★★★★</option>
          <option value="3">★★★</option>
          <option value="2">★★</option>
          <option value="1">★</option>
          <option value="0">无评分</option>
        </select>
        <label class="filter-checkbox">
          <input type="checkbox" bind:checked={showCancelled} />
          显示已取消
        </label>
        <label class="filter-checkbox">
          <input type="checkbox" bind:checked={showNoShow} />
          显示未赴约
        </label>
      </div>
    </div>

    <span class="result-count">{filteredShows.length} / {shows.length}</span>
  </div>

  {#if loading}
    <div class="loading">
      <div class="spinner"></div>
      加载中...
    </div>
  {:else if filteredShows.length === 0}
    <div class="empty">
      <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M8 15h8"/><path d="M9 9h.01"/><path d="M15 9h.01"/></svg>
      <p>{activeTab === 'normal' ? '暂无待看演出' : '暂无演出记录'}</p>
      <a href="/shows/new" class="primary-btn">添加第一场演出</a>
    </div>
  {:else}
    <div class="shows-list">
      {#each filteredShows as show (show.id)}
        <div class="show-item" class:selected={selectedIds.has(show.id)}>
          {#if batchMode}
            <label class="select-check-wrapper">
              <input type="checkbox" class="select-check" checked={selectedIds.has(show.id)} onchange={() => toggleSelect(show.id)} />
            </label>
          {/if}
          <ShowCard {show} />
          <div class="show-actions">
            <a href="/shows/{show.id}/edit" class="edit-btn">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>
            </a>
            <button class="delete-btn" onclick={() => deleteShow(show.id)}>
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            </button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .shows-page { display: flex; flex-direction: column; gap: 16px; }
  .page-header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; }
  .header-left { display: flex; align-items: center; gap: 10px; }
  .header-left h1 { font-size: 24px; font-weight: 700; letter-spacing: -0.02em; }
  .count-badge {
    font-size: 12px;
    font-weight: 600;
    padding: 3px 10px;
    border-radius: 20px;
    background: var(--accent-bg);
    color: var(--accent);
  }
  .header-right { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }

  .primary-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 18px;
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-sm);
    font-weight: 500;
    font-size: 13px;
    transition: all 0.2s;
  }
  .primary-btn:hover { background: var(--accent-light); transform: translateY(-1px); }

  .action-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    color: var(--text-secondary);
    transition: all 0.2s;
    border: 1px solid var(--border);
  }
  .action-btn:hover { background: var(--bg-surface-hover); color: var(--text-primary); }
  .action-btn.active { background: var(--accent); color: #fff; border-color: var(--accent); }

  .tabs {
    display: flex;
    gap: 4px;
    background: var(--bg-surface);
    border-radius: var(--radius-md);
    padding: 4px;
    border: 1px solid var(--border);
  }
  .tab {
    flex: 1;
    padding: 10px 20px;
    border-radius: var(--radius-sm);
    font-size: 14px;
    font-weight: 500;
    color: var(--text-muted);
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    transition: all 0.2s;
  }
  .tab:hover { color: var(--text-secondary); }
  .tab.active { background: var(--bg-card); color: var(--text-primary); box-shadow: var(--shadow-sm); }
  .tab-count {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 20px;
    background: var(--border);
    color: var(--text-muted);
  }
  .tab.active .tab-count { background: var(--accent-bg); color: var(--accent); }

  .toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .filter-toggle {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 14px;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    color: var(--text-secondary);
    border: 1px solid var(--border);
    position: relative;
    transition: all 0.2s;
  }
  .filter-toggle:hover { background: var(--bg-surface-hover); }
  .filter-badge {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 7px;
    height: 7px;
    background: var(--danger-text);
    border-radius: 50%;
  }
  .filters-panel { display: contents; }
  .filters-panel:not(.expanded) .filters { display: none; }
  .filters-panel.expanded .filters, .filters-panel .filters { display: flex; gap: 8px; flex-wrap: wrap; flex: 1; }

  .search-wrapper {
    position: relative;
    display: flex;
    align-items: center;
  }
  .search-icon {
    position: absolute;
    left: 10px;
    color: var(--text-muted);
    pointer-events: none;
  }
  .search-input {
    padding-left: 32px !important;
  }
  .search-input, .filters select {
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    min-width: 140px;
    border: 1px solid var(--border);
    background: var(--bg-input);
  }
  .result-count { font-size: 13px; color: var(--text-muted); font-weight: 500; }

  .filter-checkbox {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-secondary);
    cursor: pointer;
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    transition: background 0.15s;
  }

  .filter-checkbox:hover {
    background: var(--bg-surface);
  }

  .filter-checkbox input {
    width: 16px;
    height: 16px;
    accent-color: var(--accent);
  }

  .batch-bar {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 12px 18px;
    background: var(--bg-card);
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
    flex-wrap: wrap;
  }
  .batch-info { font-size: 14px; color: var(--text-secondary); }
  .batch-info strong { color: var(--accent); }
  .select-all-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
    color: var(--text-secondary);
    cursor: pointer;
  }
  .select-all-label input { width: 16px; height: 16px; }
  .batch-actions { display: flex; gap: 8px; margin-left: auto; }
  .batch-action {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 6px 14px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    font-weight: 500;
    background: var(--bg-surface);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    transition: all 0.2s;
  }
  .batch-action:hover:not(:disabled) { background: var(--bg-surface-hover); }
  .batch-action.danger { background: var(--danger-bg); color: var(--danger-text); border-color: transparent; }
  .batch-action.danger:hover { background: var(--danger-bg-hover); }
  .batch-action:disabled { opacity: 0.6; cursor: not-allowed; }

  .batch-panel {
    padding: 20px;
    background: var(--bg-card);
    border-radius: var(--radius-md);
    border: 1px solid var(--border);
  }
  .batch-panel h3 { font-size: 15px; font-weight: 600; margin-bottom: 16px; }
  .batch-form { display: flex; gap: 16px; align-items: flex-end; flex-wrap: wrap; }
  .batch-form .form-group { min-width: 140px; }
  .batch-form label { display: block; font-size: 13px; color: var(--text-muted); margin-bottom: 6px; font-weight: 500; }
  .batch-form select { width: 100%; padding: 8px 12px; border-radius: var(--radius-sm); border: 1px solid var(--border); }

  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 12px;
    padding: 60px 20px;
    color: var(--text-secondary);
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

  .empty {
    text-align: center;
    padding: 60px 20px;
    color: var(--text-secondary);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
  }
  .empty svg { opacity: 0.3; }
  .empty p { font-size: 15px; }

  .shows-list {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-top: 8px;
  }

  .show-item {
    position: relative;
    border-radius: var(--radius-md);
    transition: all 0.2s;
  }
  .show-item:hover { transform: translateY(-1px); box-shadow: var(--shadow-md); }
  .show-item.selected { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: var(--radius-md); }

  .select-check-wrapper {
    position: absolute;
    top: 14px;
    left: 14px;
    z-index: 5;
    cursor: pointer;
  }
  .select-check {
    width: 18px;
    height: 18px;
    cursor: pointer;
    accent-color: var(--accent);
  }

  .show-actions {
    position: absolute;
    top: 12px;
    right: 12px;
    display: flex;
    gap: 6px;
    opacity: 0;
    transition: opacity 0.2s;
    z-index: 10;
  }
  .show-item:hover .show-actions { opacity: 1; }

  .edit-btn, .delete-btn {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    backdrop-filter: blur(8px);
    transition: all 0.15s;
  }
  .edit-btn {
    background: rgba(255,255,255,0.9);
    color: var(--text-secondary);
    border: 1px solid var(--border);
  }
  .edit-btn:hover { background: var(--bg-card); color: var(--accent); border-color: var(--accent); }
  .delete-btn {
    background: rgba(254,242,242,0.95);
    color: var(--danger-text);
    border: 1px solid transparent;
  }
  .delete-btn:hover { background: var(--danger-bg); }

  @media (max-width: 768px) {
    .header-right { width: 100%; justify-content: flex-start; }
    .filter-toggle { display: inline-flex; }
    .filters-panel:not(.expanded) .filters { display: none; }
    .filters-panel.expanded .filters { display: flex; flex-direction: column; gap: 8px; width: 100%; }
    .search-input, .filters select { width: 100%; min-width: auto; }
    .batch-bar { flex-wrap: wrap; gap: 8px; }
    .batch-actions { margin-left: 0; }
    .show-actions { opacity: 1; }
  }

  @media (max-width: 480px) {
    .show-actions { opacity: 1; }
  }
</style>
