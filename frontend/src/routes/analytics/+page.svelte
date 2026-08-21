<script>
  import { onMount } from 'svelte';
  import { api, formatCurrency, coverUrl } from '$lib/api.js';

  let stats = $state(null);
  let loading = $state(true);
  let error = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      const data = await api.getDashboard();
      stats = {
        ...data,
        by_category: data.by_category ?? [],
        by_city: data.by_city ?? [],
        by_month: data.by_month ?? [],
        top_rated: data.top_rated ?? [],
        recent_records: data.recent_records ?? []
      };
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function maxOf(arr, key) {
    return Math.max(1, ...arr.map((x) => x[key] || 0));
  }

  onMount(load);
</script>

<div class="fade-up">
  <div class="page-head">
    <h1>分析</h1>
    <p class="sub">观演数据统计与趋势</p>
  </div>

  {#if loading}
    <div class="kpis">
      {#each Array(4) as _}<div class="skeleton" style="height: 96px;"></div>{/each}
    </div>
    <div class="skeleton" style="height: 260px; margin-top: 14px;"></div>
  {:else if error}
    <div class="banner error">⚠ {error}</div>
  {:else if stats}
    <div class="kpis stagger">
      <div class="card kpi">
        <div class="kpi-ico">🎫</div>
        <div class="kpi-num">{stats.total_records}</div>
        <div class="kpi-label">总记录</div>
      </div>
      <div class="card kpi">
        <div class="kpi-ico">💰</div>
        <div class="kpi-num">{formatCurrency(stats.total_cost, 'CNY')}</div>
        <div class="kpi-label">总花费</div>
      </div>
      <div class="card kpi">
        <div class="kpi-ico gold">★</div>
        <div class="kpi-num">{stats.avg_rating ? stats.avg_rating.toFixed(1) : '—'}</div>
        <div class="kpi-label">平均评分</div>
      </div>
      <div class="card kpi">
        <div class="kpi-ico">📍</div>
        <div class="kpi-num">{stats.total_cities}</div>
        <div class="kpi-label">走过城市</div>
      </div>
    </div>

    <div class="cols">
      <div class="card sec">
        <h3>按分类</h3>
        {#if stats.by_category.length === 0}<p class="tiny">暂无数据</p>{/if}
        {#each stats.by_category.slice(0, 8) as c}
          <a class="bar-row" href={`/?category=${encodeURIComponent(c.name)}`}>
            <span class="bl">{c.name}</span>
            <span class="bar-track"><span class="bar" style={`width: ${(c.count / maxOf(stats.by_category, 'count')) * 100}%`}></span></span>
            <span class="bc">{c.count}</span>
          </a>
        {/each}
      </div>
      <div class="card sec">
        <h3>按城市</h3>
        {#if stats.by_city.length === 0}<p class="tiny">暂无数据</p>{/if}
        {#each stats.by_city.slice(0, 8) as c}
          <a class="bar-row" href={`/?city=${encodeURIComponent(c.name)}`}>
            <span class="bl">{c.name}</span>
            <span class="bar-track"><span class="bar city" style={`width: ${(c.count / maxOf(stats.by_city, 'count')) * 100}%`}></span></span>
            <span class="bc">{c.count}</span>
          </a>
        {/each}
      </div>
    </div>

    <div class="cols">
      <div class="card sec">
        <h3>高分记录</h3>
        {#if !stats.top_rated?.length}<p class="tiny">暂无评分记录</p>{/if}
        <div class="rec-list">
          {#each stats.top_rated ?? [] as r}
            <a class="rec-row" href={`/records/${r.id}`}>
              <span class="thumb">
                {#if r.coverThumb}<img src={coverUrl(r.coverThumb)} alt="" />{:else if r.coverFile}<img src={coverUrl(r.coverFile)} alt="" />{:else}<span class="no-img">{(r.name || '?').slice(0, 1)}</span>{/if}
              </span>
              <span class="rr-main">
                <span class="rt">{r.name}</span>
                <span class="rm tiny">{r.categoryName}{r.dateText ? ' · ' + r.dateText.split(' ')[0] : ''}</span>
              </span>
              <span class="stars">{'★'.repeat(r.rating)}</span>
            </a>
          {/each}
        </div>
      </div>
      <div class="card sec">
        <h3>最近记录</h3>
        {#if !stats.recent_records?.length}<p class="tiny">暂无记录</p>{/if}
        <div class="rec-list">
          {#each stats.recent_records ?? [] as r}
            <a class="rec-row" href={`/records/${r.id}`}>
              <span class="thumb">
                {#if r.coverThumb}<img src={coverUrl(r.coverThumb)} alt="" />{:else if r.coverFile}<img src={coverUrl(r.coverFile)} alt="" />{:else}<span class="no-img">{(r.name || '?').slice(0, 1)}</span>{/if}
              </span>
              <span class="rr-main">
                <span class="rt">{r.name}</span>
                <span class="rm tiny">{r.city}{r.dateText ? ' · ' + r.dateText.split(' ')[0] : ''}</span>
              </span>
              {#if r.rating}<span class="stars">{'★'.repeat(r.rating)}</span>{/if}
            </a>
          {/each}
        </div>
      </div>
    </div>

    <div class="card sec">
      <h3>近 12 个月</h3>
      {#if !stats.by_month?.length}
        <p class="tiny">近 12 个月暂无记录</p>
      {:else}
        <div class="month-chart">
          {#each stats.by_month as m}
            <div class="mcol" title={`${m.month}：${m.count} 场`}>
              <span class="mbar" style={`height: ${(m.count / maxOf(stats.by_month, 'count')) * 100}%`}></span>
              <span class="mlabel">{m.month.slice(5)}</span>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <a class="btn" href={api.getICSUrl()}>⇩ 导出日历 (.ics)</a>
  {/if}
</div>

<style>
  .kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; }
  .kpi { padding: 16px; text-align: center; position: relative; }
  .kpi-ico { font-size: 20px; margin-bottom: 6px; }
  .kpi-ico.gold { color: var(--gold); }
  .kpi-num { font-size: 24px; font-weight: 700; color: var(--text); font-variant-numeric: tabular-nums; }
  .kpi-label { font-size: 12.5px; color: var(--text-muted); margin-top: 2px; }

  .cols { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 14px; }
  .sec { padding: 18px 20px; }
  .sec h3 { margin: 0 0 14px; font-size: 15.5px; }
  .card.sec:not(.cols .sec) { margin-top: 14px; }

  .bar-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 0;
    font-size: 13.5px;
    transition: opacity var(--t-fast) var(--ease);
  }
  .bar-row:hover { opacity: 0.75; }
  .bl { width: 72px; flex: 0 0 auto; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .bar-track { flex: 1; height: 8px; border-radius: 99px; background: var(--surface-3); overflow: hidden; }
  .bar { display: block; height: 100%; border-radius: 99px; background: linear-gradient(90deg, var(--accent), var(--accent-strong)); transition: width 0.6s var(--ease); }
  .bar.city { background: linear-gradient(90deg, var(--gold), #b45309); }
  .bc { width: 30px; text-align: right; color: var(--text-muted); font-variant-numeric: tabular-nums; }

  .rec-list { display: flex; flex-direction: column; }
  .rec-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 0;
    border-bottom: 1px solid var(--border);
    transition: background var(--t-fast) var(--ease);
  }
  .rec-row:last-child { border-bottom: none; }
  .rec-row:hover { background: var(--accent-softer); }
  .thumb {
    width: 34px;
    height: 46px;
    border-radius: 6px;
    overflow: hidden;
    background: var(--surface-3);
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .thumb img { width: 100%; height: 100%; object-fit: cover; }
  .no-img { font-family: var(--font-serif); color: var(--text-3); font-size: 16px; }
  .rr-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .rt { font-size: 14px; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .stars { color: var(--gold); font-size: 12px; letter-spacing: 1px; flex: 0 0 auto; }

  .month-chart { display: flex; align-items: flex-end; gap: 6px; height: 140px; padding-top: 8px; }
  .mcol { flex: 1; display: flex; flex-direction: column; align-items: center; gap: 6px; height: 100%; justify-content: flex-end; }
  .mbar {
    width: 100%;
    max-width: 36px;
    border-radius: 6px 6px 0 0;
    background: linear-gradient(180deg, var(--accent), var(--accent-strong));
    min-height: 3px;
    transition: height 0.6s var(--ease);
  }
  .mlabel { font-size: 11px; color: var(--text-muted); }

  @media (max-width: 700px) { .cols { grid-template-columns: 1fr; } }
</style>
