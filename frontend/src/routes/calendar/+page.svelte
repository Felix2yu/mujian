<script>
  import { onMount } from 'svelte';
  import { fade, scale } from 'svelte/transition';
  import { api, coverUrl } from '$lib/api.js';

  const now = new Date();

  // 从 URL 初始化年月（?y=&m=），便于刷新与分享
  const params = new URLSearchParams(location.search);
  const qy = parseInt(params.get('y'), 10);
  const qm = parseInt(params.get('m'), 10);

  let year = $state(qy > 2000 ? qy : now.getFullYear());
  let month = $state(qm >= 1 && qm <= 12 ? qm : now.getMonth() + 1);
  let events = $state([]);
  let loading = $state(true);
  let error = $state('');
  let modalDay = $state(null);
  let showYearPicker = $state(false);
  let pickerYear = $state(year);

  function openYearPicker() {
    pickerYear = year;
    showYearPicker = true;
  }

  function shiftYear(dy) {
    pickerYear += dy;
  }

  function selectMonth(m) {
    year = pickerYear;
    month = m;
    showYearPicker = false;
    load();
  }

  // 请求序号：连续翻月时两个 getCalendar 乱序返回会把旧月份的活动渲染到
  // 当前视图，用序号丢弃过期响应。
  let calReqSeq = 0;

  async function load() {
    const seq = ++calReqSeq;
    loading = true;
    error = '';
    try {
      const ev = await api.getCalendar(year, month);
      if (seq !== calReqSeq) return; // 已翻到别的月份，丢弃本次响应
      events = ev;
      const u = new URL(location.href);
      u.searchParams.set('y', String(year));
      u.searchParams.set('m', String(month));
      // 保留 SvelteKit 的历史状态（sveltekit:index 等），否则返回按钮会因
      // 历史索引被清空而回退到主页而非日历。
      history.replaceState(window.history.state ?? {}, '', u);
    } catch (e) {
      if (seq === calReqSeq) error = e.message;
    } finally {
      if (seq === calReqSeq) loading = false;
    }
  }

  function shift(dm) {
    let y = year;
    let m = month + dm;
    while (m < 1) { m += 12; y -= 1; }
    while (m > 12) { m -= 12; y += 1; }
    year = y;
    month = m;
    load();
  }

  function goToday() {
    year = now.getFullYear();
    month = now.getMonth() + 1;
    load();
  }

  const DOW = ['一', '二', '三', '四', '五', '六', '日'];
  const MAX_SHOW = 4;

  // 根据当天演出数量决定海报网格布局
  // 返回 { cols, rows }，保证尽量用最少格子填满
  function posterLayout(n) {
    if (n <= 1) return { cols: 1, rows: 1 };
    if (n === 2) return { cols: 2, rows: 1 };
    if (n === 3) return { cols: 2, rows: 2, spans: [1, 0, 0, 0] }; // 左列跨两行
    if (n <= 4) return { cols: 2, rows: 2 };
    return { cols: 2, rows: 2 }; // 4+ 显示 4 张 + 溢出角标
  }

  // 月历网格：周一起始，前后补空，行数固定为 7 的倍数
  const cells = $derived.by(() => {
    const offset = (new Date(year, month - 1, 1).getDay() + 6) % 7;
    const days = new Date(year, month, 0).getDate();
    const total = Math.ceil((offset + days) / 7) * 7;
    const arr = [];
    for (let i = 0; i < total; i++) {
      const d = i - offset + 1;
      arr.push(d >= 1 && d <= days ? d : null);
    }
    return arr;
  });

  // 按日分组（后端已按时间升序返回）
  const byDay = $derived.by(() => {
    const map = {};
    for (const e of events) {
      const d = new Date(e.date * 1000).getDate();
      (map[d] ??= []).push(e);
    }
    return map;
  });

  function isToday(d) {
    return d === now.getDate() && month === now.getMonth() + 1 && year === now.getFullYear();
  }

  function pad(n) {
    return String(n).padStart(2, '0');
  }

  function dateStr(d) {
    return `${year}-${pad(month)}-${pad(d)}`;
  }

  function openNew(d) {
    modalDay = null;
    // 用 SPA 导航（而非整页跳转）保留历史栈，使新增页的返回按钮回到日历。
    goto(`/records/new?date=${dateStr(d)}`);
  }

  function posterSrc(e) {
    return coverUrl(e.coverThumb || e.coverFile || '');
  }

  const STATUS_LABEL = { 1: '想看', 2: '已取消', 3: '未赴约' };
  const STATUS_CLASS = { 1: 'status-wish', 2: 'status-cancel', 3: 'status-miss' };
  const statusLabel = (s) => STATUS_LABEL[s] || '';
  const statusClass = (s) => STATUS_CLASS[s] || '';

  // 统一入口：点击任意日期弹出当日弹窗（有演出→选择，无演出→新增确认）
  function onCellClick(d) {
    modalDay = d;
  }

  function timeStr(ts) {
    const d = new Date(ts * 1000);
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function categoryColor(name) {
    // 简单哈希生成稳定的柔和色彩
    if (!name) return 'cat-default';
    let h = 0;
    for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) >>> 0;
    return `cat-${(h % 12)}`;
  }

  onMount(load);
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') { modalDay = null; showYearPicker = false; } }} />
<svelte:head><title>日历 - 幕间</title></svelte:head>

