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
  let dateMode = 'month';
  let startDate = '';
  let endDate = '';

  $: filteredShows = statusFilter
    ? shows.filter(s => s.status === statusFilter)
    : shows;

  onMount(() => loadShows());

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
      <div class="filters">
        <select bind:value={statusFilter}>
          <option value="">全部状态</option>
          <option value="planned">计划中</option>
          <option value="watched">已观看</option>
          <option value="cancelled">已取消</option>
        </select>
      </div>
      <a href="/shows/import" class="import-btn">📥 导入</a>
      <a href={api.getExportUrl()} class="export-btn" download>📤 导出</a>
      <a href="/shows/new" class="add-btn">+ 添加演出</a>
    </div>
  </div>

  {#if loading}
    <div class="loading">加载中...</div>
  {:else if filteredShows.length === 0}
    <div class="empty">
      <p>暂无演出记录</p>
      <a href="/shows/new">添加第一场演出</a>
    </div>
  {:else}
    <div class="shows-list">
      {#each filteredShows as show}
        <div class="show-item">
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

  .filters select {
    padding: 8px 12px;
    border-radius: 8px;
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

  .show-item {
    position: relative;
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

  @media (max-width: 768px) {
    .page-header {
      flex-direction: column;
    }

    .header-right {
      width: 100%;
      justify-content: flex-start;
    }
  }
</style>
