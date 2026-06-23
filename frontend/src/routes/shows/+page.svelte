<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let shows = [];
  let loading = true;
  let activeTab = 'normal';
  let categoryFilter = '';
  let ratingFilter = '';
  let searchQuery = '';
  let sortBy = 'date_asc';
  let selectedIds = new Set();
  let batchMode = false;
  let showBatchPanel = false;
  let categories = [];
  let batchCategory = '';
  let batchRating = '';
  let batchStatus = '';
  let batchSaving = false;
  let filtersExpanded = false;
  let isMobile = false;

  function checkMobile() { isMobile = window.innerWidth <= 768; }

  $: hasActiveFilters = searchQuery || categoryFilter || ratingFilter;

  $: filteredShows = shows.filter(s => {
    if (activeTab === 'normal' && s.status !== 'normal' && s.status !== 'pending_tickets') return false;
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
    switch (sortBy) {
      case 'date_asc': return new Date(a.date) - new Date(b.date);
      case 'date_desc': return new Date(b.date) - new Date(a.date);
      case 'name': return a.name.localeCompare(b.name);
      case 'rating_desc': return (b.rating || 0) - (a.rating || 0);
      case 'rating_asc': return (a.rating || 0) - (b.rating || 0);
      default: return 0;
    }
  });

  $: allSelected = filteredShows.length > 0 && filteredShows.every(s => selectedIds.has(s.id));

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
    sortBy = 'date_asc';
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
      selectedIds.clear(); selectedIds = selectedIds;
      showBatchPanel = false;
      batchCategory = ''; batchRating = ''; batchStatus = '';
      const allShows = await api.listAllShows();
      shows = allShows;
    } catch (e) { alert('批量更新失败: ' + e.message); }
    finally { batchSaving = false; }
  }

  async function applyBatchDelete() {
    if (selectedIds.size === 0) return;
    if (!confirm(`确定删除选中的 ${selectedIds.size} 场演出吗？`)) return;
    batchSaving = true;
    try {
      await api.batchDelete([...selectedIds]);
      selectedIds.clear(); selectedIds = selectedIds;
      showBatchPanel = false;
      const allShows = await api.listAllShows();
      shows = allShows;
    } catch (e) { alert('批量删除失败: ' + e.message); }
    finally { batchSaving = false; }
  }
</script>

