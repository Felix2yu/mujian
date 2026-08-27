<script>
  // Multi-series line chart for trend lines (e.g. monthly cost, monthly avg rating).
  let {
    labels = [],
    series = [], // [{ name, color, values: number[] }]
    height = 190,
    yMin = null,
    yMax = null,
    labelEvery = 2,
    unit = ''
  } = $props();

  const W = 760;
  const padL = 44;
  const padR = 12;
  const padTop = 14;
  const padBottom = 26;

  function allVals() {
    const vs = [];
    for (const s of series) for (const v of s.values) if (v != null && !isNaN(v)) vs.push(v);
    return vs;
  }
  let lo = $derived(yMin != null ? yMin : Math.min(0, ...allVals()));
  let hi = $derived(yMax != null ? yMax : Math.max(1, ...allVals()));
  let plotH = $derived(height - padTop - padBottom);
  let plotW = $derived(W - padL - padR);
  let n = $derived(Math.max(labels.length, 1));

  function x(i) {
    return n <= 1 ? padL + plotW / 2 : padL + (plotW * i) / (n - 1);
  }
  function y(v) {
    if (hi === lo) return padTop + plotH / 2;
    return padTop + plotH - ((v - lo) / (hi - lo)) * plotH;
  }
  function path(values) {
    return values
      .map((v, i) => `${i === 0 ? 'M' : 'L'} ${x(i).toFixed(1)} ${y(v).toFixed(1)}`)
      .join(' ');
  }
  let ticks = $derived([lo, (lo + hi) / 2, hi].map((t) => Math.round(t * 100) / 100));
</script>

<div class="line-wrap">
  <div class="legend">
    {#each series as s}
      <span class="lg"><span class="sw" style="background:{s.color}"></span>{s.name}</span>
    {/each}
  </div>
  <svg viewBox="0 0 {W} {height}" width="100%" preserveAspectRatio="xMidYMid meet" role="img">
    <!-- y gridlines + labels -->
    {#each ticks as t, i}
      <line x1={padL} y1={y(t)} x2={W - padR} y2={y(t)} stroke="var(--border)" stroke-width="1" stroke-dasharray={i === 1 ? '0' : '3 3'} opacity="0.7" />
      <text x={padL - 6} y={y(t) + 3} text-anchor="end" class="y-label">{t}{unit}</text>
    {/each}
    <!-- x labels -->
    {#each labels as lbl, i}
      {#if i % labelEvery === 0}
        <text x={x(i)} y={height - 8} text-anchor="middle" class="x-label">{lbl}</text>
      {/if}
    {/each}
    <!-- series -->
    {#each series as s}
      <path d={path(s.values)} fill="none" stroke={s.color} stroke-width="2.2" stroke-linejoin="round" stroke-linecap="round" />
      {#each s.values as v, i}
        <circle cx={x(i)} cy={y(v)} r="2.4" fill={s.color}>
          <title>{labels[i]}: {v}{unit}</title>
        </circle>
      {/each}
    {/each}
  </svg>
</div>

<style>
  .line-wrap { width: 100%; }
  .legend { display: flex; gap: 14px; flex-wrap: wrap; margin-bottom: 4px; }
  .legend .lg { font-size: 12px; color: var(--text-2); display: inline-flex; align-items: center; gap: 6px; }
  .legend .sw { width: 12px; height: 3px; border-radius: 2px; display: inline-block; }
  svg { display: block; }
  .y-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .x-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
</style>
