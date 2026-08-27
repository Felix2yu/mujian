<script>
  import { onMount } from 'svelte';
  import { api, formatCurrency } from '$lib/api.js';
  import KpiCard from '$lib/components/analytics/KpiCard.svelte';
  import Donut from '$lib/components/analytics/Donut.svelte';
  import VBarChart from '$lib/components/analytics/VBarChart.svelte';
  import LineChart from '$lib/components/analytics/LineChart.svelte';
  import CompareBars from '$lib/components/analytics/CompareBars.svelte';
  import ScatterChart from '$lib/components/analytics/ScatterChart.svelte';
  import RankList from '$lib/components/analytics/RankList.svelte';

  let data = $state(null);
  let loading = $state(true);
  let error = $state('');

  async function load() {
    loading = true;
    error = '';
    try {
      data = await api.getAnalytics();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  onMount(load);

  // ---- derived view-models ----
  let anomalyMap = $derived.by(() => {
    const m = {};
    for (const a of data?.anomalies ?? []) m[a.period] = a.type;
    return m;
  });

  let monthBars = $derived(
    (data?.trends ?? []).map((t) => ({
      label: t.period.slice(2),
      value: t.count,
      highlight: anomalyMap[t.period]
    }))
  );
  let costSeries = $derived([
    { name: '月度花费', color: 'var(--gold)', values: (data?.trends ?? []).map((t) => Math.round(t.cost)) }
  ]);
  let ratingSeries = $derived([
    { name: '平均评分', color: '#0e7490', values: (data?.trends ?? []).map((t) => t.avgRating) }
  ]);
  let monthLabels = $derived((data?.trends ?? []).map((t) => t.period.slice(2)));

  let compareData = $derived(
    (data?.compare_monthly ?? []).map((c) => ({
      label: c.period.slice(2),
      current: c.current,
      previous: c.previous
    }))
  );
  let yoy = $derived.by(() => {
    const cs = compareData.reduce((s, c) => s + c.current, 0);
    const ps = compareData.reduce((s, c) => s + c.previous, 0);
    const pct = ps > 0 ? ((cs - ps) / ps) * 100 : cs > 0 ? 100 : 0;
    return { cs: Math.round(cs), ps: Math.round(ps), pct: Math.round(pct * 10) / 10 };
  });

  let ratingBars = $derived((data?.rating_dist ?? []).map((d) => ({ label: d.name, value: d.count })));
  let yearBars = $derived((data?.year_dist ?? []).map((d) => ({ label: d.name, value: d.count })));

  function corrStrength(r) {
    const a = Math.abs(r);
    if (a < 0.2) return '极弱';
    if (a < 0.4) return '弱';
    if (a < 0.6) return '中等';
    if (a < 0.8) return '强';
    return '极强';
  }
  function corrTone(r) {
    if (r > 0) return 'pos';
    if (r < 0) return 'neg';
    return 'flat';
  }

  function fmtPct(v) {
    if (v === 0) return '0%';
    return (v > 0 ? '+' : '') + Math.round(v * 10) / 10 + '%';
  }
  function fmtDelta(v) {
    if (v === 0) return '0';
    return (v > 0 ? '+' : '−') + Math.abs(v).toFixed(1);
  }

  let venueHref = (it) => `/?address=${encodeURIComponent(it.name)}`;
  let artistHref = (it) => `/artists/${it.id}`;
  let dramaHref = (it) => `/dramas/${it.id}`;

  // ---- new dimensions derived ----
  let priceBars = $derived((data?.price_buckets ?? []).map((d) => ({ label: d.name, value: d.count })));
  let otherCostBars = $derived((data?.other_cost_buckets ?? []).map((d) => ({ label: d.name, value: d.count })));
  let weekdayBars = $derived((data?.weekday_dist ?? []).map((w) => ({ label: w.name, value: w.count })));
  let intervalBars = $derived((data?.intervals?.buckets ?? []).map((d) => ({ label: d.name, value: d.count })));
  let discoveryLabels = $derived((data?.discovery ?? []).map((d) => d.period.slice(2)));
  let discoverySeries = $derived([
    { name: '新演员', color: 'var(--accent)', values: (data?.discovery ?? []).map((d) => d.new_artists) },
    { name: '新剧目', color: 'var(--gold)', values: (data?.discovery ?? []).map((d) => d.new_dramas) }
  ]);
  let rewatch = $derived(data?.rewatch);
  let diversity = $derived(data?.diversity);
  let intervals = $derived(data?.intervals);
  function pct1(v) { return Math.round(v * 1000) / 10; }
</script>

<svelte:head><title>分析 - 幕间</title></svelte:head>

<div class="fade-up">
  <div class="page-head">
    <h1>分析</h1>
    <p class="sub">从趋势、占比、对比、异常、相关性多个视角解读你的观演数据</p>
  </div>

  {#if loading}
    <div class="kpis">
      {#each Array(6) as _}<div class="skeleton" style="height: 120px;"></div>{/each}
    </div>
    <div class="skeleton" style="height: 280px; margin-top: 14px;"></div>
  {:else if error}
    <div class="banner error">⚠ {error}</div>
  {:else if data}
    <!-- ============ KPIs ============ -->
    <div class="kpis stagger">
      <KpiCard icon="🎫" label="总记录" value={data.overview.total_records} deltaText={fmtPct(data.overview.records_delta_pct)} deltaTone={data.overview.records_delta_pct >= 0 ? 'pos' : 'neg'} />
      <KpiCard icon="💰" label="总花费" value={formatCurrency(data.overview.total_cost, 'CNY')} sub="近一年" deltaText={fmtPct(data.overview.cost_delta_pct)} deltaTone="flat" />
      <KpiCard icon="★" label="平均评分" value={data.overview.avg_rating ? data.overview.avg_rating.toFixed(1) : '—'} deltaText={fmtDelta(data.overview.rating_delta)} deltaTone={data.overview.rating_delta >= 0 ? 'pos' : 'neg'} />
      <KpiCard icon="📍" label="走过城市" value={data.overview.total_cities} />
      <KpiCard icon="🎭" label="演员" value={data.overview.total_artists} sub="已建档" />
      <KpiCard icon="📚" label="剧目" value={data.overview.total_dramas} sub="已建档" />
    </div>

    <!-- ============ 趋势变化 ============ -->
    <section class="block">
      <h2 class="sec-title">📈 趋势变化 <span class="hint">近 24 个月</span></h2>
      <div class="card sec">
        <h3>月度场次</h3>
        <VBarChart data={monthBars} height={200} labelEvery={2} unit=" 场" />
      </div>
      <div class="grid-2">
        <div class="card sec">
          <h3>月度花费</h3>
          <LineChart labels={monthLabels} series={costSeries} height={190} yMin={0} labelEvery={2} unit="元" />
        </div>
        <div class="card sec">
          <h3>月度平均评分</h3>
          <LineChart labels={monthLabels} series={ratingSeries} height={190} yMin={0} yMax={5} labelEvery={2} unit="★" />
        </div>
      </div>
    </section>

    <!-- ============ 异常波动 ============ -->
    <section class="block">
      <h2 class="sec-title">⚡ 异常波动</h2>
      {#if data.anomalies.length === 0}
        <div class="card sec"><p class="tiny">近 24 个月场次分布平稳，未检测到显著异常（z-score &gt; 1.5）。</p></div>
      {:else}
        <div class="card sec">
          <div class="anomaly-grid">
            {#each data.anomalies as a}
              <div class="anom {a.type}">
                <span class="badge">{a.type === 'spike' ? '尖峰' : '骤降'}</span>
                <span class="ap">{a.period}</span>
                <span class="ac">{a.count} 场</span>
                <span class="aexp">预期 {a.expected} · z={a.zscore}</span>
              </div>
            {/each}
          </div>
          <p class="note">基于 24 个月场次序列的 z-score 检测（均值 ± 1.5 标准差）。尖峰/骤降可能对应巡演、戏剧节等集中观演时段。</p>
        </div>
      {/if}
    </section>

    <!-- ============ 占比分布 ============ -->
    <section class="block">
      <h2 class="sec-title">🥧 占比分布</h2>
      <div class="grid-2">
        <div class="card sec">
          <h3>剧种占比</h3>
          <Donut items={data.category_dist} max={8} />
        </div>
        <div class="card sec">
          <h3>购票渠道占比</h3>
          <Donut items={data.channel_dist} max={8} />
        </div>
        <div class="card sec">
          <h3>剧团占比</h3>
          <Donut items={data.company_dist} max={8} />
        </div>
        <div class="card sec">
          <h3>城市占比</h3>
          <Donut items={data.city_dist} max={8} />
        </div>
      </div>
      <div class="grid-2" style="margin-top:14px;">
        <div class="card sec">
          <h3>评分分布</h3>
          <VBarChart data={ratingBars} height={180} labelEvery={1} unit=" 场" color="var(--gold)" />
        </div>
        <div class="card sec">
          <h3>年度分布</h3>
          <VBarChart data={yearBars} height={180} labelEvery={1} unit=" 场" color="#0e7490" />
        </div>
      </div>
    </section>

    <!-- ============ 对比差异 ============ -->
    <section class="block">
      <h2 class="sec-title">🔍 对比差异 <span class="hint">近 12 个月 vs 去年同期</span></h2>
      <div class="card sec">
        <div class="compare-head">
          <span>今年同期累计 <b>{yoy.cs}</b> 场，去年同期 <b>{yoy.ps}</b> 场，同比 <b class="delta {yoy.pct >= 0 ? 'pos' : 'neg'}">{fmtPct(yoy.pct)}</b></span>
        </div>
        <CompareBars data={compareData} height={210} labelEvery={2} unit=" 场" />
        <div class="legend-compare">
          <span><i class="sw cur"></i>今年</span>
          <span><i class="sw prev"></i>去年同期</span>
        </div>
      </div>
    </section>

    <!-- ============ 相关性 ============ -->
    <section class="block">
      <h2 class="sec-title">🔗 相关性 <span class="hint">Pearson 系数</span></h2>
      <div class="grid-2">
        <div class="card sec">
          <h3>票价 × 评分 散点</h3>
          <ScatterChart points={data.scatter} xLabel="实付票价（元）" yLabel="评分 ★" yMax={5} height={260} />
        </div>
        <div class="card sec">
          <h3>数值字段相关性</h3>
          <ul class="corr">
            {#each data.corr_pairs as p}
              <li>
                <span class="cp-name">{p.x} × {p.y}</span>
                {#if p.n < 3}
                  <span class="cp-na">样本不足（{p.n}）</span>
                {:else}
                  <span class="cp-bar"><span class="cp-fill {corrTone(p.r)}" style="width:{Math.abs(p.r) * 100}%"></span></span>
                  <span class="cp-r {corrTone(p.r)}">r={p.r} <em>{corrStrength(p.r)}</em></span>
                {/if}
              </li>
            {/each}
          </ul>
          <p class="note">相关不等于因果。样本量 n 见各散点/系数旁标注。</p>
        </div>
      </div>
    </section>

    <!-- ============ 排行 ============ -->
    <section class="block">
      <h2 class="sec-title">🏆 高频排行 <span class="hint">按观演场次</span></h2>
      <div class="grid-3">
        <div class="card sec">
          <h3>演员 Top 10</h3>
          <RankList items={data.top_artists} hrefFn={artistHref} unit="场" />
        </div>
        <div class="card sec">
          <h3>剧目 Top 10</h3>
          <RankList items={data.top_dramas} hrefFn={dramaHref} unit="场" />
        </div>
        <div class="card sec">
          <h3>场馆 Top 10</h3>
          <RankList items={data.top_venues} hrefFn={venueHref} unit="场" />
        </div>
      </div>
    </section>

    <!-- ============ 行为模式 ============ -->
    <section class="block">
      <h2 class="sec-title">🧭 行为模式</h2>

      <div class="card sec">
        <h3>观演星期分布</h3>
        <VBarChart data={weekdayBars} height={180} labelEvery={1} unit=" 场" color="#0e7490" />
      </div>

      <div class="grid-2" style="margin-top:12px;">
        <div class="card sec">
          <h3>复看率 <span class="hint">看 ≥2 次的比例</span></h3>
          {#if rewatch}
            <div class="stat-row">
              <div class="stat"><div class="v">{rewatch.drama_rate}%</div><div class="l">剧目 {rewatch.rewatched_dramas}/{rewatch.total_dramas}</div></div>
              <div class="stat"><div class="v">{rewatch.artist_rate}%</div><div class="l">演员 {rewatch.rewatched_artists}/{rewatch.total_artists}</div></div>
            </div>
          {/if}
        </div>
        <div class="card sec">
          <h3>多样性指数 <span class="hint">0–100，越高越均衡</span></h3>
          {#if diversity}
            <div class="stat-row">
              <div class="stat"><div class="v">{pct1(diversity.category_evenness)}%</div><div class="l">剧种</div></div>
              <div class="stat"><div class="v">{pct1(diversity.artist_evenness)}%</div><div class="l">演员</div></div>
              <div class="stat"><div class="v">{pct1(diversity.drama_evenness)}%</div><div class="l">剧目</div></div>
            </div>
          {/if}
        </div>
      </div>

      <div class="card sec" style="margin-top:12px;">
        <h3>观演间隔 <span class="hint">相邻两场之间天数</span></h3>
        {#if intervals}
          <div class="stat-row">
            <div class="stat"><div class="v">{intervals.avg} 天</div><div class="l">平均</div></div>
            <div class="stat"><div class="v">{intervals.median} 天</div><div class="l">中位</div></div>
            <div class="stat"><div class="v">{intervals.max} 天</div><div class="l">最长</div></div>
          </div>
          <VBarChart data={intervalBars} height={170} labelEvery={1} unit=" 次" color="var(--gold)" />
        {/if}
      </div>
    </section>

    <!-- ============ 票价与剧目结构 ============ -->
    <section class="block">
      <h2 class="sec-title">💸 票价与剧目结构 <span class="hint">实付票价 / 其他花费 分桶分布</span></h2>
      <div class="grid-3">
        <div class="card sec">
          <h3>实付票价分布</h3>
          {#if data.price_buckets.length === 0}
            <p class="tiny">暂无实付票价记录</p>
          {:else}
            <VBarChart data={priceBars} height={200} labelEvery={1} unit=" 场" color="var(--gold)" />
          {/if}
        </div>
        <div class="card sec">
          <h3>其他花费分布</h3>
          {#if (data.other_cost_buckets ?? []).length === 0}
            <p class="tiny">暂无其他花费记录</p>
          {:else}
            <VBarChart data={otherCostBars} height={200} labelEvery={1} unit=" 场" color="#0e7490" />
          {/if}
        </div>
        <div class="card sec">
          <h3>常看折子 Top 10</h3>
          {#if data.top_zhezis.length === 0}
            <p class="tiny">暂无折子记录</p>
          {:else}
            <RankList items={data.top_zhezis} unit="场" />
          {/if}
        </div>
      </div>
    </section>

    <!-- ============ 探索与发现 ============ -->
    <section class="block">
      <h2 class="sec-title">🔭 探索与发现 <span class="hint">每月首次出现的演员 / 剧目</span></h2>
      <div class="card sec">
        {#if data.discovery.length === 0}
          <p class="tiny">暂无足够数据计算发现率</p>
        {:else}
          <LineChart labels={discoveryLabels} series={discoverySeries} height={220} yMin={0} labelEvery={3} unit=" 个" />
          <div class="legend-compare">
            <span><i class="sw" style="background:var(--accent)"></i>新演员</span>
            <span><i class="sw" style="background:var(--gold)"></i>新剧目</span>
          </div>
        {/if}
      </div>
    </section>

    <p class="footer-note">分析维度：趋势（24 个月序列）· 占比（计数归一化）· 对比（同比栏状）· 异常（z-score &gt; 1.5）· 相关性（Pearson r）· 行为（周几 / 复看率 / 多样性 / 间隔）· 经济（票价分布）· 探索（每月新发现的演员与剧目）。所有数据均来自本地演出记录。</p>
  {/if}
</div>

<style>
  .page-head { margin-bottom: 14px; }
  .page-head h1 { margin: 0; font-size: 26px; }
  .sub { color: var(--text-muted); font-size: 13.5px; margin: 4px 0 0; }

  .kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; }
  .kpi { padding: 16px; }

  .block { margin-top: 22px; }
  .sec-title { font-size: 17px; margin: 0 0 12px; display: flex; align-items: baseline; gap: 10px; }
  .sec-title .hint { font-size: 12px; font-weight: 400; color: var(--text-muted); font-family: var(--font-sans, system-ui, sans-serif); }
  .card.sec { padding: 18px 20px; transition: box-shadow 0.2s ease, transform 0.2s ease; }
  .card.sec:hover { box-shadow: var(--shadow-md); transform: translateY(-1px); }
  .card.sec h3 { margin: 0 0 14px; font-size: 15px; }

  .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 12px; }
  .grid-2:first-of-type { margin-top: 0; }
  .grid-3 { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 12px; }

  .tiny { color: var(--text-muted); font-size: 13px; margin: 0; }

  /* anomalies */
  .anomaly-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 10px; }
  .anom { display: flex; flex-direction: column; gap: 2px; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--border); background: var(--surface-2); }
  .anom.spike { border-color: rgba(220,38,38,0.4); }
  .anom.drop { border-color: rgba(37,99,235,0.4); }
  .anom .badge { font-size: 11px; font-weight: 700; align-self: flex-start; padding: 1px 8px; border-radius: 99px; }
  .anom.spike .badge { background: rgba(220,38,38,0.12); color: #dc2626; }
  .anom.drop .badge { background: rgba(37,99,235,0.12); color: #2563eb; }
  .anom .ap { font-weight: 600; font-variant-numeric: tabular-nums; }
  .anom .ac { font-size: 13px; }
  .anom .aexp { font-size: 11.5px; color: var(--text-muted); }

  /* compare */
  .compare-head { font-size: 13.5px; color: var(--text-2); margin-bottom: 10px; }
  .compare-head b { color: var(--text); }
  .delta.pos { color: #16a34a; } .delta.neg { color: #dc2626; }
  :global(.dark) .delta.pos { color: #4ade80; } :global(.dark) .delta.neg { color: #f87171; }
  .legend-compare { display: flex; gap: 16px; margin-top: 8px; font-size: 12px; color: var(--text-2); }
  .legend-compare .sw { width: 12px; height: 8px; border-radius: 2px; display: inline-block; margin-right: 5px; vertical-align: middle; }
  .legend-compare .sw.cur { background: var(--accent); }
  .legend-compare .sw.prev { background: var(--text-3); opacity: 0.55; }

  /* correlation */
  .corr { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 12px; }
  .corr li { display: grid; grid-template-columns: 1fr 90px 110px; align-items: center; gap: 10px; font-size: 13px; }
  .cp-name { color: var(--text-2); }
  .cp-bar { height: 8px; border-radius: 99px; background: var(--surface-3); overflow: hidden; }
  .cp-fill { display: block; height: 100%; border-radius: 99px; }
  .cp-fill.pos { background: linear-gradient(90deg, #16a34a, #15803d); }
  .cp-fill.neg { background: linear-gradient(90deg, #dc2626, #b91c1c); }
  .cp-fill.flat { background: var(--text-3); }
  .cp-r { font-variant-numeric: tabular-nums; font-weight: 600; }
  .cp-r.pos { color: #16a34a; } .cp-r.neg { color: #dc2626; } .cp-r.flat { color: var(--text-muted); }
  :global(.dark) .cp-r.pos { color: #4ade80; } :global(.dark) .cp-r.neg { color: #f87171; }
  .cp-r em { font-style: normal; font-weight: 500; color: var(--text-muted); font-size: 11.5px; margin-left: 3px; }
  .cp-na { color: var(--text-muted); font-size: 12px; }

  .note { font-size: 11.5px; color: var(--text-muted); margin: 12px 0 0; }

  /* behavioural stat cards */
  .stat-row { display: flex; flex-wrap: wrap; gap: 14px; margin-bottom: 8px; }
  .stat { min-width: 88px; }
  .stat .v { font-size: 22px; font-weight: 700; color: var(--text); font-variant-numeric: tabular-nums; }
  .stat .l { font-size: 12px; color: var(--text-muted); margin-top: 2px; }

  .footer-note { margin-top: 28px; font-size: 11.5px; color: var(--text-3); line-height: 1.7; border-top: 1px dashed var(--border); padding-top: 14px; }

  @media (max-width: 860px) {
    .grid-2, .grid-3 { grid-template-columns: 1fr; }
  }
</style>
