<script>
  let { events = [], initialYear = new Date().getFullYear(), initialMonth = new Date().getMonth() + 1, onmonthchange } = $props();

  let today = new Date();
  let year = $state(initialYear);
  let month = $state(initialMonth);

  let popupEvents = $state([]);
  let popupPos = $state({ x: 0, y: 0, align: 'center' });
  const POPUP_W = 260;
  const POPUP_GAP = 8;

  let slideDir = $state(0);
  let animKey = $state(0);

  let firstDay = $derived((new Date(year, month - 1, 1).getDay() + 6) % 7);
  let daysInMonth = $derived(new Date(year, month, 0).getDate());
  let calendarDays = $derived((() => {
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
  })());

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

  $effect(() => {
    function handleClick(e) {
      if (popupEvents.length > 0 && !e.target.closest('.popup') && !e.target.closest('.poster-cell') && !e.target.closest('.event-text-btn')) {
        closePopup();
      }
    }
    window.addEventListener('click', handleClick);
    return () => window.removeEventListener('click', handleClick);
  });

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
      normal: '#27AE60',
      cancelled: '#E74C3C',
      pending_tickets: '#F39C12',
      no_show: '#95A5A6'
    };
    return event.color || colors[event.status] || '#999';
  }
</script>

<div class="calendar">
  <div class="calendar-header">
    <button class="nav-btn" onclick={prevMonth}>‹</button>
    <div class="title">
      <span class="year">{year}年</span>
      <span class="month">{month}月</span>
    </div>
    <button class="nav-btn" onclick={nextMonth}>›</button>
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
                  {@const first = cell.events[0]}
                  {@const posterEvents = cell.events.filter(ev => ev.poster_url)}
                  {@const textEvents = cell.events.filter(ev => !ev.poster_url)}

                  {#if posterEvents.length > 0}
                    <div class="poster-grid" class:grid-1={posterEvents.length === 1} class:grid-2={posterEvents.length === 2} class:grid-3={posterEvents.length >= 3}>
                      {#each posterEvents.slice(0, 3) as ev}
                        <button class="poster-cell" onclick={(e) => showPopup(cell.events, e)}>
                          <img src={ev.poster_url} alt={ev.name} />
                          <span class="poster-cell-status" style="background: {getEventColor(ev)}"></span>
                        </button>
                      {/each}
                      {#if posterEvents.length > 3}
                        <button class="poster-cell poster-cell-more" onclick={(e) => showPopup(cell.events, e)}>
                          <span class="poster-more-num">+{posterEvents.length - 3}</span>
                        </button>
                      {/if}
                    </div>
                  {:else}
                    <button class="event-text-btn" onclick={(e) => showPopup(cell.events, e)} style="background: {getEventColor(first)}">
                      <span class="event-text">{first.name}</span>
                      {#if cell.events.length > 1}
                        <span class="text-count">{cell.events.length}</span>
                      {/if}
                    </button>
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
    <span class="legend-item"><span class="legend-dot" style="background: #27AE60"></span>正常</span>
    <span class="legend-item"><span class="legend-dot" style="background: #E74C3C"></span>已取消</span>
    <span class="legend-item"><span class="legend-dot" style="background: #F39C12"></span>待开票</span>
    <span class="legend-item"><span class="legend-dot" style="background: #95A5A6"></span>未赴约</span>
  </div>
</div>

{#if popupEvents.length > 0}
  <!-- svelte-ignore a11y_no_static_element_interactions -->
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
    -webkit-tap-highlight-color: transparent;
  }

  .nav-btn:hover {
    background: #f0f0f0;
  }

  .nav-btn:active {
    background: #e0e0e0;
    transform: scale(0.95);
  }

  .calendar-grid-wrap {
    overflow: hidden;
    border-radius: 8px;
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
    overflow: hidden;
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

  .poster-grid {
    display: grid;
    gap: 2px;
    width: 100%;
  }

  .poster-grid.grid-1 {
    grid-template-columns: 1fr;
  }

  .poster-grid.grid-2 {
    grid-template-columns: 1fr 1fr;
  }

  .poster-grid.grid-3 {
    grid-template-columns: 1fr 1fr;
    grid-template-rows: 1fr 1fr;
  }

  .poster-cell {
    position: relative;
    border-radius: 3px;
    overflow: hidden;
    border: none;
    padding: 0;
    background: #f0f0f0;
    cursor: pointer;
    aspect-ratio: 2/3;
  }

  .poster-grid.grid-2 .poster-cell {
    aspect-ratio: auto;
    height: 100%;
  }

  .poster-grid.grid-3 .poster-cell:first-child {
    grid-row: 1 / 3;
    aspect-ratio: auto;
    height: 100%;
  }

  .poster-grid.grid-3 .poster-cell:nth-child(2),
  .poster-grid.grid-3 .poster-cell:nth-child(3),
  .poster-grid.grid-3 .poster-cell:nth-child(4) {
    aspect-ratio: auto;
    height: 100%;
  }

  .poster-cell img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .poster-cell:hover {
    opacity: 0.9;
  }

  .poster-cell-status {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 2px;
  }

  .poster-cell-more {
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0,0,0,0.7);
  }

  .poster-more-num {
    color: #fff;
    font-size: 12px;
    font-weight: 600;
  }

  .event-text-btn {
    display: block;
    width: 100%;
    padding: 2px 6px;
    border-radius: 4px;
    color: #fff;
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    cursor: pointer;
    text-align: left;
    transition: opacity 0.2s;
    border: none;
  }

  .event-text-btn:hover {
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

  .popup-overlay {
    position: fixed;
    inset: 0;
    z-index: 999;
  }

  .popup {
    position: fixed;
    z-index: 1000;
    background: var(--bg-card);
    border-radius: 12px;
    width: 260px;
    overflow: hidden;
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
    padding: 12px 14px;
  }

  .popup-name {
    font-size: 15px;
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
    margin-bottom: 6px;
  }

  .popup-meta {
    display: flex;
    gap: 12px;
    font-size: 12px;
    margin-bottom: 8px;
  }

  .popup-status {
    font-weight: 500;
  }

  .popup-duration {
    color: #999;
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
    gap: 10px;
    padding: 10px 14px;
    text-decoration: none;
    transition: background 0.15s;
  }

  .popup-item:hover {
    background: #f8f8f8;
  }

  .popup-item-poster {
    width: 40px;
    height: 56px;
    border-radius: 4px;
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
    font-size: 13px;
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
    font-weight: 500;
  }

  @media (max-width: 768px) {
    .calendar-header {
      gap: 8px;
      margin-bottom: 12px;
    }

    .nav-btn {
      width: 40px;
      height: 40px;
      font-size: 22px;
    }

    .today-btn {
      padding: 8px 16px;
      font-size: 14px;
    }

    .title {
      font-size: 18px;
    }

    .weekday {
      padding: 10px 4px;
      font-size: 12px;
    }

    .day-cell {
      min-height: 64px;
      padding: 3px;
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
      gap: 8px;
      margin-top: 12px;
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
      font-size: 20px;
    }

    .today-btn {
      padding: 6px 12px;
      font-size: 13px;
    }

    .title {
      font-size: 16px;
    }

    .weekday {
      padding: 8px 2px;
      font-size: 11px;
    }

    .day-cell {
      min-height: 52px;
      padding: 2px;
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
      width: 220px;
    }

    .calendar-legend {
      gap: 6px;
    }

    .legend-item {
      font-size: 10px;
    }
  }

  :global(.dark) .calendar-grid {
    background: #333;
  }

  :global(.dark) .calendar-header {
    color: #e0e0e0;
  }

  :global(.dark) .nav-btn {
    color: #ccc;
  }

  :global(.dark) .nav-btn:hover {
    background: #333;
  }

  :global(.dark) .title {
    color: #e0e0e0;
  }

  :global(.dark) .year {
    color: #777;
  }

  :global(.dark) .today-btn {
    background: #4A90D9;
    color: #fff;
  }

  :global(.dark) .today-btn:hover {
    background: #3a7bc8;
  }

  :global(.dark) .weekday {
    background: #2a2a2a;
    color: #999;
  }

  :global(.dark) .day-cell {
    background: #1e1e1e;
  }

  :global(.dark) .day-cell:not(.empty):hover {
    background: #2a2a2a;
  }

  :global(.dark) .day-cell.empty {
    background: #1a1a1a;
  }

  :global(.dark) .day-cell.today {
    background: #1a2a3a;
  }

  :global(.dark) .day-number {
    color: #ccc;
  }

  :global(.dark) .today .day-number {
    background: #4A90D9;
    color: #fff;
  }

  :global(.dark) .poster-cell-more {
    background: rgba(0,0,0,0.7);
  }

  :global(.dark) .poster-cell {
    background: #333;
  }

  :global(.dark) .legend-item {
    color: #999;
  }
</style>
