<script>
  // Grouped bar chart for year-over-year comparison: current vs previous.
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
</script>

<svg viewBox="0 0 {W} {height}" width="100%" preserveAspectRatio="xMidYMid meet" role="img">
  <line x1={padX} y1={padTop + plotH} x2={W - padX} y2={padTop + plotH} stroke="var(--border)" stroke-width="1" />
  <!-- max value label -->
  <text x={padX - 4} y={padTop + 3} text-anchor="end" class="y-label">{max}{unit}</text>
  {#each data as d, i}
    {@const cx = padX + slot * i + slot / 2}
    <rect x={cx - pairW / 2} y={y(d.current || 0)} width={barW} height={Math.max(padTop + plotH - y(d.current || 0), 0.5)} rx="2" fill="var(--accent)">
      <title>{d.label}: 今年 {d.current}{unit}</title>
    </rect>
    <rect x={cx + 1.5} y={y(d.previous || 0)} width={barW} height={Math.max(padTop + plotH - y(d.previous || 0), 0.5)} rx="2" fill="var(--text-3)" opacity="0.55">
      <title>{d.label}: 去年同期 {d.previous}{unit}</title>
    </rect>
    {#if i % labelEvery === 0}
      <text x={cx} y={height - 9} text-anchor="middle" class="x-label">{d.label}</text>
    {/if}
  {/each}
</svg>

<style>
  svg { display: block; }
  .y-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .x-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
</style>
