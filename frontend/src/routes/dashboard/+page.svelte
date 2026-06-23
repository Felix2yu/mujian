<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';

  let Chart;
  let data = null;
  let loading = true;
  let monthChart, categoryChart, venueChart, costChart;

  onMount(async () => {
    try {
      const chartModule = await import('chart.js/auto');
      Chart = chartModule.default;
      data = await api.getDashboard();
      await tick();
      drawCharts();
    } catch (e) {
      console.error('Failed to load dashboard:', e);
    } finally {
      loading = false;
    }
  });

  async function tick() {
    return new Promise(resolve => setTimeout(resolve, 50));
  }

  function drawCharts() {
    if (!data || !Chart) return;
    try { drawMonthChart(); } catch (e) { console.warn('Month chart error:', e); }
    try { drawCategoryChart(); } catch (e) { console.warn('Category chart error:', e); }
    try { drawVenueChart(); } catch (e) { console.warn('Venue chart error:', e); }
    try { drawCostChart(); } catch (e) { console.warn('Cost chart error:', e); }
  }

  function drawMonthChart() {
    const canvas = document.getElementById('monthChart');
    if (!canvas || !data.by_month?.length) return;
    const ctx = canvas.getContext('2d');

    const labels = data.by_month.map(m => m.month.slice(5) + '月');
    const values = data.by_month.map(m => m.count);

    if (monthChart) monthChart.destroy();
    monthChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels,
        datasets: [{
          label: '演出场次',
          data: values,
          backgroundColor: '#4A90D9',
          borderRadius: 6
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: { y: { beginAtZero: true, ticks: { stepSize: 1 } } }
      }
    });
  }

  function drawCategoryChart() {
    const canvas = document.getElementById('categoryChart');
    if (!canvas || !data.by_category?.length) return;
    const ctx = canvas.getContext('2d');
    const isMobile = typeof window !== 'undefined' && window.innerWidth <= 768;

    if (categoryChart) categoryChart.destroy();
    categoryChart = new Chart(ctx, {
      type: 'doughnut',
      data: {
        labels: data.by_category.map(c => c.name),
        datasets: [{
          data: data.by_category.map(c => c.count),
          backgroundColor: data.by_category.map(c => c.color)
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            position: isMobile ? 'bottom' : 'right',
            labels: { padding: isMobile ? 8 : 12, usePointStyle: true, font: { size: isMobile ? 11 : 13 } }
          }
        }
      }
    });
  }

  function drawVenueChart() {
    const canvas = document.getElementById('venueChart');
    if (!canvas || !data.by_venue?.length) return;
    const ctx = canvas.getContext('2d');

    if (venueChart) venueChart.destroy();
    venueChart = new Chart(ctx, {
      type: 'bar',
      data: {
        labels: data.by_venue.map(v => v.name),
        datasets: [{
          label: '演出场次',
          data: data.by_venue.map(v => v.count),
          backgroundColor: '#27AE60',
          borderRadius: 6
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: { x: { beginAtZero: true, ticks: { stepSize: 1 } } }
      }
    });
  }

  function drawCostChart() {
    const canvas = document.getElementById('costChart');
    if (!canvas || !data.cost_by_month?.length) return;
    const ctx = canvas.getContext('2d');

    if (costChart) costChart.destroy();
    costChart = new Chart(ctx, {
      type: 'line',
      data: {
        labels: data.cost_by_month.map(m => m.month.slice(5) + '月'),
        datasets: [{
          label: '花费 (元)',
          data: data.cost_by_month.map(m => m.cost),
          borderColor: '#E67E22',
          backgroundColor: 'rgba(230, 126, 34, 0.1)',
          fill: true,
          tension: 0.3
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: { y: { beginAtZero: true } }
      }
    });
  }

  function statusLabel(s) {
    return { planned: '计划中', watched: '已观看', cancelled: '已取消' }[s] || s;
  }

  function statusColor(s) {
    return { planned: '#4A90D9', watched: '#27AE60', cancelled: '#E74C3C' }[s] || '#999';
  }
</script>

<div class="dashboard">
  <h1>数据看板</h1>

  {#if loading}
    <div class="loading">加载中...</div>
  {:else if data}
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-value">{data.total_shows}</div>
        <div class="stat-label">总演出场次</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{data.total_hours.toFixed(0)}</div>
        <div class="stat-label">总时长 (小时)</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{data.total_venues}</div>
        <div class="stat-label">去过场馆</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">¥{data.total_cost.toFixed(0)}</div>
        <div class="stat-label">总花费</div>
      </div>
      <div class="stat-card">
        <div class="stat-value">{data.avg_rating > 0 ? data.avg_rating.toFixed(1) : '-'}</div>
        <div class="stat-label">平均评分</div>
      </div>
    </div>

    <div class="charts-grid">
      <div class="chart-card">
        <h3>月度演出趋势</h3>
        <div class="chart-container">
          <canvas id="monthChart"></canvas>
        </div>
      </div>

      <div class="chart-card">
        <h3>分类分布</h3>
        <div class="chart-container">
          <canvas id="categoryChart"></canvas>
        </div>
      </div>

      <div class="chart-card">
        <h3>场馆排行</h3>
        <div class="chart-container">
          <canvas id="venueChart"></canvas>
        </div>
      </div>

      <div class="chart-card">
        <h3>月度花费</h3>
        <div class="chart-container">
          <canvas id="costChart"></canvas>
        </div>
      </div>
    </div>

    <div class="lists-grid">
      <div class="list-card">
        <h3>演出状态</h3>
        <div class="status-list">
          {#each data.by_status as s}
            <div class="status-item">
              <span class="status-dot" style="background: {statusColor(s.status)}"></span>
              <span class="status-name">{statusLabel(s.status)}</span>
              <span class="status-count">{s.count} 场</span>
            </div>
          {/each}
        </div>
      </div>

      <div class="list-card">
        <h3>高分演出</h3>
        {#if data.top_rated?.length}
          {#each data.top_rated as show}
            <div class="mini-show">
              <a href="/shows/{show.id}">
                <span class="show-name">{show.name}</span>
                <span class="show-rating">{'★'.repeat(show.rating)}</span>
              </a>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无评分记录</p>
        {/if}
      </div>

      <div class="list-card">
        <h3>最近观看</h3>
        {#if data.recent_watched?.length}
          {#each data.recent_watched as show}
            <div class="mini-show">
              <a href="/shows/{show.id}">
                <span class="show-name">{show.name}</span>
                <span class="show-date">{show.date.slice(0, 10)}</span>
              </a>
            </div>
          {/each}
        {:else}
          <p class="empty">暂无观看记录</p>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .dashboard {
    max-width: 1100px;
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
    color: #666;
  }

  .stats-grid {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 16px;
    margin-bottom: 24px;
  }

  .stat-card {
    background: #fff;
    border-radius: 12px;
    padding: 20px;
    text-align: center;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .stat-value {
    font-size: 28px;
    font-weight: 700;
    color: #4A90D9;
  }

  .stat-label {
    font-size: 13px;
    color: #666;
    margin-top: 4px;
  }

  .charts-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 20px;
    margin-bottom: 24px;
  }

  .chart-card {
    background: #fff;
    border-radius: 12px;
    padding: 20px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .chart-card h3 {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 16px;
    color: #333;
  }

  .chart-container {
    height: 250px;
    position: relative;
  }

  .lists-grid {
    display: grid;
    grid-template-columns: 1fr 1fr 1fr;
    gap: 20px;
  }

  .list-card {
    background: #fff;
    border-radius: 12px;
    padding: 20px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
  }

  .list-card h3 {
    font-size: 15px;
    font-weight: 600;
    margin-bottom: 16px;
    color: #333;
  }

  .status-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .status-item {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .status-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }

  .status-name {
    flex: 1;
    font-size: 14px;
  }

  .status-count {
    font-size: 14px;
    color: #666;
  }

  .mini-show a {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 0;
    border-bottom: 1px solid #f0f0f0;
  }

  .mini-show:last-child a {
    border-bottom: none;
  }

  .mini-show a:hover .show-name {
    color: #4A90D9;
  }

  .show-name {
    font-size: 14px;
    transition: color 0.2s;
  }

  .show-rating {
    color: #f39c12;
    font-size: 12px;
  }

  .show-date {
    font-size: 12px;
    color: #999;
  }

  .empty {
    color: #999;
    font-size: 13px;
    text-align: center;
    padding: 20px;
  }

  @media (max-width: 768px) {
    .dashboard {
      padding: 0;
    }

    h1 {
      font-size: 20px;
      margin-bottom: 16px;
    }

    .stats-grid {
      grid-template-columns: repeat(2, 1fr);
      gap: 10px;
    }

    .stat-card {
      padding: 14px 10px;
    }

    .stat-value {
      font-size: 22px;
    }

    .charts-grid {
      grid-template-columns: 1fr;
      gap: 12px;
    }

    .chart-card {
      padding: 16px;
    }

    .chart-container {
      height: 200px;
    }

    .lists-grid {
      grid-template-columns: 1fr;
      gap: 12px;
    }

    .list-card {
      padding: 16px;
    }
  }

  @media (max-width: 480px) {
    .stats-grid {
      gap: 8px;
    }

    .stat-card {
      padding: 12px 8px;
    }

    .stat-value {
      font-size: 20px;
    }

    .stat-label {
      font-size: 11px;
    }
  }
</style>
