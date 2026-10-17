<script>
  import ChartTip from './ChartTip.svelte';
  // Grouped bar chart for year-over-year comparison: current vs previous.
  // Hover tooltips are shown via a floating layer (see `tip` state).
  let { data = [], height = 200, labelEvery = 2, unit = '' } = $props();

  // 让 SVG 坐标系宽度等于容器实际像素宽度，避免等比缩放导致文字不可读
  let wrap = $state(null);
  let containerW = $state(400);
  $effect(() => {
    if (!wrap) return;
    const ro = new ResizeObserver((entries) => {
      containerW = Math.round(entries[0].contentRect.width);
    });
    ro.observe(wrap);
    return () => ro.disconnect();
  });

  let innerMinW = 320;
  let slot = $derived(Math.max(60, (containerW - 60) / Math.max(data.length, 1)));
  let W = $derived(Math.max(innerMinW, containerW));
  const padTop = 16;
  const padBottom = 28;
  const padX = 30;   /* 给 y 轴数字留出空间 */

  let max = $derived(Math.max(1, ...data.flatMap((d) => [d.current || 0, d.previous || 0])));
  let plotH = $derived(height - padTop - padBottom);
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

<div bind:this={wrap} class="chart-wrap"><svg viewBox="0 0 {W} {height}" width="100%" preserveAspectRatio="none" aria-label="对比柱状图表">
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
</svg></div>

<ChartTip {tip} />

<style>
  .chart-wrap { width: 100%; overflow-x: auto; }
  svg { display: block; width: 100%; height: auto; }
  .y-label { font-size: 11px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .x-label { font-size: 11px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  svg rect[tabindex]:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; border-radius: 2px; }
</style>
