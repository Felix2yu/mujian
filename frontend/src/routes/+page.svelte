<script>
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import Calendar from '$lib/components/Calendar.svelte';
  import ShowCard from '$lib/components/ShowCard.svelte';

  const stored = (() => {
    try {
      const v = localStorage.getItem('calendarMonth');
      if (v) {
        const [y, m] = JSON.parse(v);
        if (y && m >= 1 && m <= 12) return { year: y, month: m };
      }
    } catch {}
    return null;
  })();

  let currentYear = $state(stored ? stored.year : new Date().getFullYear());
  let currentMonth = $state(stored ? stored.month : new Date().getMonth() + 1);
  let events = $state([]);
  let recent = $state([]);
  let stats = $state(null);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      const [eventsRes, recentRes, statsRes] = await Promise.all([
        api.getCalendar(currentYear, currentMonth),
        api.getRecent(5),
        api.getStats()
      ]);
      events = eventsRes;
      recent = recentRes;
      stats = statsRes;
    } catch (e) {
      console.error('Failed to load data:', e);
    }
  }

  function handleMonthChange(year, month) {
    currentYear = year;
    currentMonth = month;
    localStorage.setItem('calendarMonth', JSON.stringify([year, month]));
    api.getCalendar(year, month).then(e => events = e);
  }
</script>

<div class="home">
  {#if stats && stats.total_shows > 0}
    <div class="stats-bar">
      <div class="stat-item">
        <span class="stat-icon">🎭</span>
        <div class="stat-info">
          <span class="stat-value">{stats.total_shows}</span>
          <span class="stat-label">场演出</span>
        </div>
      </div>
      <div class="stat-divider"></div>
      <div class="stat-item">
        <span class="stat-icon">⏱️</span>
        <div class="stat-info">
          <span class="stat-value">{stats.total_hours.toFixed(0)}</span>
          <span class="stat-label">小时</span>
        </div>
      </div>
      <div class="stat-divider"></div>
      <div class="stat-item">
        <span class="stat-icon">🏛️</span>
        <div class="stat-info">
          <span class="stat-value">{stats.total_venues}</span>
          <span class="stat-label">个场馆</span>
        </div>
      </div>
      <div class="stat-divider"></div>
      <div class="stat-item">
        <span class="stat-icon">⭐</span>
        <div class="stat-info">
          <span class="stat-value">{stats.avg_rating > 0 ? stats.avg_rating.toFixed(1) : '-'}</span>
          <span class="stat-label">平均评分</span>
        </div>
      </div>
    </div>
  {/if}

  <div class="main-content">
    <div class="calendar-section">
      <Calendar {events} initialYear={currentYear} initialMonth={currentMonth} onmonthchange={(e) => handleMonthChange(e.year, e.month)} />
    </div>

    <div class="sidebar">
      <div class="sidebar-section">
        <div class="section-header">
          <h3>最近观看</h3>
          {#if recent.length > 0}
            <span class="section-badge">{recent.length}</span>
          {/if}
        </div>
        {#if recent.length === 0}
          <div class="empty-state">
            <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>
            <p>暂无观看记录</p>
          </div>
        {:else}
          <div class="card-list">
            {#each recent as show}
              <ShowCard {show} compact />
            {/each}
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .home {
    display: flex;
    flex-direction: column;
    gap: 28px;
  }

  .stats-bar {
    display: flex;
    align-items: center;
    gap: 0;
    padding: 24px 32px;
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .stat-item {
    display: flex;
    align-items: center;
    gap: 12px;
    flex: 1;
    justify-content: center;
  }

  .stat-icon {
    font-size: 24px;
    width: 48px;
    height: 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--accent-bg);
    border-radius: var(--radius-md);
  }

  .stat-info {
    display: flex;
    flex-direction: column;
  }

  .stat-value {
    font-size: 24px;
    font-weight: 700;
    color: var(--text-primary);
    line-height: 1.2;
    letter-spacing: -0.02em;
  }

  .stat-label {
    font-size: 13px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .stat-divider {
    width: 1px;
    height: 40px;
    background: var(--border);
    margin: 0 8px;
    flex-shrink: 0;
  }

  .main-content {
    display: grid;
    grid-template-columns: 1fr 360px;
    gap: 28px;
  }

  .calendar-section {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 28px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .sidebar {
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .sidebar-section {
    background: var(--bg-card);
    border-radius: var(--radius-lg);
    padding: 20px;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-sm);
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 16px;
  }

  .sidebar-section h3 {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .section-badge {
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 20px;
    background: var(--accent-bg);
    color: var(--accent);
  }

  .card-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 24px;
    color: var(--text-muted);
  }

  .empty-state svg {
    opacity: 0.4;
  }

  .empty-state p {
    font-size: 14px;
    text-align: center;
  }

  @media (max-width: 1024px) {
    .main-content {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 768px) {
    .stats-bar {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 16px;
      padding: 20px;
    }

    .stat-divider {
      display: none;
    }

    .stat-item {
      justify-content: flex-start;
    }

    .stat-value {
      font-size: 20px;
    }

    .stat-icon {
      width: 40px;
      height: 40px;
      font-size: 20px;
    }

    .calendar-section {
      padding: 16px;
    }

    .sidebar-section {
      padding: 16px;
    }
  }

  @media (max-width: 480px) {
    .stats-bar {
      gap: 12px;
      padding: 16px;
    }

    .stat-value {
      font-size: 18px;
    }

    .stat-label {
      font-size: 12px;
    }
  }
</style>
