<script>
  // Grouped bar chart for year-over-year comparison: current vs previous.
  // Hover tooltips are shown via a floating layer (see `tip` state).
  let { data = [], height = 200, labelEvery = 2, unit = '' } = $props();

  const W = 760;
  const padTop = 16;
  const padBottom = 28;
  const padX = 10;

  let max = $derived(Math.max(1, ...data.flatMap((d) => [d.current || 0, d.previous || 0])));
  let plotH = $derived(height - padTop - padBottom);
  let slot = $derived((W - padX * 2) / Math.max(data.length, 1));
  let pairW = $derived(Math.min(slot * 0.7, 30));
  let barW = $derived(pairW / 2 - 1.5);

  function y(v) {
    return padTop + plotH - (v / max) * plotH;
  }

  // ---- floating tooltip ----
  let tip = $state(null);
  function showTip(e, title, lines) {
    tip = { x: e.clientX, y: e.clientY, title, lines };
  }
  function hideTip() {
    tip = null;
  }
</script>

<svg viewBox="0 0 {W} {height}" width="100%" preserveAspectRatio="xMidYMid meet" role="img">
  <line x1={padX} y1={padTop + plotH} x2={W - padX} y2={padTop + plotH} stroke="var(--border)" stroke-width="1" />
  <!-- max value label -->
  <text x={padX - 4} y={padTop + 3} text-anchor="end" class="y-label">{max}{unit}</text>
  {#each data as d, i}
    {@const cx = padX + slot * i + slot / 2}
    <rect x={cx - pairW / 2} y={y(d.current || 0)} width={barW} height={Math.max(padTop + plotH - y(d.current || 0), 0.5)} rx="2" fill="var(--accent)" pointer-events="none" />
    <rect x={cx + 1.5} y={y(d.previous || 0)} width={barW} height={Math.max(padTop + plotH - y(d.previous || 0), 0.5)} rx="2" fill="var(--text-3)" opacity="0.55" pointer-events="none" />
    <rect
      x={cx - pairW / 2} y={padTop} width={pairW} height={plotH}
      fill="transparent" style="pointer-events:all;cursor:pointer"
      role="img" aria-label={`${d.label}: 今年 ${d.current}${unit}, 去年同期 ${d.previous}${unit}`}
      onmouseenter={(e) => showTip(e, d.label, [`今年 ${d.current}${unit}`, `去年同期 ${d.previous}${unit}`])}
      onmousemove={(e) => showTip(e, d.label, [`今年 ${d.current}${unit}`, `去年同期 ${d.previous}${unit}`])}
      onmouseleave={hideTip}
    />
    {#if i % labelEvery === 0}
      <text x={cx} y={height - 9} text-anchor="middle" class="x-label">{d.label}</text>
    {/if}
  {/each}
</svg>

{#if tip}
  <div class="chart-tip" style="left:{tip.x + 12}px; top:{tip.y + 12}px">
    <div class="t-title">{tip.title}</div>
    {#each tip.lines as l}<div class="t-line">{l}</div>{/each}
  </div>
{/if}

<style>
  svg { display: block; }
  .y-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .x-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .chart-tip {
    position: fixed; z-index: 60; pointer-events: none;
    background: var(--surface-2); border: 1px solid var(--border);
    border-radius: 8px; padding: 7px 10px; font-size: 12px; color: var(--text);
    box-shadow: var(--shadow-md); max-width: 240px;
  }
  .chart-tip .t-title { font-weight: 700; margin-bottom: 2px; }
  .chart-tip .t-line { color: var(--text-2); font-variant-numeric: tabular-nums; }
</style>
