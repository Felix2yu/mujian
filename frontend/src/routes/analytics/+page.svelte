<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let stats = null;
  let shows = [];
  let loading = true;

  let categoryStats = {};
  let monthlyStats = {};
  let venueStats = {};
  let ratingStats = { 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 };
  let statusStats = { planned: 0, watched: 0, cancelled: 0 };
  let totalCost = 0;
  let avgDuration = 0;

  onMount(async () => {
    try {
      const [statsRes, showsRes] = await Promise.all([
        api.getStats(),
        api.listAllShows()
      ]);
      stats = statsRes;
      shows = showsRes;
      analyzeData();
    } catch (e) {
      console.error('Failed to load data:', e);
    } finally {
      loading = false;
    }
  });

  function analyzeData() {
    categoryStats = {};
    monthlyStats = {};
    venueStats = {};
    ratingStats = { 1: 0, 2: 0, 3: 0, 4: 0, 5: 0 };
    statusStats = { planned: 0, watched: 0, cancelled: 0 };
    totalCost = 0;
    let totalDuration = 0;
    let durationCount = 0;

    shows.forEach(show => {
      const cat = show.category_name || '未分类';
      categoryStats[cat] = (categoryStats[cat] || 0) + 1;

      const month = show.date.substring(0, 7);
      monthlyStats[month] = (monthlyStats[month] || 0) + 1;

      if (show.venue) {
        venueStats[show.venue] = (venueStats[show.venue] || 0) + 1;
      }

      if (show.rating) {
        ratingStats[show.rating] = (ratingStats[show.rating] || 0) + 1;
      }

      statusStats[show.status] = (statusStats[show.status] || 0) + 1;

      if (show.ticket_cost) totalCost += show.ticket_cost;
      if (show.other_cost) totalCost += show.other_cost;

      if (show.duration > 0) {
        totalDuration += show.duration;
        durationCount++;
      }
    });

    avgDuration = durationCount > 0 ? Math.round(totalDuration / durationCount) : 0;
  }

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

  const catColors = ['#4A90D9', '#27AE60', '#E74C3C', '#9B59B6', '#E67E22', '#1ABC9C', '#34495E', '#F39C12'];
</script>

<div class="analytics-page">
  <h1>数据分析</h1>

  {#if loading}
    <div class="loading"><div class="spinner"></div>加载中...</div>
  {:else if stats}
    <div class="stats-grid">
      <div class="stat-card">
        <span class="stat-value">{stats.total_shows}</span>
        <span class="stat-label">总演出</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{stats.total_hours.toFixed(0)}</span>
        <span class="stat-label">总时长(小时)</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{stats.total_venues}</span>
        <span class="stat-label">场馆数</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">{stats.avg_rating > 0 ? stats.avg_rating.toFixed(1) : '-'}</span>
        <span class="stat-label">平均评分</span>
      </div>
      <div class="stat-card">
        <span class="stat-value">¥{totalCost.toFixed(0)}</span>
        <span class="stat-label">总花费</span>
      </div>
      <div class="stat-card">
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
              <span class="status-dot" style="background: #4A90D9"></span>
              计划中
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.planned, stats.total_shows)}%; background: #4A90D9"></div>
            </div>
            <span class="bar-value">{statusStats.planned}</span>
          </div>
          <div class="status-bar">
            <div class="status-label">
              <span class="status-dot" style="background: #27AE60"></span>
              已观看
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.watched, stats.total_shows)}%; background: #27AE60"></div>
            </div>
            <span class="bar-value">{statusStats.watched}</span>
          </div>
          <div class="status-bar">
            <div class="status-label">
              <span class="status-dot" style="background: #E74C3C"></span>
              已取消
            </div>
            <div class="bar-track">
              <div class="bar-fill" style="width: {getBarWidth(statusStats.cancelled, stats.total_shows)}%; background: #E74C3C"></div>
            </div>
            <span class="bar-value">{statusStats.cancelled}</span>
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
                <div class="bar-fill" style="width: {getBarWidth(ratingStats[star], getMaxValue(ratingStats))}%; background: #F39C12"></div>
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
    max-width: 1200px;
    margin: 0 auto;
  }

  h1 {
    font-size: 24px;
    font-weight: 700;
    margin-bottom: 24px;
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
    width: 24px;
    height: 24px;
    border: 3px solid var(--border);
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
    margin-bottom: 24px;
  }

  .stat-card {
    background: var(--bg-card);
    border-radius: 12px;
    padding: 16px;
    text-align: center;
  }

  .stat-value {
    display: block;
    font-size: 24px;
    font-weight: 700;
    color: var(--accent);
    margin-bottom: 4px;
  }

  .stat-label {
    font-size: 12px;
    color: var(--text-muted);
  }

  .charts-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
  }

  .chart-card {
    background: var(--bg-card);
    border-radius: 12px;
    padding: 20px;
  }

  .chart-card.full-width {
    grid-column: 1 / -1;
  }

  .chart-card h3 {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 16px;
    color: var(--text-primary);
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
    border-radius: 4px;
    overflow: hidden;
  }

  .bar-fill {
    height: 100%;
    border-radius: 4px;
    transition: width 0.5s ease;
  }

  .bar-value {
    width: 30px;
    text-align: right;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
  }

  .monthly-chart {
    display: flex;
    align-items: flex-end;
    gap: 8px;
    height: 150px;
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
    border-radius: 4px 4px 0 0;
    display: flex;
    align-items: flex-start;
    justify-content: center;
    padding-top: 4px;
    min-height: 20px;
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
    padding: 10px 12px;
    background: var(--bg-surface);
    border-radius: 8px;
  }

  .venue-rank {
    font-size: 13px;
    font-weight: 600;
    color: var(--accent);
    width: 24px;
  }

  .venue-name {
    flex: 1;
    font-size: 13px;
    color: var(--text-primary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .venue-count {
    font-size: 12px;
    color: var(--text-muted);
    flex-shrink: 0;
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
  }
</style>
