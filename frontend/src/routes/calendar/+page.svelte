<script>
  import { onMount } from 'svelte';
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

  async function load() {
    loading = true;
    error = '';
    try {
      events = await api.getCalendar(year, month);
      const u = new URL(location.href);
      u.searchParams.set('y', String(year));
      u.searchParams.set('m', String(month));
      history.replaceState({}, '', u);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
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
  const MAX_SHOW = 3;

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
    location.href = `/records/new?date=${dateStr(d)}`;
  }

  function posterSrc(e) {
    return coverUrl(e.coverThumb || e.coverFile || '');
  }

  const STATUS_LABEL = { 1: '想看', 2: '已取消', 3: '未赴约' };
  const statusLabel = (s) => STATUS_LABEL[s] || '';

  // 统一入口：点击任意日期弹出当日弹窗（有演出→选择，无演出→新增确认）
  function onCellClick(d) {
    modalDay = d;
  }

  function timeStr(ts) {
    const d = new Date(ts * 1000);
    return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  onMount(load);
</script>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape') modalDay = null; }} />
<svelte:head><title>日历 - 幕间</title></svelte:head>

<div class="fade-up">
  <div class="page-head">
    <h1>日历</h1>
  </div>

  <div class="toolbar">
    <button class="btn ghost nav" onclick={() => shift(-1)} aria-label="上个月">←</button>
    <button class="btn ghost nav today-btn" onclick={goToday}>今天</button>
    <div class="ym">{year} 年 {month} 月</div>
    <button class="btn ghost nav" onclick={() => shift(1)} aria-label="下个月">→</button>
    {#if !loading}<span class="count">{events.length} 场</span>{/if}
  </div>

  {#if error}<div class="banner error">⚠ {error}</div>{/if}

  <div class="dow">
    {#each DOW as w, i}<div class:weekend={i >= 5}>{w}</div>{/each}
  </div>

  {#if loading}
    <div class="empty">加载中…</div>
  {:else}
    <div class="grid">
      {#each cells as d, i (i)}
        {#if d === null}
          <div class="cell blank"></div>
        {:else}
          {@const evs = byDay[d] ?? []}
          <div
            class="cell"
            class:today={isToday(d)}
            role="button"
            tabindex="-1"
            onclick={() => onCellClick(d)}
          >
            <div class="dnum" class:weekend={(i % 7) >= 5}>{d}</div>
            {#if evs.length}
              <div class="posters">
                {#each evs.slice(0, MAX_SHOW) as e (e.id)}
                  <span class="plink" title={e.name}>
                    {#if posterSrc(e)}
                      <img class="pimg" class:dimmed={e.active_status} src={posterSrc(e)} alt={e.name} loading="lazy" />
                    {:else}
                      <span class="ph">{e.name?.[0] ?? '?'}</span>
                    {/if}
                    {#if e === evs[MAX_SHOW - 1] && evs.length > MAX_SHOW}
                      <span class="more">+{evs.length - MAX_SHOW}</span>
                    {/if}
                  </span>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <p class="hint">点击日期查看当天演出；点击空白日期可新增该日演出。</p>
</div>

{#if modalDay !== null}
  {@const evs = byDay[modalDay] ?? []}
  <div class="mask" role="presentation" onclick={() => (modalDay = null)}>
    <div class="modal card" role="dialog" aria-modal="true">
      {#if evs.length}
        <h3>{month} 月 {modalDay} 日 · {evs.length} 场</h3>
        <div class="day-list">
          {#each evs as e (e.id)}
            <a class="day-item" href={`/records/${e.id}`}>
              {#if posterSrc(e)}
                <img src={posterSrc(e)} alt="" loading="lazy" />
              {:else}
                <span class="diph">{e.name?.[0] ?? '?'}</span>
              {/if}
              <span class="di-info">
                <span class="di-name">{e.name}</span>
                <span class="di-meta">{timeStr(e.date)}{e.city ? ` · ${e.city}` : ''}{e.address ? ` · ${e.address}` : ''}</span>
              </span>
              {#if statusLabel(e.active_status)}
                <span class="di-status">{statusLabel(e.active_status)}</span>
              {/if}
            </a>
          {/each}
        </div>
        <div class="actions">
          <button class="btn ghost sm" onclick={() => (modalDay = null)}>关闭</button>
          <button class="btn primary sm" onclick={() => openNew(modalDay)}>+ 新增该日演出</button>
        </div>
      {:else}
        <h3>{month} 月 {modalDay} 日暂无演出</h3>
        <p>要新增这一天的演出记录吗？</p>
        <div class="actions">
          <button class="btn ghost" onclick={() => (modalDay = null)}>取消</button>
          <button class="btn primary" onclick={() => openNew(modalDay)}>新增演出</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .toolbar {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 10px;
    margin-bottom: 14px;
  }
  .nav {
    font-size: 17px;
    min-width: 40px;
    height: 40px;
    padding: 0 12px;
  }
  .ym {
    font-size: 24px;
    font-weight: 700;
    font-family: var(--font-serif);
    letter-spacing: 1px;
    padding: 0 6px;
    white-space: nowrap;
  }
  .count {
    position: absolute;
    right: 0;
    color: var(--text-3);
    font-size: 13px;
  }

  .dow, .grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 5px;
  }
  .dow { margin-bottom: 5px; }
  .dow div {
    text-align: center;
    font-size: 12px;
    color: var(--text-3);
    padding: 2px 0;
  }
  .dow .weekend { color: var(--gold); }

  .cell {
    aspect-ratio: 4 / 5;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    transition: border-color var(--t-fast) var(--ease), box-shadow var(--t-fast) var(--ease);
  }
  .cell:not(.blank) { cursor: pointer; }
  .cell:not(.blank):hover {
    border-color: var(--accent);
    box-shadow: var(--shadow-xs);
  }
  .cell.blank {
    background: transparent;
    border-color: transparent;
  }
  .cell.today {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent-soft);
  }

  .dnum {
    padding: 4px 7px 2px;
    font-size: 12.5px;
    font-weight: 600;
    color: var(--text-2);
    flex-shrink: 0;
  }
  .dnum.weekend { color: var(--gold); }
  .today .dnum { color: var(--accent); }

  .posters {
    flex: 1;
    min-height: 0;
    display: flex;
    gap: 2px;
    padding: 0 4px 4px;
  }
  .plink {
    flex: 1;
    min-width: 0;
    position: relative;
    display: block;
    border-radius: 4px;
    overflow: hidden;
    background: var(--surface-2);
  }
  .pimg {
    width: 100%;
    height: 100%;
    aspect-ratio: 3 / 4;
    object-fit: cover;
    display: block;
  }
  .pimg.dimmed { filter: grayscale(0.75); opacity: 0.55; }
  .ph {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: clamp(11px, 1.4vw, 16px);
    color: var(--text-3);
    background: var(--surface-3, var(--surface-2));
  }
  .more {
    position: absolute;
    right: 3px;
    bottom: 3px;
    background: rgba(20, 20, 24, 0.72);
    color: #fff;
    font-size: 10.5px;
    line-height: 1;
    padding: 3px 5px;
    border-radius: 999px;
  }

  .hint {
    margin-top: 14px;
    text-align: center;
    font-size: 12.5px;
    color: var(--text-3);
  }

  .mask {
    position: fixed;
    inset: 0;
    background: rgba(15, 15, 18, 0.45);
    backdrop-filter: blur(2px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 60;
  }
  .modal {
    width: min(400px, calc(100vw - 48px));
    padding: 22px 24px;
  }
  .modal h3 { margin: 0 0 12px; font-size: 16.5px; font-family: var(--font-serif); }
  .modal p { margin: 0 0 18px; color: var(--text-2); font-size: 14px; }
  .actions { display: flex; justify-content: flex-end; gap: 8px; }

  .day-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 16px;
    max-height: min(52vh, 420px);
    overflow-y: auto;
  }
  .day-item {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 8px;
    border-radius: var(--radius-sm);
    background: var(--surface-2, var(--bg));
    transition: background var(--t-fast) var(--ease), transform var(--t-fast) var(--ease);
  }
  .day-item:hover { background: var(--accent-soft); transform: translateX(2px); }
  .day-item img, .diph {
    width: 42px;
    aspect-ratio: 3 / 4;
    object-fit: cover;
    border-radius: 4px;
    flex-shrink: 0;
  }
  .diph {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--surface-3, var(--surface-2));
    color: var(--text-3);
    font-size: 17px;
  }
  .di-info { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
  .di-name {
    font-size: 14px;
    font-weight: 600;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .di-meta {
    font-size: 12px;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .di-status { margin-left: auto; flex-shrink: 0; font-size: 11.5px; color: var(--gold); }

  @media (max-width: 640px) {
    .cell { aspect-ratio: 3 / 4; }
    .dnum { padding: 2px 4px 0; font-size: 11px; }
    .posters { padding: 0 2px 2px; gap: 1px; }
  }
</style>
