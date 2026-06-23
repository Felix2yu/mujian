<script>
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { api } from '$lib/api';
  import ShowCard from '$lib/components/ShowCard.svelte';

  let shows = [];
  let loading = true;
  let currentYear = new Date().getFullYear();
  let currentMonth = new Date().getMonth() + 1;
  let statusFilter = '';
  let categoryFilter = '';
  let ratingFilter = '';
  let searchQuery = '';
  let sortBy = 'date_desc';
  let dateMode = 'month';
  let startDate = '';
  let endDate = '';
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

  function checkMobile() {
    isMobile = window.innerWidth <= 768;
  }

  $: hasActiveFilters = searchQuery || statusFilter || categoryFilter || ratingFilter;

  $: filteredShows = shows.filter(s => {
    if (statusFilter && s.status !== statusFilter) return false;
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
    loadShows();
    categories = await api.listCategories();
  });

  async function loadShows() {
    loading = true;
    try {
      if (dateMode === 'range') {
        shows = await api.listShowsByDateRange(startDate, endDate);
      } else {
        shows = await api.listShows(currentYear, currentMonth);
      }
    } catch (e) {
      console.error('Failed to load shows:', e);
    } finally {
      loading = false;
    }
  }

  function changeMonth(delta) {
    currentMonth += delta;
    if (currentMonth < 1) { currentMonth = 12; currentYear--; }
    if (currentMonth > 12) { currentMonth = 1; currentYear++; }
    loadShows();
  }

  function switchMode(mode) {
    dateMode = mode;
    if (mode === 'month') {
      loadShows();
    } else {
      if (!startDate) {
        const now = new Date();
        startDate = `${now.getFullYear()}-01-01`;
        endDate = now.toISOString().split('T')[0];
      }
      loadShows();
    }
  }

  function clearFilters() {
    statusFilter = '';
    categoryFilter = '';
    ratingFilter = '';
    searchQuery = '';
    sortBy = 'date_desc';
  }

  function toggleSelectAll() {
    if (allSelected) {
      selectedIds.clear();
    } else {
      filteredShows.forEach(s => selectedIds.add(s.id));
    }
    selectedIds = selectedIds;
  }

  function toggleSelect(id) {
    if (selectedIds.has(id)) {
      selectedIds.delete(id);
    } else {
      selectedIds.add(id);
    }
    selectedIds = selectedIds;
  }

  function toggleBatchMode() {
    batchMode = !batchMode;
    if (!batchMode) {
      selectedIds.clear();
      selectedIds = selectedIds;
      showBatchPanel = false;
    }
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
      selectedIds.clear();
      selectedIds = selectedIds;
      showBatchPanel = false;
      batchCategory = '';
      batchRating = '';
      batchStatus = '';
      await loadShows();
    } catch (e) {
      alert('批量更新失败: ' + e.message);
    } finally {
      batchSaving = false;
    }
  }

  async function applyBatchDelete() {
    if (selectedIds.size === 0) return;
    if (!confirm(`确定删除选中的 ${selectedIds.size} 场演出吗？`)) return;

    batchSaving = true;
    try {
      await api.batchDelete([...selectedIds]);
      selectedIds.clear();
      selectedIds = selectedIds;
      showBatchPanel = false;
      await loadShows();
    } catch (e) {
      alert('批量删除失败: ' + e.message);
    } finally {
      batchSaving = false;
    }
  }

  async function deleteShow(id) {
    if (!confirm('确定删除这场演出吗？')) return;
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
      <h1>演出列表</h1>
      <div class="date-controls">
        <div class="mode-toggle">
          <button class:active={dateMode === 'month'} on:click={() => switchMode('month')}>按月</button>
          <button class:active={dateMode === 'range'} on:click={() => switchMode('range')}>按日期</button>
        </div>
        {#if dateMode === 'month'}
          <div class="month-nav">
            <button on:click={() => changeMonth(-1)}>‹</button>
            <span>{currentYear}年{currentMonth}月</span>
            <button on:click={() => changeMonth(1)}>›</button>
          </div>
        {:else}
          <div class="date-range">
            <input type="date" bind:value={startDate} on:change={loadShows} />
            <span>至</span>
            <input type="date" bind:value={endDate} on:change={loadShows} />
          </div>
        {/if}
      </div>
    </div>
    <div class="header-right">
      <button class="filter-toggle" on:click={() => filtersExpanded = !filtersExpanded}>
        🔍 筛选 {#if hasActiveFilters}<span class="filter-badge"></span>{/if}
      </button>

      <div class="filters-panel" class:expanded={filtersExpanded || !isMobile}>
        <div class="filters">
          <input type="text" class="search-input" placeholder="搜索..." bind:value={searchQuery} />
          <select bind:value={statusFilter}>
            <option value="">全部状态</option>
            <option value="planned">计划中</option>
            <option value="watched">已观看</option>
            <option value="cancelled">已取消</option>
          </select>
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
            <option value="date_desc">日期 ↓</option>
            <option value="date_asc">日期 ↑</option>
            <option value="name">名称</option>
            <option value="rating_desc">评分 ↓</option>
            <option value="rating_asc">评分 ↑</option>
          </select>
          {#if searchQuery || statusFilter || categoryFilter || ratingFilter}
            <button class="clear-btn" on:click={clearFilters}>清除</button>
          {/if}
        </div>
      </div>

      <span class="result-count">{filteredShows.length}/{shows.length}</span>
      <button class="batch-btn" class:active={batchMode} on:click={toggleBatchMode}>
        {batchMode ? '退出' : '批量'}
      </button>
      <a href="/shows/import" class="action-btn" title="导入">📥</a>
      <a href={api.getExportUrl()} class="action-btn" download title="导出">📤</a>
      <a href="/shows/new" class="add-btn" title="添加">+</a>
    </div>
  </div>

  {#if batchMode && selectedIds.size > 0}
    <div class="batch-bar">
      <span class="batch-count">已选 {selectedIds.size} 场</span>
      <button class="batch-action" on:click={() => showBatchPanel = !showBatchPanel}>
        批量修改
      </button>
      <button class="batch-action danger" on:click={applyBatchDelete} disabled={batchSaving}>
        批量删除
      </button>
      <button class="batch-action" on:click={() => { selectedIds.clear(); selectedIds = selectedIds; }}>
        取消选择
      </button>
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
            {#each categories as cat}
              <option value={cat.id}>{cat.name}</option>
            {/each}
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
            <option value="planned">计划中</option>
            <option value="watched">已观看</option>
            <option value="cancelled">已取消</option>
          </select>
        </div>
        <button class="btn-apply" on:click={applyBatchUpdate} disabled={batchSaving}>
          {batchSaving ? '保存中...' : '应用'}
        </button>
      </div>
    </div>
  {/if}

  {#if loading}
    <div class="loading">加载中...</div>
  {:else if filteredShows.length === 0}
    <div class="empty">
      <p>暂无演出记录</p>
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
            <button class="delete-btn" on:click={() => deleteShow(show.id)}>删除</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .shows-page {
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    flex-wrap: wrap;
    gap: 16px;
  }

  .header-left {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .header-left h1 {
    font-size: 24px;
    font-weight: 700;
  }

  .date-controls {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
  }

  .mode-toggle {
    display: flex;
    background: #f0f0f0;
    border-radius: 8px;
    padding: 2px;
  }

  .mode-toggle button {
    padding: 6px 14px;
    border-radius: 6px;
    font-size: 13px;
    color: #666;
    transition: all 0.2s;
  }

  .mode-toggle button.active {
    background: #fff;
    color: #333;
    box-shadow: 0 1px 3px rgba(0,0,0,0.1);
  }

  .month-nav {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .month-nav button {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: #f0f0f0;
    font-size: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .month-nav button:hover {
    background: #e0e0e0;
  }

  .date-range {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .date-range input[type="date"] {
    padding: 6px 10px;
    border-radius: 6px;
    font-size: 13px;
  }

  .date-range span {
    color: #999;
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }

  .filters {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .search-input {
    padding: 8px 12px;
    border-radius: 8px;
    width: 160px;
    font-size: 13px;
  }

  .filters select {
    padding: 8px 12px;
    border-radius: 8px;
    font-size: 13px;
  }

  .clear-btn {
    padding: 6px 12px;
    background: #fee;
    color: #c00;
    border-radius: 6px;
    font-size: 12px;
    font-weight: 500;
    transition: background 0.2s;
  }

  .clear-btn:hover {
    background: #fdd;
  }

  .result-count {
    font-size: 13px;
    color: #999;
    white-space: nowrap;
  }

  .import-btn, .export-btn {
    padding: 8px 16px;
    background: #f0f0f0;
    color: #333;
    border-radius: 8px;
    font-weight: 500;
    transition: background 0.2s;
  }

  .import-btn:hover, .export-btn:hover {
    background: #e0e0e0;
  }

  .batch-btn {
    padding: 8px 16px;
    background: #f0f0f0;
    color: #333;
    border-radius: 8px;
    font-weight: 500;
    transition: all 0.2s;
  }

  .batch-btn:hover {
    background: #e0e0e0;
  }

  .batch-btn.active {
    background: #4A90D9;
    color: #fff;
  }

  .batch-bar {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 16px;
    background: #fff;
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.1);
  }

  .batch-count {
    font-weight: 500;
    color: #4A90D9;
  }

  .batch-action {
    padding: 6px 16px;
    border-radius: 6px;
    font-size: 13px;
    font-weight: 500;
    background: #f0f0f0;
    color: #333;
    transition: background 0.2s;
  }

  .batch-action:hover:not(:disabled) {
    background: #e0e0e0;
  }

  .batch-action.danger {
    background: #fee;
    color: #c00;
  }

  .batch-action.danger:hover:not(:disabled) {
    background: #fdd;
  }

  .batch-action:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .batch-panel {
    padding: 20px;
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .batch-panel h3 {
    font-size: 16px;
    font-weight: 600;
    margin-bottom: 16px;
  }

  .batch-form {
    display: flex;
    gap: 16px;
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .batch-form .form-group {
    min-width: 150px;
  }

  .batch-form label {
    display: block;
    font-size: 13px;
    color: #666;
    margin-bottom: 6px;
  }

  .batch-form select {
    width: 100%;
    padding: 8px 12px;
    border-radius: 6px;
  }

  .btn-apply {
    padding: 8px 24px;
    background: #4A90D9;
    color: #fff;
    border-radius: 8px;
    font-weight: 500;
    transition: background 0.2s;
  }

  .btn-apply:hover:not(:disabled) {
    background: #3a7bc8;
  }

  .btn-apply:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .add-btn {
    padding: 8px 20px;
    background: #4A90D9;
    color: #fff;
    border-radius: 8px;
    font-weight: 500;
    transition: background 0.2s;
  }

  .add-btn:hover {
    background: #3a7bc8;
  }

  .loading, .empty {
    text-align: center;
    padding: 60px 20px;
    color: #666;
  }

  .empty a {
    display: inline-block;
    margin-top: 12px;
    color: #4A90D9;
    font-weight: 500;
  }

  .shows-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .select-all {
    padding: 12px 16px;
    background: #fff;
    border-radius: 8px;
    border: 1px solid #eee;
  }

  .select-all label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
    color: #666;
    cursor: pointer;
  }

  .select-all input[type="checkbox"] {
    width: 18px;
    height: 18px;
    cursor: pointer;
  }

  .show-item {
    position: relative;
    display: flex;
    align-items: flex-start;
    gap: 12px;
  }

  .show-item.selected {
    background: #f0f7ff;
    border-radius: 8px;
  }

  .select-check {
    margin-top: 20px;
    width: 18px;
    height: 18px;
    flex-shrink: 0;
    cursor: pointer;
  }

  .show-actions {
    position: absolute;
    top: 16px;
    right: 16px;
    display: flex;
    gap: 8px;
    opacity: 0;
    transition: opacity 0.2s;
  }

  .show-item:hover .show-actions {
    opacity: 1;
  }

  .edit-btn, .delete-btn {
    padding: 4px 12px;
    border-radius: 6px;
    font-size: 12px;
  }

  .edit-btn {
    background: #f0f0f0;
    color: #666;
  }

  .edit-btn:hover {
    background: #e0e0e0;
  }

  .delete-btn {
    background: #fee;
    color: #c00;
  }

  .delete-btn:hover {
    background: #fdd;
  }

  .filter-toggle {
    display: none;
    padding: 8px 16px;
    background: #f0f0f0;
    border-radius: 8px;
    font-weight: 500;
    position: relative;
  }

  .filter-badge {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 8px;
    height: 8px;
    background: #E74C3C;
    border-radius: 50%;
  }

  .filters-panel {
    display: contents;
  }

  .action-btn {
    padding: 8px 12px;
    background: #f0f0f0;
    border-radius: 8px;
    font-size: 14px;
    transition: background 0.2s;
  }

  .action-btn:hover {
    background: #e0e0e0;
  }

  @media (max-width: 768px) {
    .page-header {
      flex-direction: column;
      gap: 12px;
    }

    .header-right {
      width: 100%;
      flex-wrap: wrap;
      gap: 8px;
    }

    .filter-toggle {
      display: block;
    }

    .filters-panel {
      display: none;
      width: 100%;
    }

    .filters-panel.expanded {
      display: block;
    }

    .filters {
      flex-direction: column;
      gap: 8px;
      width: 100%;
    }

    .filters select, .search-input {
      width: 100%;
    }

    .result-count {
      font-size: 12px;
    }

    .batch-btn {
      padding: 8px 12px;
      font-size: 13px;
    }

    .import-btn, .export-btn {
      display: none;
    }

    .add-btn {
      width: 36px;
      height: 36px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 18px;
      padding: 0;
    }

    .batch-bar {
      flex-wrap: wrap;
      gap: 8px;
    }

    .batch-panel .batch-form {
      flex-direction: column;
    }

    .batch-form .form-group {
      width: 100%;
    }
  }
</style>
