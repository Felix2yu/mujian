<script>
  // Donut chart for proportion/distribution data. Supports a "其他" bucket
  // for anything beyond `max` slices so the legend stays readable.
  let { items = [], max = 8, size = 200, title = '' } = $props();

  const palette = [
    '#b42318', '#d97706', '#0e7490', '#7c3aed', '#15803d',
    '#be123c', '#0369a1', '#a16207', '#4d7c0f', '#9333ea',
    '#dc2626', '#0891b2'
  ];

  let total = $derived(items.reduce((s, i) => s + (i.count || 0), 0));
  let shown = $derived(items.slice(0, max));
  let others = $derived(items.slice(max).reduce((s, i) => s + (i.count || 0), 0));
  let r = size / 2 - 14;
  let C = 2 * Math.PI * r;

  // Build donut segments with cumulative dash offsets.
  let segments = $derived.by(() => {
    let acc = 0;
    const all = others > 0 ? [...shown, { name: '其他', count: others }] : shown;
    return all.map((it, idx) => {
      const frac = total > 0 ? it.count / total : 0;
      const len = frac * C;
      const seg = { ...it, color: palette[idx % palette.length], len, offset: -acc };
      acc += len;
      return seg;
    });
  });
</script>

<div class="donut-wrap">
  {#if total === 0}
    <div class="empty-state">暂无数据</div>
  {:else}
    <div class="donut-svg">
      <svg viewBox="0 0 {size} {size}" width={size} height={size} role="img" aria-label={title}>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="var(--surface-3)" stroke-width="16" />
        {#each segments as s}
          <circle
            cx={size / 2} cy={size / 2} r={r} fill="none"
            stroke={s.color} stroke-width="16" stroke-linecap="butt"
            stroke-dasharray="{s.len} {C - s.len}"
            stroke-dashoffset={s.offset}
            transform="rotate(-90 {size / 2} {size / 2})"
          >
            <title>{s.name}: {s.count} ({s.pct?.toFixed(1)}%)</title>
          </circle>
        {/each}
        <text x={size / 2} y={size / 2 - 4} text-anchor="middle" class="donut-center-num">{total}</text>
        <text x={size / 2} y={size / 2 + 14} text-anchor="middle" class="donut-center-lbl">总计</text>
      </svg>
    </div>
    <ul class="legend">
      {#each segments as s}
        <li>
          <span class="dot" style="background:{s.color}"></span>
          <span class="lg-name" title={s.name}>{s.name}</span>
          <span class="lg-val">{s.count}<span class="lg-pct">{s.pct?.toFixed(0)}%</span></span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .donut-wrap { display: flex; gap: 16px; align-items: center; flex-wrap: wrap; }
  .donut-svg { flex: 0 0 auto; }
  .donut-center-num { font-size: 22px; font-weight: 700; fill: var(--text); font-variant-numeric: tabular-nums; }
  .donut-center-lbl { font-size: 11px; fill: var(--text-muted); }
  .legend { list-style: none; margin: 0; padding: 0; flex: 1 1 160px; min-width: 0; }
  .legend li { display: flex; align-items: center; gap: 8px; padding: 3px 0; font-size: 13px; }
  .legend .dot { width: 10px; height: 10px; border-radius: 3px; flex: 0 0 auto; }
  .legend .lg-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-2); }
  .legend .lg-val { font-variant-numeric: tabular-nums; color: var(--text); font-weight: 600; display: flex; align-items: baseline; gap: 5px; }
  .legend .lg-pct { font-size: 11px; color: var(--text-muted); font-weight: 500; }
  .empty-state { width: 100%; padding: 42px 10px; text-align: center; color: var(--text-muted); font-size: 13px; }
</style>
