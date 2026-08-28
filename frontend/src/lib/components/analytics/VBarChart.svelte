<script>
  import ChartTip from './ChartTip.svelte';
  // Vertical bar chart with optional anomaly markers. Used for monthly counts,
  // rating histogram and yearly distribution. Renders as a responsive SVG.
  // Hover tooltips are shown via a floating layer (see `tip` state).
  let {
    data = [],
    color = 'var(--accent)',
    height = 180,
    labelEvery = 1,
    maxValue = null,
    unit = ''
  } = $props();

  const padTop = 14;
  const padBottom = 32;
  const padX = 8;
  const padLeft = 40;

  let max = $derived(
    Math.max(maxValue ?? 1, ...data.map((d) => d.value || 0), 1)
  );
  let W = $derived(Math.max(360, data.length * 60 + padLeft + padX));
  let plotH = $derived(height - padTop - padBottom);
  let plotW = $derived(W - padLeft - padX);
  let slot = $derived(plotW / Math.max(data.length, 1));
  let barW = $derived(Math.max(2, Math.min(slot * 0.62, 34)));

  let yTicks = $derived([0, Math.round(max / 2), max]);

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
  <!-- y gridlines + labels -->
  {#each yTicks as t}
    <line x1={padLeft} y1={y(t)} x2={W - padX} y2={y(t)} stroke="var(--border)" stroke-width="1" stroke-dasharray="3 3" opacity="0.5" />
    <text x={padLeft - 6} y={y(t) + 4} text-anchor="end" class="y-label">{t}{unit}</text>
  {/each}
  <!-- baseline -->
  <line x1={padLeft} y1={padTop + plotH} x2={W - padX} y2={padTop + plotH} stroke="var(--border)" stroke-width="1" />
  {#each data as d, i}
    {@const bx = padLeft + slot * i + (slot - barW) / 2}
    {@const top = y(d.value || 0)}
    {@const h = padTop + plotH - top}
    <!-- transparent full-column hit area for reliable hover -->
    <rect
      x={bx} y={padTop} width={barW} height={plotH}
      fill="transparent" style="pointer-events:all;cursor:pointer"
      role="img" aria-label={`${d.label}: ${d.value}${unit}`}
      onmouseenter={(e) => showTip(e, d.label, [`${d.value}${unit}`])}
      onmousemove={(e) => showTip(e, d.label, [`${d.value}${unit}`])}
      onmouseleave={hideTip}
    />
    <rect x={bx} y={top} width={barW} height={Math.max(h, 0.5)} rx="3" fill={color} opacity={d.highlight ? 1 : 0.92} pointer-events="none" />
    {#if d.highlight}
      <polygon
        points="{bx + barW / 2},{top - 11} {bx + barW / 2 - 5},{top - 2} {bx + barW / 2 + 5},{top - 2}"
        fill={d.highlight === 'drop' ? '#2563eb' : '#dc2626'}
        pointer-events="none"
      />
    {/if}
    {#if i % labelEvery === 0}
      <text x={bx + barW / 2} y={height - 10} text-anchor="middle" class="x-label">{d.label}</text>
    {/if}
  {/each}
</svg>

<ChartTip {tip} />

<style>
  svg { display: block; }
  .y-label { font-size: 11px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .x-label { font-size: 12px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
</style>
