<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let allShows = $state([]);
  let loading = $state(true);

  let selectedYear = $state('all');
  let selectedMonth = $state('all');

  let years = $state([]);

  onMount(async () => {
    try {
      allShows = await api.listAllShows();
      const yearSet = new Set();
      allShows.forEach(s => {
        const y = s.date.substring(0, 4);
        yearSet.add(y);
      });
      years = [...yearSet].sort().reverse();
    } catch (e) {
      console.error('Failed to load data:', e);
    } finally {
      loading = false;
    }
  });

  let filteredShows = $derived(allShows.filter(s => {
    if (selectedYear !== 'all' && !s.date.startsWith(selectedYear)) return false;
    if (selectedMonth !== 'all') {
      const m = s.date.substring(5, 7);
      if (m !== selectedMonth) return false;
    }
    return true;
  }));

  function analyzeData(shows) {
    const cats = {};
    const months = {};
    const venues = {};
    const ratings = { 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 };
    const statuses = { normal: 0, cancelled: 0, pending_tickets: 0, no_show: 0 };
    let cost = 0;
    let duration = 0;
    let durCount = 0;

    shows.forEach(show => {
      const cat = show.category_name || '未分类';
      cats[cat] = (cats[cat] || 0) + 1;

      const month = show.date.substring(0, 7);
      months[month] = (months[month] || 0) + 1;

      if (show.venue) {
        venues[show.venue] = (venues[show.venue] || 0) + 1;
      }

      if (show.rating) {
        ratings[show.rating] = (ratings[show.rating] || 0) + 1;
      }

      statuses[show.status] = (statuses[show.status] || 0) + 1;

      if (show.ticket_cost) cost += show.ticket_cost;
      if (show.other_cost) cost += show.other_cost;

      if (show.duration > 0) {
        duration += show.duration;
        durCount++;
      }
    });

    return {
      categoryStats: cats,
      monthlyStats: months,
      venueStats: venues,
      ratingStats: ratings,
      statusStats: statuses,
      totalCost: cost,
      avgDuration: durCount > 0 ? Math.round(duration / durCount) : 0,
      totalCount: shows.length
    };
  }

  let analysis = $derived.by(() => analyzeData(filteredShows));

  let categoryStats = $derived(analysis.categoryStats);
  let monthlyStats = $derived(analysis.monthlyStats);
  let venueStats = $derived(analysis.venueStats);
  let ratingStats = $derived(analysis.ratingStats);
  let statusStats = $derived(analysis.statusStats);
  let totalCost = $derived(analysis.totalCost);
  let avgDuration = $derived(analysis.avgDuration);
  let totalCount = $derived(analysis.totalCount);

  function getMaxValue(obj) {
    return Math.max(...Object.values(obj), 1);
  }

  function getBarWidth(value, max) {
    return (value / max) * 100;
  }

  function formatDuration(mins) {
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (h > 0 && m > 0) return `${h}h ${m}m`;
    if (h > 0) return `${h}h`;
    return `${m}m`;
  }

  const months = [
    { value: 'all', label: '全部' },
    { value: '01', label: '1月' }, { value: '02', label: '2月' },
    { value: '03', label: '3月' }, { value: '04', label: '4月' },
    { value: '05', label: '5月' }, { value: '06', label: '6月' },
    { value: '07', label: '7月' }, { value: '08', label: '8月' },
    { value: '09', label: '9月' }, { value: '10', label: '10月' },
    { value: '11', label: '11月' }, { value: '12', label: '12月' }
  ];

  const catColors = ['#6366f1', '#10b981', '#ef4444', '#8b5cf6', '#f97316', '#06b6d4', '#475569', '#f59e0b'];
</script>

