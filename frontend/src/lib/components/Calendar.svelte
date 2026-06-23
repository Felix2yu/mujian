<script>
  import { createEventDispatcher } from 'svelte';

  export let events = [];

  const dispatch = createEventDispatcher();

  let today = new Date();
  let year = today.getFullYear();
  let month = today.getMonth() + 1;

  $: firstDay = new Date(year, month - 1, 1).getDay();
  $: daysInMonth = new Date(year, month, 0).getDate();
  $: calendarDays = buildCalendarDays();

  function buildCalendarDays() {
    const days = [];
    for (let i = 0; i < firstDay; i++) {
      days.push({ day: null, events: [] });
    }
    for (let d = 1; d <= daysInMonth; d++) {
      const dateStr = `${year}-${String(month).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
      const dayEvents = events.filter(e => e.date === dateStr);
      days.push({ day: d, events: dayEvents, isToday: isToday(d) });
    }
    return days;
  }

  function isToday(d) {
    return year === today.getFullYear() && month === today.getMonth() + 1 && d === today.getDate();
  }

  function prevMonth() {
    month--;
    if (month < 1) { month = 12; year--; }
    dispatch('monthChange', { year, month });
  }

  function nextMonth() {
    month++;
    if (month > 12) { month = 1; year++; }
    dispatch('monthChange', { year, month });
  }

  function goToToday() {
    year = today.getFullYear();
    month = today.getMonth() + 1;
    dispatch('monthChange', { year, month });
  }

  const weekDays = ['日', '一', '二', '三', '四', '五', '六'];

  function getEventColor(event) {
    const colors = {
      planned: '#4A90D9',
      watched: '#27AE60',
      cancelled: '#E74C3C'
    };
    return event.color || colors[event.status] || '#999';
  }
</script>

<div class="calendar">
  <div class="calendar-header">
    <button class="nav-btn" on:click={prevMonth}>‹</button>
    <div class="title">
      <span class="year">{year}年</span>
      <span class="month">{month}月</span>
    </div>
    <button class="nav-btn" on:click={nextMonth}>›</button>
    <button class="today-btn" on:click={goToToday}>今天</button>
  </div>

  <div class="calendar-grid">
    {#each weekDays as wd}
      <div class="weekday">{wd}</div>
    {/each}

    {#each calendarDays as cell}
      <div class="day-cell" class:empty={!cell.day} class:today={cell.isToday}>
        {#if cell.day}
          <span class="day-number">{cell.day}</span>
          <div class="day-events">
            {#each cell.events.slice(0, 3) as event}
              <a href="/shows/{event.id}" class="event-dot" style="background: {getEventColor(event)}" title={event.name}>
                <span class="event-text">{event.name}</span>
              </a>
            {/each}
            {#if cell.events.length > 3}
              <span class="more">+{cell.events.length - 3}</span>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <div class="calendar-legend">
    <span class="legend-item"><span class="legend-dot" style="background: #4A90D9"></span>计划中</span>
    <span class="legend-item"><span class="legend-dot" style="background: #27AE60"></span>已观看</span>
    <span class="legend-item"><span class="legend-dot" style="background: #E74C3C"></span>已取消</span>
  </div>
</div>

<style>
  .calendar {
    user-select: none;
  }

  .calendar-header {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 20px;
  }

  .nav-btn {
    width: 36px;
    height: 36px;
    border-radius: 50%;
    font-size: 20px;
    color: #666;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.2s;
  }

  .nav-btn:hover {
    background: #f0f0f0;
  }

  .title {
    font-size: 20px;
    font-weight: 600;
  }

  .year {
    color: #999;
    font-weight: 400;
  }

  .today-btn {
    margin-left: auto;
    padding: 6px 16px;
    border-radius: 20px;
    background: #4A90D9;
    color: #fff;
    font-size: 13px;
    transition: background 0.2s;
  }

  .today-btn:hover {
    background: #3a7bc8;
  }

  .calendar-grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 1px;
    background: #eee;
    border-radius: 8px;
    overflow: hidden;
  }

  .weekday {
    padding: 8px;
    text-align: center;
    font-size: 13px;
    font-weight: 500;
    color: #666;
    background: #f8f8f8;
  }

  .day-cell {
    min-height: 90px;
    padding: 4px;
    background: #fff;
    transition: background 0.2s;
  }

  .day-cell:not(.empty):hover {
    background: #f8f8f8;
  }

  .day-cell.empty {
    background: #fafafa;
  }

  .day-cell.today {
    background: #e8f4fd;
  }

  .day-number {
    display: block;
    font-size: 13px;
    color: #333;
    padding: 2px 6px;
  }

  .today .day-number {
    background: #4A90D9;
    color: #fff;
    border-radius: 50%;
    width: 24px;
    height: 24px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-weight: 600;
  }

  .day-events {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin-top: 2px;
  }

  .event-dot {
    display: block;
    padding: 2px 6px;
    border-radius: 4px;
    color: #fff;
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    cursor: pointer;
    transition: opacity 0.2s;
  }

  .event-dot:hover {
    opacity: 0.85;
  }

  .event-text {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .more {
    font-size: 11px;
    color: #999;
    padding: 0 6px;
  }

  .calendar-legend {
    display: flex;
    gap: 16px;
    margin-top: 16px;
    justify-content: center;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: #666;
  }

  .legend-dot {
    width: 10px;
    height: 10px;
    border-radius: 50%;
  }
</style>