<div class="fade-up">
  <div class="page-head">
    <h1>日历</h1>
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  <section class="calendar-card card" aria-label={`${year}年${month}月日历`}>
    <!-- 工具栏 -->
    <header class="cal-header">
      <div class="nav-group">
        <button class="nav-btn" onclick={() => shift(-1)} aria-label="上个月" type="button">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
        <button class="today-btn" onclick={goToday} type="button">今天</button>
        <button class="nav-btn" onclick={() => shift(1)} aria-label="下个月" type="button">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M9 18l6-6-6-6" />
          </svg>
        </button>
      </div>
      <button
        class="ym-display"
        type="button"
        onclick={openYearPicker}
        aria-label={`选择年份与月份，当前 ${year} 年 ${month} 月`}
        aria-haspopup="dialog"
      >
        <span class="y">{year}</span>
        <span class="sep">·</span>
        <span class="m">{pad(month)}</span>
        <svg class="ym-caret" viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>
      <div class="cal-stats">
        {#if !loading}
          <span class="stat-num">{events.length}</span>
          <span class="stat-label">场</span>
        {/if}
      </div>
    </header>

    <!-- 星期标题 -->
    <div class="dow" role="row">
      {#each DOW as w, i}
        <div
          class="dow-cell"
          class:weekend={i >= 5}
          role="columnheader"
          aria-label={`星期${w}`}
        >{w}</div>
      {/each}
    </div>

    {#if loading}
      <div class="grid skeleton-grid">
        {#each Array(35) as _}
          <div class="cell blank-skel"></div>
        {/each}
      </div>
    {:else}
      <div class="grid" role="grid">
        {#each cells as d, i (i)}
          {#if d === null}
            <div class="cell empty" aria-hidden="true"></div>
          {:else}
            {@const evs = byDay[d] ?? []}
            {@const hasToday = isToday(d)}
            {@const isWeekend = (i % 7) >= 5}
            {@const showCount = Math.min(evs.length, MAX_SHOW)}
            {@const overflow = evs.length > MAX_SHOW ? evs.length - MAX_SHOW : 0}
            {@const layout = posterLayout(showCount)}
            <button
              type="button"
              class="cell"
              class:today={hasToday}
              class:weekend={isWeekend}
              class:has-event={evs.length > 0}
              role="gridcell"
              aria-label={`${year}年${month}月${d}日${evs.length ? '，' + evs.length + '场演出' : ''}`}
              onclick={() => onCellClick(d)}
            >
              <!-- 背景海报层：填满整个格子 -->
              {#if evs.length}
                <div
                  class="poster-grid"
                  style="grid-template-columns: repeat({layout.cols}, 1fr); grid-template-rows: repeat({layout.rows}, 1fr);"
                >
                  {#each evs.slice(0, MAX_SHOW) as e, idx (e.id)}
                    <span
                      class="p-slot"
                      class:dimmed={e.active_status}
                      class:span-full={(layout.spans?.[idx] ?? 0) === 1}
                      title={e.name}
                    >
                      {#if posterSrc(e)}
                        <img src={posterSrc(e)} alt="" loading="lazy" />
                      {:else}
                        <span class="p-ph">{e.name?.[0] ?? '?'}</span>
                      {/if}
                    </span>
                  {/each}
                </div>
                {#if overflow}
                  <span class="more-badge">+{overflow}</span>
                {/if}
              {/if}

              <!-- 日期徽章：始终覆盖在格子左上角 -->
              <span class="d-badge" class:today-badge={hasToday} class:weekend-badge={isWeekend && !evs.length}>
                {d}
                {#if hasToday && !evs.length}<span class="today-dot"></span>{/if}
              </span>
            </button>
          {/if}
        {/each}
      </div>
    {/if}
  </section>

  <p class="hint">点击日期查看当天演出 · 点击空白日期可新增</p>
</div>

{#if modalDay !== null}
  {@const evs = byDay[modalDay] ?? []}
  <div class="mask" role="presentation" onclick={() => (modalDay = null)} transition:fade={{ duration: 180 }}>
    <div
      class="modal card"
      role="dialog"
      aria-modal="true"
      aria-label={`${month}月${modalDay}日`}
      onclick={(e) => e.stopPropagation()}
      transition:scale={{ duration: 200, start: 0.94 }}
    >
      <header class="modal-head">
        <div class="modal-date">
          <span class="modal-month">{month}月</span>
          <span class="modal-day">{modalDay}</span>
        </div>
        <div class="modal-sub">
          {#if evs.length}
            <span class="modal-sub-text">共 {evs.length} 场演出</span>
          {:else}
            <span class="modal-sub-text">暂无演出</span>
          {/if}
        </div>
      </header>

      {#if evs.length}
        <ul class="day-list" role="list">
          {#each evs as e (e.id)}
            <li class="day-item" role="listitem">
              <a class="day-link" href={`/records/${e.id}`}>
                {#if posterSrc(e)}
                  <img src={posterSrc(e)} alt="" loading="lazy" width="52" height="70" />
                {:else}
                  <span class="diph">{e.name?.[0] ?? '?'}</span>
                {/if}
                <div class="di-body">
                  <div class="di-top">
                    <span class="di-name">{e.name}</span>
                    {#if statusLabel(e.active_status)}
                      <span class="di-status {statusClass(e.active_status)}">{statusLabel(e.active_status)}</span>
                    {/if}
                  </div>
                  <div class="di-meta">
                    <span class="di-time">{timeStr(e.date)}</span>
                    {#if e.category_name}
                      <span class="di-tag {categoryColor(e.category_name)}">{e.category_name}</span>
                    {/if}
                    {#if e.city}<span class="di-city">{e.city}</span>{/if}
                  </div>
                  {#if e.address}<div class="di-addr">{e.address}</div>{/if}
                </div>
              </a>
            </li>
          {/each}
        </ul>
      {:else}
        <div class="empty-state">
          <svg viewBox="0 0 24 24" width="40" height="40" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <rect x="3" y="4" width="18" height="18" rx="2" />
            <path d="M3 9h18M8 2v4M16 2v4" />
          </svg>
          <p>这一天还没有演出记录</p>
        </div>
      {/if}

      <footer class="modal-foot">
        <button class="btn ghost sm" onclick={() => (modalDay = null)} type="button">关闭</button>
        <button class="btn primary sm" onclick={() => openNew(modalDay)} type="button">+ 新增演出</button>
      </footer>
    </div>
  </div>
{/if}

{#if showYearPicker}
  <div
    class="mask"
    role="presentation"
    onclick={() => (showYearPicker = false)}
    transition:fade={{ duration: 180 }}
  >
    <div
      class="year-modal card"
      role="dialog"
      aria-modal="true"
      aria-label="选择年份与月份"
      onclick={(e) => e.stopPropagation()}
      transition:scale={{ duration: 200, start: 0.94 }}
    >
      <header class="ym-head">
        <button class="nav-btn" type="button" onclick={() => shiftYear(-1)} aria-label="上一年">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M15 18l-6-6 6-6" />
          </svg>
        </button>
        <span class="ym-title">{pickerYear} 年</span>
        <button class="nav-btn" type="button" onclick={() => shiftYear(1)} aria-label="下一年">
          <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M9 18l6-6-6-6" />
          </svg>
        </button>
      </header>
      <div class="month-grid">
        {#each Array(12) as _, i (i)}
          <button
            class="month-cell"
            class:current={pickerYear === year && i + 1 === month}
            type="button"
            onclick={() => selectMonth(i + 1)}
            aria-label={`${pickerYear} 年 ${i + 1} 月`}
            aria-current={pickerYear === year && i + 1 === month ? 'true' : undefined}
          >{i + 1} 月</button>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  /* ============ 外层卡片 ============ */
  .calendar-card {
    padding: 0;
    overflow: hidden;
    border-radius: var(--radius-xl);
  }

  /* ============ 头部导航 ============ */
  .cal-header {
    display: flex;
    align-items: center;
    padding: 18px 20px 14px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, var(--surface-2) 0%, var(--surface) 100%);
    gap: 16px;
  }

  .nav-group {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .nav-btn {
    width: 36px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-2);
    cursor: pointer;
    transition: background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease);
  }
  .nav-btn:hover {
    background: var(--accent-soft);
    color: var(--accent);
    border-color: var(--accent-soft);
  }
  .nav-btn:active { transform: scale(0.94); }

  .today-btn {
    height: 32px;
    padding: 0 14px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    color: var(--text-2);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease);
  }
  .today-btn:hover {
    background: var(--accent-soft);
    color: var(--accent);
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .ym-display {
    flex: 1;
    display: flex;
    align-items: baseline;
    gap: 6px;
    justify-content: center;
    font-family: var(--font-serif);
    background: transparent;
    border: none;
    margin: 0;
    padding: 4px 10px;
    border-radius: var(--radius-sm);
    color: inherit;
    font-size: inherit;
    font-weight: inherit;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease);
  }
  .ym-display:hover {
    background: var(--accent-soft);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }
  .ym-display:active { transform: scale(0.97); }
  .ym-caret {
    align-self: center;
    color: var(--text-3);
    transition: transform var(--t-fast) var(--ease), color var(--t-fast) var(--ease);
  }
  .ym-display:hover .ym-caret {
    color: var(--accent);
    transform: translateY(1px);
  }
  .ym-display .y {
    font-size: 15px;
    font-weight: 500;
    color: var(--text-3);
    letter-spacing: 0.08em;
  }
  .ym-display .sep {
    color: var(--border-strong);
    font-size: 18px;
  }
  .ym-display .m {
    font-size: 24px;
    font-weight: 700;
    color: var(--text);
    letter-spacing: 0.04em;
  }

  .cal-stats {
    display: flex;
    align-items: baseline;
    gap: 3px;
    flex-shrink: 0;
  }
  .stat-num {
    font-family: var(--font-serif);
    font-size: 22px;
    font-weight: 700;
    color: var(--accent);
    line-height: 1;
  }
  .stat-label {
    font-size: 12px;
    color: var(--text-3);
    letter-spacing: 0.04em;
  }

  /* ============ 年历选择弹窗 ============ */
  .year-modal {
    width: min(360px, 100%);
    padding: 0;
    border-radius: var(--radius-xl);
    overflow: hidden;
  }
  .ym-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 18px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, var(--surface-2) 0%, var(--surface) 100%);
  }
  .ym-title {
    font-family: var(--font-serif);
    font-size: 18px;
    font-weight: 700;
    color: var(--text);
    letter-spacing: 0.04em;
  }
  .month-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
    padding: 18px;
  }
  .month-cell {
    padding: 15px 0;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    color: var(--text-2);
    font-family: var(--font-serif);
    font-size: 15px;
    font-weight: 600;
    line-height: 1;
    cursor: pointer;
    transition: background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease);
  }
  .month-cell:hover {
    border-color: var(--accent);
    color: var(--accent);
    background: var(--accent-softer);
    box-shadow: 0 2px 8px -2px var(--accent-soft);
    transform: translateY(-1px);
  }
  .month-cell:active { transform: translateY(0) scale(0.97); }
  .month-cell.current {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
    box-shadow: 0 2px 8px -2px var(--accent);
  }
  .month-cell.current:hover {
    background: var(--accent);
    color: #fff;
  }

  /* ============ 星期标题 ============ */
  .dow {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    padding: 10px 12px 4px;
  }
  .dow-cell {
    text-align: center;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-3);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }
  .dow-cell.weekend { color: var(--gold); }

  /* ============ 日期网格 ============ */
  .grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 5px;
    padding: 6px 12px 14px;
  }

  .cell {
    aspect-ratio: 1 / 1;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface);
    display: block;
    text-align: left;
    cursor: pointer;
    position: relative;
    overflow: hidden;
    font: inherit;
    padding: 0;
    transition: border-color var(--t-fast) var(--ease),
      box-shadow var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease);
  }
  .cell:hover:not(.empty) {
    border-color: var(--accent);
    box-shadow: 0 2px 10px -2px var(--accent-soft);
    transform: translateY(-1px);
  }
  .cell:active:not(.empty) { transform: translateY(0) scale(0.98); }

  .cell.empty {
    background: transparent;
    border-color: transparent;
    cursor: default;
  }

  .cell.blank-skel {
    aspect-ratio: 1 / 1;
    border: none;
    border-radius: var(--radius);
    background-image: linear-gradient(90deg, var(--surface-3) 25%, var(--surface-2) 50%, var(--surface-3) 75%);
    background-size: 800px 100%;
    animation: shimmer 1.4s ease infinite;
  }

  /* ============ 海报填充层 ============ */
  .poster-grid {
    position: absolute;
    inset: 0;
    display: grid;
    gap: 2px;
    background: var(--surface-2);
  }
  .p-slot {
    position: relative;
    display: block;
    overflow: hidden;
    background: var(--surface-3);
  }
  .p-slot.span-full {
    grid-row: 1 / span 2;
  }
  .p-slot img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
    transition: transform 0.3s var(--ease);
  }
  .cell:hover .p-slot img {
    transform: scale(1.04);
  }
  .p-slot.dimmed img {
    filter: grayscale(0.65) opacity(0.55);
  }
  .p-ph {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    font-family: var(--font-serif);
    font-size: clamp(14px, 3.5vw, 22px);
    font-weight: 700;
    color: var(--text-3);
    background: linear-gradient(135deg, var(--surface-3), var(--surface-2));
  }

  /* 海报间微分隔线 */
  .p-slot + .p-slot {
    box-shadow: inset 1px 0 0 var(--surface);
  }
  .poster-grid[style*="grid-template-rows"] .p-slot {
    box-shadow: none;
  }

  /* ============ 日期徽章（叠层左上角） ============ */
  .d-badge {
    position: absolute;
    top: 5px;
    left: 5px;
    z-index: 2;
    font-family: var(--font-serif);
    font-size: 13px;
    font-weight: 700;
    line-height: 1;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: 5px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 3px;
    background: var(--surface);
    color: var(--text-2);
    border: 1px solid var(--border);
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.12);
    letter-spacing: 0;
  }
  /* 有海报时日期徽章更紧凑、更透明 */
  .has-event .d-badge {
    font-size: 11px;
    min-width: 15px;
    height: 15px;
    padding: 0 3px;
    border-radius: 4px;
    background: rgba(255, 255, 255, 0.9);
    backdrop-filter: blur(2px);
    border: none;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.18);
  }
  :global(.dark) .has-event .d-badge {
    background: rgba(28, 25, 23, 0.88);
    color: var(--text);
  }

  /* 今日徽章 */
  .d-badge.today-badge {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
    box-shadow: 0 2px 6px -1px var(--accent);
  }
  .has-event .d-badge.today-badge {
    background: var(--accent);
    color: #fff;
    border: none;
    box-shadow: 0 2px 6px -1px var(--accent);
  }

  /* 周末纯文本日期 */
  .d-badge.weekend-badge {
    color: var(--gold);
  }

  .today-dot {
    width: 4px;
    height: 4px;
    border-radius: 50%;
    background: var(--accent);
    margin-left: 1px;
  }

  /* 有海报时格子的整体暗化遮罩，让日期徽章更清晰 */
  .has-event::after {
    content: '';
    position: absolute;
    inset: 0;
    pointer-events: none;
    background: linear-gradient(180deg, rgba(0, 0, 0, 0.12) 0%, transparent 30%);
    z-index: 1;
  }
  /* 但 hover 时稍微去掉遮罩让海报更亮 */
  .cell:hover.has-event::after {
    background: linear-gradient(180deg, rgba(0, 0, 0, 0.06) 0%, transparent 25%);
  }

  /* 溢出角标：超过 MAX_SHOW 的场次 */
  .more-badge {
    position: absolute;
    z-index: 3;
    bottom: 4px;
    right: 4px;
    padding: 2px 6px;
    border-radius: 999px;
    background: rgba(28, 25, 23, 0.82);
    backdrop-filter: blur(4px);
    color: #fff;
    font-size: 10px;
    font-weight: 600;
    line-height: 1;
    letter-spacing: 0.02em;
    box-shadow: 0 1px 4px rgba(0, 0, 0, 0.25);
  }

  /* ============ 提示文字 ============ */
  .hint {
    margin-top: 14px;
    text-align: center;
    font-size: 12.5px;
    color: var(--text-3);
  }

  /* ============ Modal ============ */
  .mask {
    position: fixed;
    inset: 0;
    background: rgba(20, 17, 15, 0.5);
    backdrop-filter: blur(4px);
    z-index: 60;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 20px;
  }
  .modal {
    width: min(440px, 100%);
    padding: 0;
    border-radius: var(--radius-xl);
    overflow: hidden;
  }

  .modal-head {
    padding: 22px 24px 16px;
    border-bottom: 1px solid var(--border);
    background: linear-gradient(180deg, var(--accent-softer) 0%, var(--surface) 100%);
    text-align: center;
  }
  .modal-date {
    display: inline-flex;
    align-items: baseline;
    gap: 4px;
    font-family: var(--font-serif);
  }
  .modal-month {
    font-size: 14px;
    color: var(--text-3);
    font-weight: 500;
    letter-spacing: 0.04em;
  }
  .modal-day {
    font-size: 36px;
    font-weight: 700;
    color: var(--accent);
    line-height: 1;
  }
  .modal-sub {
    margin-top: 6px;
  }
  .modal-sub-text {
    font-size: 13px;
    color: var(--text-3);
  }

  .day-list {
    list-style: none;
    margin: 0;
    padding: 12px 16px;
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-height: min(50vh, 380px);
    overflow-y: auto;
    overscroll-behavior: contain;
  }

  .day-item { margin: 0; padding: 0; }
  .day-link {
    display: flex;
    align-items: stretch;
    gap: 12px;
    padding: 10px;
    border-radius: var(--radius);
    background: var(--surface-2);
    transition: background var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease);
    border: 1px solid transparent;
  }
  .day-link:hover {
    background: var(--accent-softer);
    border-color: var(--accent-soft);
    transform: translateX(3px);
  }
  .day-link img, .diph {
    width: 44px;
    aspect-ratio: 3 / 4;
    object-fit: cover;
    border-radius: 6px;
    flex-shrink: 0;
  }
  .diph {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--surface-3);
    color: var(--text-3);
    font-size: 18px;
    font-weight: 600;
  }

  .di-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .di-top {
    display: flex;
    align-items: flex-start;
    gap: 6px;
  }
  .di-name {
    font-size: 14.5px;
    font-weight: 600;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .di-status {
    font-size: 11px;
    font-weight: 500;
    padding: 2px 7px;
    border-radius: 999px;
    line-height: 1.2;
    flex-shrink: 0;
  }
  .di-status.status-wish {
    background: var(--gold-soft);
    color: var(--gold);
  }
  .di-status.status-cancel {
    background: var(--danger-soft);
    color: var(--danger);
  }
  .di-status.status-miss {
    background: var(--surface-3);
    color: var(--text-muted);
  }

  .di-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-3);
    flex-wrap: wrap;
  }
  .di-time {
    font-weight: 500;
    color: var(--text-2);
  }
  .di-tag {
    font-size: 10.5px;
    padding: 1px 6px;
    border-radius: 4px;
    font-weight: 500;
  }
  .di-addr {
    font-size: 11.5px;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* 分类颜色变体（柔和色调） */
  .cat-default { background: var(--surface-3); color: var(--text-muted); }
  .cat-0  { background: rgba(180, 35, 24, 0.08); color: var(--accent); }
  .cat-1  { background: rgba(217, 119, 6, 0.10); color: var(--gold); }
  .cat-2  { background: rgba(21, 128, 61, 0.08); color: #15803d; }
  .cat-3  { background: rgba(59, 130, 246, 0.10); color: #3b82f6; }
  .cat-4  { background: rgba(147, 51, 234, 0.08); color: #9333ea; }
  .cat-5  { background: rgba(236, 72, 153, 0.10); color: #ec4899; }
  .cat-6  { background: rgba(249, 115, 22, 0.10); color: #f97316; }
  .cat-7  { background: rgba(14, 165, 233, 0.10); color: #0ea5e9; }
  .cat-8  { background: rgba(168, 85, 247, 0.10); color: #a855f7; }
  .cat-9  { background: rgba(34, 197, 94, 0.10); color: #22c55e; }
  .cat-10 { background: rgba(244, 63, 94, 0.10); color: #f43f5e; }
  .cat-11 { background: rgba(20, 184, 166, 0.10); color: #14b8a6; }

  .empty-state {
    padding: 32px 24px;
    text-align: center;
    color: var(--text-3);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
  .empty-state svg { opacity: 0.5; color: var(--text-3); }
  .empty-state p {
    margin: 0;
    font-size: 14px;
    color: var(--text-2);
  }

  .modal-foot {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 14px 20px;
    border-top: 1px solid var(--border);
    background: var(--surface-2);
  }

  /* ============ 响应式 ============ */
  @media (max-width: 640px) {
    .cal-header {
      padding: 14px 14px 12px;
      gap: 10px;
    }
    .nav-btn { width: 32px; height: 32px; }
    .today-btn { height: 28px; padding: 0 10px; font-size: 12px; }
    .ym-display { gap: 4px; }
    .ym-display .y { font-size: 13px; }
    .ym-display .m { font-size: 20px; }
    .stat-num { font-size: 18px; }
    .stat-label { font-size: 11px; }

    .dow { padding: 8px 8px 2px; }
    .dow-cell { font-size: 11px; letter-spacing: 0.08em; }

    .grid { gap: 3px; padding: 4px 8px 12px; }
    .cell { border-radius: 8px; }
    .d-badge {
      top: 3px; left: 3px;
      font-size: 11px;
      min-width: 15px; height: 15px;
      padding: 0 3px;
    }
    .has-event .d-badge {
      font-size: 10px;
      min-width: 13px; height: 13px;
      padding: 0 2px;
    }
    .more-badge { font-size: 9px; padding: 1px 5px; bottom: 3px; right: 3px; }

    .modal-head { padding: 18px 18px 14px; }
    .modal-day { font-size: 30px; }
    .day-list { padding: 10px 12px; }
    .day-link img, .diph { width: 38px; }
    .di-name { font-size: 14px; }
    .modal-foot { padding: 12px 14px; }
  }

  @media (max-width: 420px) {
    .ym-display .y { display: none; }
    .ym-display .sep { display: none; }
    .ym-display .m { font-size: 22px; }
    .month-grid { gap: 6px; padding: 14px; }
    .month-cell { padding: 13px 0; font-size: 14px; }
  }
</style>