<div class="analytics-page">
  <div class="page-header">
    <h1>数据分析</h1>
    <div class="filters">
      <select bind:value={selectedYear}>
        <option value="all">全部年份</option>
        {#each years as y}
          <option value={y}>{y}年</option>
        {/each}
      </select>
      <select bind:value={selectedMonth}>
        {#each months as m}
          <option value={m.value}>{m.label}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if loading}
    <div class="loading"><div class="spinner"></div>加载中...</div>
  {:else}
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon-wrap" style="background: var(--accent-bg)">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
        </div>
        <span class="stat-value">{totalCount}</span>
        <span class="stat-label">总演出</span>
      </div>
      <div class="stat-card">
        <div class="stat-icon-wrap" style="background: var(--success-bg)">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--success)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
        </div>
        <span class="stat-value">{formatDuration(avgDuration * totalCount)}</span>
        <span class="stat-label">总时长</span>
      </div>
      <div class="stat-card">
        <div class="stat-icon-wrap" style="background: var(--warning-bg)">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--warning)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>
        </div>
        <span class="stat-value">{Object.keys(venueStats).length}</span>
        <span class="stat-label">场馆数</span>
      </div>
      <div class="stat-card">
        <div class="stat-icon-wrap" style="background: var(--danger-bg)">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--danger-text)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
        </div>
        <span class="stat-value">{totalCount > 0 ? (filteredShows.filter(s => s.rating).reduce((a, s) => a + s.rating, 0) / filteredShows.filter(s => s.rating).length || 0).toFixed(1) : '-'}</span>
        <span class="stat-label">平均评分</span>
      </div>
      <div class="stat-card">
        <div class="stat-icon-wrap" style="background: var(--accent-bg)">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/></svg>
        </div>
        <span class="stat-value">¥{totalCost.toFixed(0)}</span>
        <span class="stat-label">总花费</span>
      </div>
      <div class="stat-card">
        <div class="stat-icon-wrap" style="background: var(--success-bg)">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--success)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
        </div>
        <span class="stat-value">{formatDuration(avgDuration)}</span>
        <span class="stat-label">平均时长</span>
      </div>
    </div>

    <div class="charts-grid">
      <div class="chart-card">
        <h3>状态分布</h3>
        <div class="status-bars">
          <div class="status-bar">
            <div class="status-label">
              <span class="status-dot" style="background: #10b981"></span>
              正常
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.normal, totalCount)}%; background: #10b981"></div>
            </div>
            <span class="bar-value">{statusStats.normal}</span>
          </div>
          <div class="status-bar">
            <div class="status-label">
              <span class="status-dot" style="background: #ef4444"></span>
              已取消
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.cancelled, totalCount)}%; background: #ef4444"></div>
            </div>
            <span class="bar-value">{statusStats.cancelled}</span>
          </div>
          <div class="status-bar">
            <div class="status-label">
              <span class="status-dot" style="background: #f59e0b"></span>
              待开票
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.pending_tickets, totalCount)}%; background: #f59e0b"></div>
            </div>
            <span class="bar-value">{statusStats.pending_tickets}</span>
          </div>
          <div class="status-bar">
            <div class="status-label">
              <span class="status-dot" style="background: #94a3b8"></span>
              未赴约
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.no_show, totalCount)}%; background: #94a3b8"></div>
            </div>
            <span class="bar-value">{statusStats.no_show}</span>
          </div>
        </div>
      </div>

      <div class="chart-card">
        <h3>评分分布</h3>
        <div class="rating-bars">
          {#each [5, 4, 3, 2, 1] as star}
            <div class="rating-bar">
              <span class="star-label">{star}★</span>
              <div class="bar-track">
                <div class="bar-fill" style="width: {getBarWidth(ratingStats[star], getMaxValue(ratingStats))}%; background: var(--warning)"></div>
              </div>
              <span class="bar-value">{ratingStats[star]}</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="chart-card">
        <h3>分类统计</h3>
        <div class="category-bars">
          {#each Object.entries(categoryStats).sort((a, b) => b[1] - a[1]).slice(0, 8) as [name, count], i}
            <div class="category-bar">
              <span class="cat-label">{name}</span>
              <div class="bar-track">
                <div class="bar-fill" style="width: {getBarWidth(count, getMaxValue(categoryStats))}%; background: {catColors[i % catColors.length]}"></div>
              </div>
              <span class="bar-value">{count}</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="chart-card">
        <h3>月度趋势</h3>
        <div class="monthly-chart">
          {#each Object.entries(monthlyStats).sort((a, b) => a[0].localeCompare(b[0])).slice(-12) as [month, count]}
            <div class="month-bar">
              <div class="bar-fill-v" style="height: {getBarWidth(count, getMaxValue(monthlyStats))}%">
                <span class="bar-value">{count}</span>
              </div>
              <span class="month-label">{month.substring(5)}</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="chart-card full-width">
        <h3>常去场馆 TOP 10</h3>
        <div class="venue-list">
          {#each Object.entries(venueStats).sort((a, b) => b[1] - a[1]).slice(0, 10) as [venue, count], i}
            <div class="venue-item">
              <span class="venue-rank">#{i + 1}</span>
              <span class="venue-name">{venue}</span>
              <span class="venue-count">{count} 场</span>
            </div>
          {/each}
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .analytics-page {
    margin: 0 auto;
  }

  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 28px;
    flex-wrap: wrap;
    gap: 12px;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    letter-spacing: -0.02em;
  }

  .filters {
    display: flex;
    gap: 8px;
  }

  .filters select {
    padding: 8px 14px;
    border-radius: var(--radius-sm);
    font-size: 13px;
    min-width: 100px;
    border: 1px solid var(--border);
    background: var(--bg-input);
    font-weight: 500;
  }

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

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .stats-grid {
    display: grid;
    grid-template-columns: repeat(6, 1fr);
    gap: 12px;
    margin-bottom: 28px;
  }

  .stat-card {
    background: var(--bg-card);
    border-radius: var(--radius-md);
    padding: 20px;
    text-align: center;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
    transition: all 0.2s;
  }

  .stat-card:hover {
    transform: translateY(-2px);
    box-shadow: var(--shadow-md);
  }

  .stat-icon-wrap {
    width: 44px;
    height: 44px;
    border-radius: var(--radius-md);
    display: flex;
    align-items: center;
    justify-content: center;
    margin: 0 auto 12px;
  }

  .stat-value {
    display: block;
    font-size: 24px;
    font-weight: 700;
    color: var(--text-primary);
    margin-bottom: 4px;
    letter-spacing: -0.02em;
  }

  .stat-label {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .charts-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
  }

  .chart-card {
    background: var(--bg-card);
    border-radius: var(--radius-md);
    padding: 24px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .chart-card.full-width {
    grid-column: 1 / -1;
  }

  .chart-card h3 {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 18px;
    color: var(--text-primary);
    letter-spacing: -0.01em;
  }

  .status-bars, .rating-bars, .category-bars {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .status-bar, .rating-bar, .category-bar {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .status-label, .star-label, .cat-label {
    width: 80px;
    font-size: 13px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
    font-weight: 500;
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .bar-track {
    flex: 1;
    height: 20px;
    background: var(--bg-surface);
    border-radius: 10px;
    overflow: hidden;
  }

  .bar-fill {
    height: 100%;
    border-radius: 10px;
    transition: width 0.5s ease;
  }

  .bar-value {
    width: 30px;
    text-align: right;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .monthly-chart {
    display: flex;
    align-items: flex-end;
    gap: 6px;
    height: 160px;
    padding-top: 20px;
  }

  .month-bar {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    height: 100%;
    justify-content: flex-end;
  }

  .bar-fill-v {
    width: 100%;
    background: var(--accent);
    border-radius: 6px 6px 0 0;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 4px;
    min-height: 24px;
  }

  .bar-fill-v .bar-value {
    color: #fff;
    font-size: 11px;
    width: auto;
  }

  .month-label {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 6px;
    font-weight: 500;
  }

  .venue-list {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
  }

  .venue-item {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 14px;
    background: var(--bg-surface);
    border-radius: var(--radius-sm);
    transition: background 0.15s;
  }

  .venue-item:hover {
    background: var(--bg-surface-hover);
  }

  .venue-rank {
    font-size: 13px;
    font-weight: 700;
    color: var(--accent);
    width: 28px;
  }

  .venue-name {
    flex: 1;
    font-size: 13px;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 500;
  }

  .venue-count {
    font-size: 12px;
    color: var(--text-muted);
    flex-shrink: 0;
    font-weight: 500;
  }

  @media (max-width: 768px) {
    .stats-grid {
      grid-template-columns: repeat(3, 1fr);
    }

    .charts-grid {
      grid-template-columns: 1fr;
    }

    .venue-list {
      grid-template-columns: 1fr;
    }

    .page-header {
      flex-direction: column;
      align-items: flex-start;
    }
  }

  @media (max-width: 480px) {
    .stats-grid {
      grid-template-columns: repeat(2, 1fr);
    }

    .stat-value {
      font-size: 20px;
    }

    .status-label, .star-label, .cat-label {
      width: 60px;
      font-size: 12px;
    }

    .filters {
      width: 100%;
    }

    .filters select {
      flex: 1;
    }
  }
</style>
