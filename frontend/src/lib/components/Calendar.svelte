<script>
  let { events = [], initialYear = new Date().getFullYear(), initialMonth = new Date().getMonth() + 1, onmonthchange } = $props();

  let today = new Date();
  let year = $state(initialYear);
  let month = $state(initialMonth);

  let popupEvents = $state([]);
  let popupPos = $state({ x: 0, y: 0, align: 'center' });
  const POPUP_W = 280;
  const POPUP_GAP = 8;

  let slideDir = $state(0);
  let animKey = $state(0);

  let firstDay = $derived((new Date(year, month - 1, 1).getDay() + 6) % 7);
  let daysInMonth = $derived(new Date(year, month, 0).getDate());
  let calendarDays = $derived.by(() => {
    const safeEvents = Array.isArray(events) ? events : [];
    const days = [];
    for (let i = 0; i < firstDay; i++) {
      days.push({ day: null, events: [] });
    }
    for (let d = 1; d <= daysInMonth; d++) {
      const dateStr = `${year}-${String(month).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
      const dayEvents = safeEvents.filter(e => e.date === dateStr);
      days.push({ day: d, events: dayEvents, isToday: isToday(d) });
    }
    const target = Math.ceil((firstDay + daysInMonth) / 7) * 7;
    while (days.length < target) {
      days.push({ day: null, events: [] });
    }
    return days;
  });

  function isToday(d) {
    return year === today.getFullYear() && month === today.getMonth() + 1 && d === today.getDate();
  }

  function prevMonth() {
    slideDir = -1;
    animKey++;
    month--;
    if (month < 1) { month = 12; year--; }
    onmonthchange?.({ year, month });
  }

  function nextMonth() {
    slideDir = 1;
    animKey++;
    month++;
    if (month > 12) { month = 1; year++; }
    onmonthchange?.({ year, month });
  }

  function goToToday() {
    const prevM = month;
    const prevY = year;
    year = today.getFullYear();
    month = today.getMonth() + 1;
    slideDir = year === prevY ? (month > prevM ? 1 : -1) : (year > prevY ? 1 : -1);
    animKey++;
    onmonthchange?.({ year, month });
  }

  let touchStartX = 0;
  let touchStartY = 0;
  function handleTouchStart(e) {
    touchStartX = e.touches[0].clientX;
    touchStartY = e.touches[0].clientY;
  }
  function handleTouchEnd(e) {
    const dx = e.changedTouches[0].clientX - touchStartX;
    const dy = e.changedTouches[0].clientY - touchStartY;
    if (Math.abs(dx) > 50 && Math.abs(dx) > Math.abs(dy)) {
      if (dx > 0) prevMonth();
      else nextMonth();
    }
  }

  function showPopup(evts, e) {
    e.preventDefault();
    e.stopPropagation();
    const rect = e.currentTarget.getBoundingClientRect();
    const cx = rect.left + rect.width / 2;
    const top = rect.top;
    const vw = window.innerWidth;

    let align = 'center';
    let x = cx;
    if (x - POPUP_W / 2 < 8) {
      x = POPUP_W / 2 + 8;
      align = 'left';
    } else if (x + POPUP_W / 2 > vw - 8) {
      x = vw - POPUP_W / 2 - 8;
      align = 'right';
    }

    let y = top;
    let below = false;
    if (top < 380) {
      y = rect.bottom + POPUP_GAP;
      below = true;
    } else {
      y = top - POPUP_GAP;
    }

    popupPos = { x, y, align, below };
    popupEvents = evts;
  }

  function closePopup() {
    popupEvents = [];
  }

  function getStatusLabel(status) {
    const labels = { normal: '正常', cancelled: '已取消', pending_tickets: '待开票', no_show: '未赴约' };
    return labels[status] || status;
  }

  function formatDuration(mins) {
    if (!mins) return '';
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (h > 0 && m > 0) return `${h}小时${m}分钟`;
    if (h > 0) return `${h}小时`;
    return `${m}分钟`;
  }

  const weekDays = ['一', '二', '三', '四', '五', '六', '日'];

  function getEventColor(event) {
    const colors = {
      normal: '#10b981',
      cancelled: '#ef4444',
      pending_tickets: '#f59e0b',
      no_show: '#94a3b8'
    };
    return event.color || colors[event.status] || '#94a3b8';
  }
</script>

<div class="calendar">
  <div class="calendar-header">
    <div class="nav-group">
      <button class="nav-btn" onclick={prevMonth}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
      </button>
      <button class="nav-btn" onclick={nextMonth}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
      </button>
    </div>
    <div class="title">
      <span class="month">{month}月</span>
      <span class="year">{year}年</span>
    </div>
    <button class="today-btn" onclick={goToToday}>今天</button>
  </div>

  <div class="calendar-grid-wrap"
    ontouchstart={handleTouchStart}
    ontouchend={handleTouchEnd}
  >
    {#key animKey}
      <div class="calendar-grid" in:fly={{ x: slideDir * 30, duration: 200, delay: 50 }}>
        {#each weekDays as wd}
          <div class="weekday">{wd}</div>
        {/each}

        {#each calendarDays as cell}
          <div class="day-cell" class:empty={!cell.day} class:today={cell.isToday}>
            {#if cell.day}
              <span class="day-number">{cell.day}</span>
              <div class="day-events">
                {#if cell.events.length > 0}
                  {@const posterEvents = cell.events.filter(ev => ev.poster_url)}
                  {@const textEvents = cell.events.filter(ev => !ev.poster_url)}

                  {#if posterEvents.length > 0}
                    <div class="poster-grid" class:grid-1={posterEvents.length === 1}>
                      {#each posterEvents.slice(0, 2) as ev}
                        <button class="poster-cell" onclick={(e) => showPopup(cell.events, e)}>
                          <img src={ev.poster_url} alt={ev.name} />
                          <span class="poster-cell-status" style="background: {getEventColor(ev)}"></span>
                        </button>
                      {/each}
                      {#if posterEvents.length > 2}
                        <button class="poster-cell poster-cell-more" onclick={(e) => showPopup(cell.events, e)}>
                          <span class="poster-more-num">+{posterEvents.length - 2}</span>
                        </button>
                      {/if}
                    </div>
                  {/if}

                  {#if textEvents.length > 0}
                    {#each textEvents.slice(0, 1) as ev}
                      <button class="event-text-btn" onclick={(e) => showPopup(cell.events, e)} style="background: {getEventColor(ev)}">
                        <span class="event-text">{ev.name}</span>
                        {#if textEvents.length > 1}
                          <span class="text-count">+{textEvents.length - 1}</span>
                        {/if}
                      </button>
                    {/each}
                  {/if}
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/key}
  </div>

  <div class="calendar-legend">
    <span class="legend-item"><span class="legend-dot" style="background: #10b981"></span>正常</span>
    <span class="legend-item"><span class="legend-dot" style="background: #ef4444"></span>已取消</span>
    <span class="legend-item"><span class="legend-dot" style="background: #f59e0b"></span>待开票</span>
    <span class="legend-item"><span class="legend-dot" style="background: #94a3b8"></span>未赴约</span>
  </div>
</div>

{#if popupEvents.length > 0}
  <div class="popup-overlay" onclick={closePopup}></div>
  <div class="popup popup-{popupPos.align}" class:popup-below={popupPos.below} style="left: {popupPos.x}px; top: {popupPos.y}px">
    {#if popupEvents.length === 1}
      {@const ev = popupEvents[0]}
      {#if ev.poster_url}
        <div class="popup-poster">
          <img src={ev.poster_url} alt={ev.name} />
        </div>
      {/if}
      <div class="popup-content">
        <a href="/shows/{ev.id}" class="popup-name">{ev.name}</a>
        {#if ev.venue}
          <div class="popup-venue">{ev.venue}</div>
        {/if}
        <div class="popup-meta">
          <span class="popup-status" style="color: {getEventColor(ev)}">{getStatusLabel(ev.status)}</span>
          {#if ev.duration}
            <span class="popup-duration">{formatDuration(ev.duration)}</span>
          {/if}
        </div>
        <a href="/shows/{ev.id}" class="popup-link">查看详情 →</a>
      </div>
    {:else}
      <div class="popup-list">
        {#each popupEvents as ev}
          <a href="/shows/{ev.id}" class="popup-item">
            {#if ev.poster_url}
              <div class="popup-item-poster">
                <img src={ev.poster_url} alt={ev.name} />
              </div>
            {/if}
            <div class="popup-item-info">
              <span class="popup-item-name">{ev.name}</span>
              {#if ev.venue}
                <span class="popup-item-venue">{ev.venue}</span>
              {/if}
              <span class="popup-item-status" style="color: {getEventColor(ev)}">{getStatusLabel(ev.status)}</span>
            </div>
          </a>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .calendar {
    user-select: none;
  }

  .calendar-header {
    display: flex;
    align-items: center;
    gap: 16px;
    margin-bottom: 24px;
  }

  .nav-group {
    display: flex;
    gap: 4px;
  }

  .nav-btn {
    width: 36px;
    height: 36px;
    border-radius: var(--radius-sm);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-secondary);
    transition: all 0.2s ease;
    -webkit-tap-highlight-color: transparent;
  }

  .nav-btn:hover {
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  .nav-btn:active {
    transform: scale(0.95);
  }

  .calendar-grid-wrap {
    overflow: hidden;
    border-radius: var(--radius-md);
  }

  .title {
    display: flex;
    align-items: baseline;
    gap: 6px;
  }

  .month {
    font-size: 22px;
    font-weight: 700;
    color: var(--text-primary);
    letter-spacing: -0.02em;
  }

  .year {
    font-size: 15px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .today-btn {
    margin-left: auto;
    padding: 6px 18px;
    border-radius: 20px;
    background: var(--accent);
    color: #fff;
    font-size: 13px;
    font-weight: 500;
    transition: all 0.2s ease;
  }

  .today-btn:hover {
    background: var(--accent-light);
    transform: translateY(-1px);
  }

  .calendar-grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 1px;
    background: var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
  }

  .weekday {
    padding: 10px 4px;
    text-align: center;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-muted);
    background: var(--bg-surface);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .day-cell {
    min-height: 90px;
    padding: 6px;
    background: var(--bg-card);
    transition: background 0.2s;
    overflow: hidden;
  }

  .day-cell:not(.empty):hover {
    background: var(--bg-card-hover);
  }

  .day-cell.empty {
    background: var(--bg-surface);
    opacity: 0.5;
  }

  .day-cell.today {
    background: var(--accent-bg);
  }

  .day-number {
    display: block;
    font-size: 13px;
    color: var(--text-secondary);
    padding: 2px 6px;
    font-weight: 500;
  }

  .today .day-number {
    background: var(--accent);
    color: #fff;
    border-radius: 50%;
    width: 26px;
    height: 26px;
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

  .poster-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 2px;
    width: 100%;
  }

  .poster-grid.grid-1 {
    grid-template-columns: 1fr;
  }

  .poster-cell {
    position: relative;
    border-radius: 4px;
    overflow: hidden;
    border: none;
    padding: 0;
    background: var(--bg-surface);
    cursor: pointer;
    aspect-ratio: 2/3;
  }

  .poster-grid.grid-1 .poster-cell {
    aspect-ratio: 2/3;
  }

  .poster-cell img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .poster-cell:hover {
    opacity: 0.85;
  }

  .poster-cell-status {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 3px;
  }

  .poster-cell-more {
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0,0,0,0.6);
    backdrop-filter: blur(4px);
  }

  .poster-more-num {
    color: #fff;
    font-size: 12px;
    font-weight: 600;
  }

  .event-text-btn {
    display: block;
    width: 100%;
    padding: 3px 8px;
    border-radius: 6px;
    color: #fff;
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    cursor: pointer;
    text-align: left;
    transition: opacity 0.2s;
    border: none;
    font-weight: 500;
  }

  .event-text-btn:hover {
    opacity: 0.85;
  }

  .event-text {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .text-count {
    margin-left: 4px;
    opacity: 0.8;
    font-size: 10px;
  }

  .calendar-legend {
    display: flex;
    gap: 20px;
    margin-top: 20px;
    justify-content: center;
  }

  .legend-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .legend-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
  }

  .popup-overlay {
    position: fixed;
    inset: 0;
    z-index: 999;
  }

  .popup {
    position: fixed;
    z-index: 1000;
    background: var(--bg-card);
    border-radius: var(--radius-md);
    width: 280px;
    overflow: hidden;
    border: 1px solid var(--border);
    box-shadow: var(--shadow-lg);
  }

  .popup-center {
    transform: translateX(-50%) translateY(-100%);
  }

  .popup-left {
    transform: translateX(0) translateY(-100%);
  }

  .popup-right {
    transform: translateX(-100%) translateY(-100%);
  }

  .popup-below.popup-center {
    transform: translateX(-50%) translateY(0);
  }

  .popup-below.popup-left {
    transform: translateX(0) translateY(0);
  }

  .popup-below.popup-right {
    transform: translateX(-100%) translateY(0);
  }

  .popup-poster {
    width: 100%;
    aspect-ratio: 2/3;
    overflow: hidden;
  }

  .popup-poster img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .popup-content {
    padding: 16px;
  }

  .popup-name {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
    display: block;
    margin-bottom: 4px;
  }

  .popup-name:hover {
    color: var(--accent);
  }

  .popup-venue {
    font-size: 13px;
    color: var(--text-muted);
    margin-bottom: 8px;
  }

  .popup-meta {
    display: flex;
    gap: 12px;
    font-size: 12px;
    margin-bottom: 12px;
  }

  .popup-status {
    font-weight: 600;
  }

  .popup-duration {
    color: var(--text-muted);
  }

  .popup-link {
    display: inline-block;
    font-size: 13px;
    color: var(--accent);
    font-weight: 500;
  }

  .popup-list {
    max-height: 320px;
    overflow-y: auto;
  }

  .popup-item {
    display: flex;
    gap: 12px;
    padding: 12px 16px;
    text-decoration: none;
    transition: background 0.15s;
    border-bottom: 1px solid var(--border);
  }

  .popup-item:last-child {
    border-bottom: none;
  }

  .popup-item:hover {
    background: var(--bg-surface);
  }

  .popup-item-poster {
    width: 40px;
    height: 56px;
    border-radius: 6px;
    overflow: hidden;
    flex-shrink: 0;
    background: var(--bg-surface);
  }

  .popup-item-poster img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .popup-item-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .popup-item-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .popup-item-venue {
    font-size: 12px;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .popup-item-status {
    font-size: 11px;
    font-weight: 600;
  }

  @media (max-width: 768px) {
    .calendar-header {
      gap: 10px;
      margin-bottom: 16px;
    }

    .nav-btn {
      width: 40px;
      height: 40px;
    }

    .title {
      gap: 4px;
    }

    .month {
      font-size: 18px;
    }

    .year {
      font-size: 13px;
    }

    .today-btn {
      padding: 8px 16px;
      font-size: 13px;
    }

    .weekday {
      padding: 10px 2px;
      font-size: 11px;
    }

    .day-cell {
      min-height: 64px;
      padding: 4px;
    }

    .day-number {
      font-size: 12px;
      padding: 1px 4px;
    }

    .poster-more-num {
      font-size: 10px;
    }

    .calendar-legend {
      flex-wrap: wrap;
      gap: 12px;
      margin-top: 16px;
    }

    .legend-item {
      font-size: 11px;
    }
  }

  @media (max-width: 480px) {
    .calendar-header {
      gap: 6px;
    }

    .nav-btn {
      width: 36px;
      height: 36px;
    }

    .today-btn {
      padding: 6px 12px;
      font-size: 12px;
    }

    .month {
      font-size: 16px;
    }

    .weekday {
      padding: 8px 2px;
      font-size: 10px;
    }

    .day-cell {
      min-height: 52px;
      padding: 3px;
    }

    .day-number {
      font-size: 11px;
    }

    .poster-grid.grid-1 .poster-cell {
      aspect-ratio: 1;
    }

    .poster-more-num {
      font-size: 9px;
    }

    .popup {
      width: 240px;
    }

    .calendar-legend {
      gap: 8px;
    }

    .legend-item {
      font-size: 10px;
    }
  }
</style>
