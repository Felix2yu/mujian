<script>
  import { page } from '$app/state';
  import { api, formatCurrency } from '#lib/api.js';
  import KpiCard from '#lib/components/analytics/KpiCard.svelte';
  import Donut from '#lib/components/analytics/Donut.svelte';
  import VBarChart from '#lib/components/analytics/VBarChart.svelte';
  import LineChart from '#lib/components/analytics/LineChart.svelte';
  import RankList from '#lib/components/analytics/RankList.svelte';

  let report = $state(null);
  let loading = $state(true);
  let error = $state('');

  let year = $derived(page.params.year);

  $effect(() => {
    const y = year;
    if (!y) return;
    load(y);
  });

  async function load(y) {
    loading = true;
    error = '';
    try {
      report = await api.getYearlyReport(y);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function fmtPct(v) {
    if (!v) return '0%';
    return (v > 0 ? '+' : '') + Math.round(v * 10) / 10 + '%';
  }
  function fmtDelta(v) {
    if (!v) return '0';
    return (v > 0 ? '+' : '−') + Math.abs(v).toFixed(1);
  }

  let monthBars = $derived((report?.monthly ?? []).map((t) => ({ label: t.period.slice(5) + '月', value: t.count })));
  let monthLabels = $derived((report?.monthly ?? []).map((t) => t.period.slice(5) + '月'));
  let costSeries = $derived([
    { name: '月度花费', color: 'var(--gold)', values: (report?.monthly ?? []).map((t) => Math.round(t.cost)) }
  ]);
  let ratingSeries = $derived([
    { name: '平均评分', color: '#0e7490', values: (report?.monthly ?? []).map((t) => t.avg_rating) }
  ]);
  let ratingBars = $derived((report?.rating_dist ?? []).map((d) => ({ label: d.name, value: d.count })));
  let weekdayBars = $derived((report?.weekday_dist ?? []).map((w) => ({ label: w.name, value: w.count })));
  let priceBars = $derived((report?.price_buckets ?? []).map((d) => ({ label: d.name, value: d.count })));

  let venueHref = (it) => `/?address=${encodeURIComponent(it.name)}`;
  let artistHref = (it) => `/artists/${it.id}`;
  let dramaHref = (it) => `/dramas/${it.id}`;
  let zheziHref = (it) => `/?zhezi=${encodeURIComponent(it.id)}`;
  let showHref = (ref) => (ref ? `/records/${ref.id}` : '#');

  // 年度总结文案：只由已有数据拼出，缺项自动跳过。
  let summary = $derived.by(() => {
    if (!report || !report.overview.total_records) return '';
    const o = report.overview;
    const parts = [`这一年你看了 ${o.total_records} 场演出，`];
    parts.push(`在路上花了 ${formatCurrency(o.total_cost, 'CNY')}${o.cost_delta_pct ? `（${o.cost_delta_pct > 0 ? '多花' : '少花'} ${Math.abs(o.cost_delta_pct)}%）` : ''}，`);
    if (report.highlights.peak_month) {
      parts.push(`最忙的是 ${Number(report.highlights.peak_month.slice(5))} 月（${report.highlights.peak_month_count} 场），`);
    }
    const topDrama = (report.top_dramas ?? [])[0];
    if (topDrama) parts.push(`看得最多的剧目是《${topDrama.name}》（${topDrama.count} 场），`);
    if (o.new_artists || o.new_dramas) parts.push(`还初遇了 ${o.new_artists} 位演员、${o.new_dramas} 部剧目，`);
    if (report.holiday.shows) parts.push(`其中 ${report.holiday.shows} 场是在假期里度过的，`);
    parts.push(o.avg_rating ? `年平均评分 ${o.avg_rating.toFixed(1)}★。` : '期待明年继续。');
    return parts.join('');
  });
</script>

<svelte:head><title>{year} 年报 - 幕间</title></svelte:head>

<div class="fade-up">
  <div class="page-head">
    <div>
      <h1>📄 {year} 年度观演报告</h1>
      <p class="sub">自然年视角回顾你的观演一年</p>
    </div>
    <a class="back" href="/analytics">← 返回分析</a>
  </div>

  {#if loading}
    <div class="skeleton" style="height: 120px;"></div>
    <div class="skeleton" style="height: 300px; margin-top: 14px;"></div>
  {:else if error}
    <div class="banner error">⚠ {error}</div>
  {:else if report}
    {#if report.available_years.length > 1}
      <div class="years">
        {#each report.available_years as y}
          <a class="year-chip" class:cur={y === report.year} href="/analytics/{y}">{y}</a>
        {/each}
      </div>
    {/if}

    {#if summary}
      <div class="card sec summary">✍️ {summary}</div>
    {/if}

    {#if report.overview.total_records === 0}
      <div class="card sec"><p class="tiny">{year} 年没有观演记录。</p></div>
    {:else}
      <!-- ============ KPI ============ -->
      <div class="kpis stagger">
        <KpiCard icon="🎫" label="观演场次" value={report.overview.total_records} deltaText={fmtPct(report.overview.records_delta_pct)} deltaTone={report.overview.records_delta_pct >= 0 ? 'pos' : 'neg'} sub="同比上年" />
        <KpiCard icon="💰" label="总花费" value={formatCurrency(report.overview.total_cost, 'CNY')} deltaText={fmtPct(report.overview.cost_delta_pct)} deltaTone={report.overview.cost_delta_pct >= 0 ? 'pos' : 'neg'} sub="同比上年" />
        <KpiCard icon="★" label="平均评分" value={report.overview.avg_rating ? report.overview.avg_rating.toFixed(1) : '—'} deltaText={fmtDelta(report.overview.rating_delta)} deltaTone={report.overview.rating_delta >= 0 ? 'pos' : 'neg'} />
        <KpiCard icon="📍" label="走过城市" value={report.overview.total_cities} />
        <KpiCard icon="⏱" label="观演时长" value={report.overview.total_hours ? `${report.overview.total_hours} h` : '—'} />
        <KpiCard icon="🔭" label="新发现" value={`${report.overview.new_artists}+${report.overview.new_dramas}`} sub="演员 + 剧目首遇" />
      </div>

      <!-- ============ 年度亮点 ============ -->
      <section class="block">
        <h2 class="sec-title">✨ 年度亮点</h2>
        <div class="hl-grid">
          {#if report.highlights.peak_month}
            <div class="card hl">
              <div class="hl-k">最活跃月份</div>
              <div class="hl-v">{Number(report.highlights.peak_month.slice(5))} 月</div>
              <div class="hl-s">{report.highlights.peak_month_count} 场</div>
            </div>
          {/if}
          {#if report.highlights.best_rated}
            <a class="card hl" href={showHref(report.highlights.best_rated)}>
              <div class="hl-k">最高分一场</div>
              <div class="hl-v">{report.highlights.best_rated.name}</div>
              <div class="hl-s">{report.highlights.best_rated.value}★ · {report.highlights.best_rated.date_text}</div>
            </a>
          {/if}
          {#if report.highlights.most_expensive}
            <a class="card hl" href={showHref(report.highlights.most_expensive)}>
              <div class="hl-k">最贵一票</div>
              <div class="hl-v">{report.highlights.most_expensive.name}</div>
              <div class="hl-s">{formatCurrency(report.highlights.most_expensive.value, 'CNY')} · {report.highlights.most_expensive.date_text}</div>
            </a>
          {/if}
          {#if report.highlights.first_show}
            <a class="card hl" href={showHref(report.highlights.first_show)}>
              <div class="hl-k">年度开幕</div>
              <div class="hl-v">{report.highlights.first_show.name}</div>
              <div class="hl-s">{report.highlights.first_show.date_text}</div>
            </a>
          {/if}
          {#if report.highlights.last_show}
            <a class="card hl" href={showHref(report.highlights.last_show)}>
              <div class="hl-k">年度收官</div>
              <div class="hl-v">{report.highlights.last_show.name}</div>
              <div class="hl-s">{report.highlights.last_show.date_text}</div>
            </a>
          {/if}
          {#if report.highlights.longest_gap_days > 0}
            <div class="card hl">
              <div class="hl-k">年内最长空窗</div>
              <div class="hl-v">{report.highlights.longest_gap_days} 天</div>
              <div class="hl-s">两场之间最久的等待</div>
            </div>
          {/if}
        </div>
      </section>

      <!-- ============ 全年排名 ============ -->
      <section class="block">
        <h2 class="sec-title">🏅 历年坐标</h2>
        <div class="card sec">
          <div class="stat-row">
            <div class="stat">
              <div class="v">第 {report.rank.count_rank} / {report.rank.years_with_shows} 名</div>
              <div class="l">场次在有你记录的所有年份中的位置</div>
            </div>
            <div class="stat">
              <div class="v">第 {report.rank.cost_rank} 名</div>
              <div class="l">花费历年排名</div>
            </div>
            {#if report.rank.is_record_count}
              <div class="stat"><div class="v">🎉</div><div class="l">场次创历史新高</div></div>
            {/if}
            {#if report.rank.is_record_cost}
              <div class="stat"><div class="v">💸</div><div class="l">花费创历史新高</div></div>
            {/if}
          </div>
          <div class="pct-track"><div class="pct-fill" style="width:{report.rank.count_percentile}%"></div></div>
          <p class="tiny">活跃度百分位 {report.rank.count_percentile}% —— 超过你 {report.rank.years_with_shows} 年观演史中的 {report.rank.count_percentile}% 年份。</p>
        </div>
      </section>

      <!-- ============ 月度趋势 ============ -->
      <section class="block">
        <h2 class="sec-title">📈 全年节奏</h2>
        <div class="card sec">
          <h3>月度场次</h3>
          <VBarChart data={monthBars} height={200} labelEvery={1} unit=" 场" />
        </div>
        <div class="grid-2">
          <div class="card sec">
            <h3>月度花费</h3>
            <LineChart labels={monthLabels} series={costSeries} height={190} yMin={0} labelEvery={1} unit="元" />
          </div>
          <div class="card sec">
            <h3>月度平均评分</h3>
            <LineChart labels={monthLabels} series={ratingSeries} height={190} yMin={0} yMax={5} labelEvery={1} unit="★" />
          </div>
        </div>
      </section>

      <!-- ============ 分布 ============ -->
      <section class="block">
        <h2 class="sec-title">🥧 这一年</h2>
        <div class="grid-2">
          <div class="card sec">
            <h3>剧种占比</h3>
            <Donut items={report.category_dist} max={8} />
          </div>
          <div class="card sec">
            <h3>城市占比</h3>
            <Donut items={report.city_dist} max={8} />
          </div>
          <div class="card sec">
            <h3>评分分布</h3>
            <VBarChart data={ratingBars} height={170} labelEvery={1} unit=" 场" color="var(--gold)" />
          </div>
          <div class="card sec">
            <h3>观演星期分布</h3>
            <VBarChart data={weekdayBars} height={170} labelEvery={1} unit=" 场" color="#0e7490" />
          </div>
          <div class="card sec">
            <h3>票价分布</h3>
            {#if report.price_buckets.length === 0}
              <p class="tiny">暂无票价记录</p>
            {:else}
              <VBarChart data={priceBars} height={170} labelEvery={1} unit=" 场" />
            {/if}
          </div>
          {#if report.holiday.shows > 0}
            <div class="card sec">
              <h3>假期观演</h3>
              <div class="stat-row">
                <div class="stat"><div class="v">{report.holiday.shows} 场</div><div class="l">占全年 {report.holiday.pct}%</div></div>
              </div>
              <ul class="hol">
                {#each report.holiday.by_holiday as h}
                  <li><span>{h.name}</span><b>{h.count} 场</b></li>
                {/each}
              </ul>
            </div>
          {:else}
            <div class="card sec">
              <h3>假期观演</h3>
              <p class="tiny">这一年没有在国家法定节假日里看演出，或者节假日数据尚未录入。</p>
            </div>
          {/if}
        </div>
      </section>

      <!-- ============ 年度排行 ============ -->
      <section class="block">
        <h2 class="sec-title">🏆 年度高频</h2>
        <div class="grid-2">
          <div class="card sec">
            <h3>演员 Top 10</h3>
            <RankList items={report.top_artists} hrefFn={artistHref} unit="场" />
          </div>
          <div class="card sec">
            <h3>剧目 Top 10</h3>
            <RankList items={report.top_dramas} hrefFn={dramaHref} unit="场" />
          </div>
          <div class="card sec">
            <h3>场馆 Top 10</h3>
            <RankList items={report.top_venues} hrefFn={venueHref} unit="场" />
          </div>
          <div class="card sec">
            <h3>折子 Top 10</h3>
            {#if report.top_zhezis.length === 0}
              <p class="tiny">暂无折子记录</p>
            {:else}
              <RankList items={report.top_zhezis} hrefFn={zheziHref} unit="场" />
            {/if}
          </div>
        </div>
      </section>

      <!-- ============ 年度初遇 ============ -->
      <section class="block">
        <h2 class="sec-title">🔭 年度初遇</h2>
        <div class="grid-2">
          <div class="card sec">
            <h3>第一次看到的演员</h3>
            {#if report.highlights.first_artists.length === 0}
              <p class="tiny">今年没有初遇新演员</p>
            {:else}
              <div class="first">
                {#each report.highlights.first_artists as f}
                  <span class="chip"><span class="fd">{f.date_text.slice(5)}</span>{f.name}</span>
                {/each}
              </div>
            {/if}
          </div>
          <div class="card sec">
            <h3>第一次看到的剧目</h3>
            {#if report.highlights.first_dramas.length === 0}
              <p class="tiny">今年没有初遇新剧目</p>
            {:else}
              <div class="first">
                {#each report.highlights.first_dramas as f}
                  <span class="chip"><span class="fd">{f.date_text.slice(5)}</span>{f.name}</span>
                {/each}
              </div>
            {/if}
          </div>
        </div>
      </section>
    {/if}
  {/if}
</div>

<style>
  .page-head { margin-bottom: 14px; display: flex; justify-content: space-between; align-items: flex-end; gap: 12px; }
  .page-head h1 { margin: 0; font-size: 26px; }
  .sub { color: var(--text-muted); font-size: 13.5px; margin: 4px 0 0; }
  .back { font-size: 13px; color: var(--accent); text-decoration: none; white-space: nowrap; }

  .years { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 14px; }
  .year-chip { padding: 4px 14px; border-radius: 99px; border: 1px solid var(--border); font-size: 13px; text-decoration: none; color: var(--text-2); background: var(--surface); }
  .year-chip.cur { background: var(--accent); border-color: var(--accent); color: #fff; }
  :global(.dark) .year-chip.cur { color: #1a1a1a; }

  .summary { font-size: 14.5px; line-height: 1.9; color: var(--text-2); margin-bottom: 14px; }

  .kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(140px, 1fr)); gap: 12px; }

  .block { margin-top: 22px; }
  .sec-title { font-size: 17px; margin: 0 0 12px; display: flex; align-items: baseline; gap: 10px; }
  .card.sec { padding: 18px 20px; transition: box-shadow 0.2s ease, transform 0.2s ease; }
  .card.sec:hover { box-shadow: var(--shadow-md); transform: translateY(-1px); }
  .card.sec h3 { margin: 0 0 14px; font-size: 15px; }

  .grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; margin-top: 12px; }
  .grid-2:first-of-type { margin-top: 0; }

  .tiny { color: var(--text-muted); font-size: 13px; margin: 0; }

  .hl-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; }
  .card.hl { padding: 14px 16px; text-decoration: none; display: block; }
  .hl-k { font-size: 12px; color: var(--text-muted); }
  .hl-v { font-size: 15.5px; font-weight: 700; color: var(--text); margin: 6px 0 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .hl-s { font-size: 12px; color: var(--text-2); }

  .stat-row { display: flex; flex-wrap: wrap; gap: 20px; margin-bottom: 10px; }
  .stat .v { font-size: 20px; font-weight: 700; color: var(--text); font-variant-numeric: tabular-nums; }
  .stat .l { font-size: 12px; color: var(--text-muted); margin-top: 2px; }

  .pct-track { height: 8px; border-radius: 99px; background: var(--surface-3); overflow: hidden; margin: 6px 0 8px; }
  .pct-fill { height: 100%; border-radius: 99px; background: linear-gradient(90deg, var(--accent), var(--gold)); }

  .hol { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; font-size: 13px; }
  .hol li { display: flex; justify-content: space-between; color: var(--text-2); }

  .first { display: flex; flex-wrap: wrap; gap: 6px; }
  .chip { display: inline-flex; align-items: baseline; gap: 5px; padding: 3px 10px; border-radius: 99px; border: 1px solid var(--border); background: var(--surface-2); font-size: 12.5px; color: var(--text); white-space: nowrap; }
  .fd { color: var(--text-muted); font-size: 11px; font-variant-numeric: tabular-nums; }

  @media (max-width: 860px) {
    .grid-2 { grid-template-columns: 1fr; }
  }
</style>
