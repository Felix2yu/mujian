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
  let upcoming = $state([]);
  let recent = $state([]);
  let stats = $state(null);

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      const [eventsRes, upcomingRes, recentRes, statsRes] = await Promise.all([
        api.getCalendar(currentYear, currentMonth),
        api.getUpcoming(5),
        api.getRecent(5),
        api.getStats()
      ]);
      events = eventsRes;
      upcoming = upcomingRes;
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
        <span class="stat-value">{stats.total_shows}</span>
        <span class="stat-label">场演出</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{stats.total_hours.toFixed(0)}</span>
        <span class="stat-label">小时</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{stats.total_venues}</span>
        <span class="stat-label">个场馆</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{stats.avg_rating > 0 ? stats.avg_rating.toFixed(1) : '-'}</span>
        <span class="stat-label">平均评分</span>
      </div>
    </div>
  {/if}

  <div class="main-content">
    <div class="calendar-section">
      <Calendar {events} initialYear={currentYear} initialMonth={currentMonth} onmonthchange={(e) => handleMonthChange(e.year, e.month)} />
    </div>

    <div class="sidebar">
      <div class="sidebar-section">
        <h3>即将到来</h3>
        {#if upcoming.length === 0}
          <p class="empty">暂无即将进行的演出</p>
        {:else}
          {#each upcoming as show}
            <ShowCard {show} compact />
          {/each}
        {/if}
      </div>

      <div class="sidebar-section">
        <h3>最近观看</h3>
        {#if recent.length === 0}
          <p class="empty">暂无观看记录</p>
        {:else}
          {#each recent as show}
            <ShowCard {show} compact />
          {/each}
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .home {
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .stats-bar {
    display: flex;
    gap: 32px;
    padding: 20px 24px;
    background: var(--bg-card);
    border-radius: 12px;
  }

  .stat-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }

  .stat-value {
    font-size: 28px;
    font-weight: 700;
    color: #4A90D9;
  }

  .stat-label {
    font-size: 13px;
    color: #666;
  }

  .main-content {
    display: grid;
    grid-template-columns: 1fr 320px;
    gap: 24px;
  }

  .calendar-section {
    background: var(--bg-card);
    border-radius: 12px;
    padding: 24px;
  }

  .sidebar {
    display: flex;
    flex-direction: column;
    gap: 24px;
  }

  .sidebar-section {
    background: var(--bg-card);
    border-radius: 12px;
    padding: 20px;
  }

  .sidebar-section h3 {
    margin-bottom: 16px;
    font-size: 16px;
    color: #333;
  }

  .empty {
    color: #999;
    text-align: center;
    padding: 20px;
    font-size: 14px;
  }

  @media (max-width: 768px) {
    .main-content {
      grid-template-columns: 1fr;
    }

    .stats-bar {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 12px;
      padding: 16px;
    }

    .stat-value {
      font-size: 22px;
    }

    .calendar-section {
      padding: 12px;
    }

    .sidebar-section {
      padding: 16px;
    }
  }

  @media (max-width: 480px) {
    .stats-bar {
      gap: 8px;
      padding: 12px;
    }

    .stat-value {
      font-size: 20px;
    }

    .stat-label {
      font-size: 11px;
    }
  }

  :global(.dark) .stats-bar {
    background: var(--bg-card);
  }

  :global(.dark) .stat-label {
    color: var(--text-muted);
  }

  :global(.dark) .sidebar-section {
    background: var(--bg-card);
  }

  :global(.dark) .sidebar-section h3 {
    color: var(--text-primary);
  }

  :global(.dark) .empty {
    color: var(--text-muted);
  }

  :global(.dark) .calendar-section {
    background: var(--bg-card);
  }
</style>