<div class="shows-page">
  <div class="page-header">
    <div class="header-left">
      <h1>演出列表</h1>
    </div>
    <div class="header-right">
      <button class="batch-btn" class:active={batchMode} on:click={toggleBatchMode}>
        {batchMode ? '退出' : '批量'}
      </button>
      <a href="/shows/import" class="action-btn" title="导入">📥</a>
      <a href={api.getExportUrl()} class="action-btn" download title="导出">📤</a>
      <a href="/shows/new" class="add-btn">+ 添加演出</a>
    </div>
  </div>

  <div class="tabs">
    <button class="tab" class:active={activeTab === 'normal'} on:click={() => { activeTab = 'normal'; clearFilters(); }}>
      待看列表
      {#if shows.filter(s => s.status === 'normal' || s.status === 'pending_tickets').length > 0}
        <span class="tab-count">{shows.filter(s => s.status === 'normal' || s.status === 'pending_tickets').length}</span>
      {/if}
    </button>
    <button class="tab" class:active={activeTab === 'all'} on:click={() => { activeTab = 'all'; clearFilters(); }}>
      全部列表
      <span class="tab-count">{shows.length}</span>
    </button>
  </div>

  <div class="toolbar">
    <button class="filter-toggle" on:click={() => filtersExpanded = !filtersExpanded}>
      🔍 筛选 {#if hasActiveFilters}<span class="filter-badge"></span>{/if}
    </button>

    <div class="filters-panel" class:expanded={filtersExpanded || !isMobile}>
      <div class="filters">
        <input type="text" class="search-input" placeholder="搜索..." bind:value={searchQuery} />
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
        <select bind:value={sortBy}>
          <option value="date_asc">日期 ↑</option>
          <option value="date_desc">日期 ↓</option>
          <option value="name">名称</option>
          <option value="rating_desc">评分 ↓</option>
          <option value="rating_asc">评分 ↑</option>
        </select>
        {#if hasActiveFilters}
          <button class="clear-btn" on:click={clearFilters}>清除</button>
        {/if}
      </div>
    </div>

    <span class="result-count">{filteredShows.length}/{shows.length}</span>
  </div>

  {#if batchMode && selectedIds.size > 0}
    <div class="batch-bar">
      <span class="batch-count">已选 {selectedIds.size} 场</span>
      <button class="batch-action" on:click={() => showBatchPanel = !showBatchPanel}>批量修改</button>
      <button class="batch-action danger" on:click={applyBatchDelete} disabled={batchSaving}>批量删除</button>
      <button class="batch-action" on:click={() => { selectedIds.clear(); selectedIds = selectedIds; }}>取消选择</button>
    </div>
  {/if}

  {#if showBatchPanel}
    <div class="batch-panel">
      <h3>批量修改</h3>
      <div class="batch-form">
        <div class="form-group">
          <label>分类</label>
          <select bind:value={batchCategory}>
            <option value="">不修改</option>
            {#each categories as cat}<option value={cat.id}>{cat.name}</option>{/each}
          </select>
        </div>
        <div class="form-group">
          <label>评分</label>
          <select bind:value={batchRating}>
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
          <label>状态</label>
          <select bind:value={batchStatus}>
            <option value="">不修改</option>
            <option value="normal">正常</option>
            <option value="cancelled">已取消</option>
            <option value="pending_tickets">待开票</option>
            <option value="no_show">未赴约</option>
          </select>
        </div>
        <button class="btn-apply" on:click={applyBatchUpdate} disabled={batchSaving}>
          {batchSaving ? '保存中...' : '应用'}
        </button>
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="loading"><div class="spinner"></div><span>加载中...</span></div>
  {:else if filteredShows.length === 0}
    <div class="empty">
      <p>{activeTab === 'normal' ? '暂无待看演出' : '暂无演出记录'}</p>
      <a href="/shows/new">添加第一场演出</a>
    </div>
  {:else}
    <div class="shows-list">
      {#if batchMode}
        <div class="select-all">
          <label>
            <input type="checkbox" checked={allSelected} on:change={toggleSelectAll} />
            全选 ({filteredShows.length} 场)
          </label>
        </div>
      {/if}
      {#each filteredShows as show (show.id)}
        <div class="show-item" class:selected={selectedIds.has(show.id)}>
          {#if batchMode}
            <input type="checkbox" class="select-check" checked={selectedIds.has(show.id)} on:change={() => toggleSelect(show.id)} />
          {/if}
          <ShowCard {show} />
          <div class="show-actions">
            <a href="/shows/{show.id}/edit" class="edit-btn">编辑</a>
            <button class="delete-btn" on:click={() => { if (confirm('确定删除？')) api.deleteShow(show.id).then(() => shows = shows.filter(s => s.id !== show.id)); }}>删除</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .shows-page { display: flex; flex-direction: column; gap: 16px; }
  .page-header { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; }
  .header-left h1 { font-size: 24px; font-weight: 700; }
  .header-right { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .add-btn { padding: 8px 20px; background: #4A90D9; color: #fff; border-radius: 8px; font-weight: 500; }
  .add-btn:hover { background: #3a7bc8; }
  .action-btn { padding: 8px 12px; background: #f0f0f0; border-radius: 8px; font-size: 14px; }
  .action-btn:hover { background: #e0e0e0; }
  .batch-btn { padding: 8px 16px; background: #f0f0f0; border-radius: 8px; font-weight: 500; font-size: 13px; }
  .batch-btn.active { background: #4A90D9; color: #fff; }

  .tabs { display: flex; gap: 4px; background: #f0f0f0; border-radius: 10px; padding: 4px; }
  .tab { flex: 1; padding: 10px 20px; border-radius: 8px; font-size: 15px; font-weight: 500; color: #666; display: flex; align-items: center; justify-content: center; gap: 8px; }
  .tab.active { background: var(--bg-card); color: var(--text-primary); }
  .tab-count { font-size: 12px; background: #e0e0e0; padding: 1px 7px; border-radius: 10px; }
  .tab.active .tab-count { background: #4A90D9; color: #fff; }

  .toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .filter-toggle { padding: 8px 16px; background: #f0f0f0; border-radius: 8px; font-size: 13px; position: relative; }
  .filter-badge { position: absolute; top: 4px; right: 4px; width: 7px; height: 7px; background: #E74C3C; border-radius: 50%; }
  .filters-panel { display: contents; }
  .filters-panel:not(.expanded) .filters { display: none; }
  .filters-panel.expanded .filters, .filters-panel .filters { display: flex; gap: 8px; flex-wrap: wrap; flex: 1; }
  .search-input { padding: 8px 12px; border-radius: 8px; width: 140px; font-size: 13px; }
  .filters select { padding: 8px 12px; border-radius: 8px; font-size: 13px; }
  .clear-btn { padding: 6px 12px; background: #fee; color: #c00; border-radius: 6px; font-size: 12px; font-weight: 500; }
  .clear-btn:hover { background: #fdd; }
  .result-count { font-size: 13px; color: #999; }

  .batch-bar { display: flex; align-items: center; gap: 10px; padding: 10px 16px; background: var(--bg-card); border-radius: 8px; flex-wrap: wrap; }
  .batch-count { font-weight: 500; color: #4A90D9; }
  .batch-action { padding: 6px 14px; border-radius: 6px; font-size: 13px; background: #f0f0f0; }
  .batch-action:hover:not(:disabled) { background: #e0e0e0; }
  .batch-action.danger { background: #fee; color: #c00; }
  .batch-action.danger:hover { background: #fdd; }
  .batch-action:disabled { opacity: 0.6; }

  .batch-panel { padding: 20px; background: var(--bg-card); border-radius: 12px; }
  .batch-panel h3 { font-size: 16px; font-weight: 600; margin-bottom: 16px; }
  .batch-form { display: flex; gap: 16px; align-items: flex-end; flex-wrap: wrap; }
  .batch-form .form-group { min-width: 140px; }
  .batch-form label { display: block; font-size: 13px; color: #666; margin-bottom: 6px; }
  .batch-form select { width: 100%; padding: 8px 12px; border-radius: 6px; }
  .btn-apply { padding: 8px 24px; background: #4A90D9; color: #fff; border-radius: 8px; font-weight: 500; }
  .btn-apply:hover:not(:disabled) { background: #3a7bc8; }
  .btn-apply:disabled { opacity: 0.6; }

  .loading { display: flex; align-items: center; justify-content: center; gap: 12px; padding: 60px 20px; color: #666; }
  .spinner { width: 24px; height: 24px; border: 3px solid #ddd; border-top-color: #4A90D9; border-radius: 50%; animation: spin 0.8s linear infinite; }
  @keyframes spin { to { transform: rotate(360deg); } }
  .empty { text-align: center; padding: 60px 20px; color: #666; }
  .empty a { display: inline-block; margin-top: 12px; color: #4A90D9; font-weight: 500; }

  .shows-list { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }
  .select-all { padding: 12px 16px; background: var(--bg-card); border-radius: 8px; grid-column: 1 / -1; }
  .select-all label { display: flex; align-items: center; gap: 8px; font-size: 14px; color: var(--text-secondary); cursor: pointer; }
  .select-all input[type="checkbox"] { width: 18px; height: 18px; }
  .show-item { position: relative; border-radius: 10px; transition: all 0.2s; }
  .show-item:hover { transform: translateY(-2px); }
  .show-item.selected { outline: 2px solid var(--accent); outline-offset: -2px; }
  .select-check { position: absolute; top: 12px; left: 12px; width: 18px; height: 18px; z-index: 5; cursor: pointer; }
  .show-actions { position: absolute; top: 10px; right: 10px; display: flex; gap: 6px; opacity: 0; transition: opacity 0.2s; z-index: 10; }
  .show-item:hover .show-actions { opacity: 1; }
  .edit-btn, .delete-btn { padding: 4px 10px; border-radius: 6px; font-size: 11px; font-weight: 500; backdrop-filter: blur(4px); }
  .edit-btn { background: rgba(255,255,255,0.9); color: #555; }
  .edit-btn:hover { background: #fff; color: #333; }
  .delete-btn { background: rgba(254,238,238,0.9); color: #c00; }
  .delete-btn:hover { background: #fee; }

  @media (max-width: 768px) {
    .header-right { width: 100%; justify-content: flex-start; }
    .filter-toggle { display: block; }
    .filters-panel:not(.expanded) .filters { display: none; }
    .filters-panel.expanded .filters { display: flex; flex-direction: column; gap: 8px; width: 100%; }
    .search-input { width: 100%; }
    .batch-bar { flex-wrap: wrap; gap: 8px; }
    .shows-list { grid-template-columns: repeat(2, 1fr); }
  }

  @media (max-width: 480px) {
    .shows-list { grid-template-columns: 1fr; }
  }

  :global(.dark) .action-btn { background: #333; color: #ccc; }
  :global(.dark) .action-btn:hover { background: #444; }
  :global(.dark) .batch-btn { background: #333; color: #ccc; }
  :global(.dark) .batch-btn.active { background: #4A90D9; color: #fff; }
  :global(.dark) .tabs { background: #1e1e1e; }
  :global(.dark) .tab { color: #999; }
  :global(.dark) .tab.active { background: #2a2a2a; color: #e0e0e0; box-shadow: 0 1px 3px rgba(0,0,0,0.3); }
  :global(.dark) .tab-count { background: #333; color: #999; }
  :global(.dark) .filter-toggle { background: #333; color: #ccc; }
  :global(.dark) .batch-bar { background: #2a2a2a; box-shadow: 0 2px 8px rgba(0,0,0,0.3); }
  :global(.dark) .batch-action { background: #333; color: #ccc; }
  :global(.dark) .batch-action:hover:not(:disabled) { background: #444; }
  :global(.dark) .batch-action.danger { background: #3a2020; color: #f66; }
  :global(.dark) .batch-action.danger:hover { background: #4a2020; }
  :global(.dark) .batch-panel { background: #2a2a2a; box-shadow: 0 2px 8px rgba(0,0,0,0.3); }
  :global(.dark) .batch-panel h3 { color: #e0e0e0; }
  :global(.dark) .batch-form label { color: #999; }
  :global(.dark) .select-all { background: #2a2a2a; border-color: #333; }
  :global(.dark) .select-all label { color: #ccc; }
  :global(.dark) .edit-btn { background: rgba(51,51,51,0.9); color: #ccc; }
  :global(.dark) .edit-btn:hover { background: #444; }
  :global(.dark) .delete-btn { background: rgba(58,32,32,0.9); color: #f66; }
  :global(.dark) .delete-btn:hover { background: #4a2020; }
  :global(.dark) .loading { color: #999; }
  :global(.dark) .empty { color: #999; }
  :global(.dark) .result-count { color: #777; }
  :global(.dark) .clear-btn { background: #3a2020; color: #f66; }
  :global(.dark) .clear-btn:hover { background: #4a2020; }
  :global(.dark) .header-left h1 { color: #e0e0e0; }
  :global(.dark) .show-item.selected { background: #1a2a3a; }
  :global(.dark) .spinner { border-color: #444; border-top-color: #4A90D9; }

  :global(.dark) .show-card {
    background: #2a2a2a;
    border-color: #333;
  }

  :global(.dark) .show-card:hover {
    border-color: #444;
  }

  :global(.dark) .card-title {
    color: #e0e0e0;
  }

  :global(.dark) .card-meta {
    color: #888;
  }

  :global(.dark) .card-extra {
    color: #777;
    border-top-color: #333;
  }

  :global(.dark) .category {
    background: #333;
    color: #999;
  }

  :global(.dark) .star-mini {
    color: #555;
  }

  :global(.dark) .card-poster {
    background: #333;
  }
</style>
