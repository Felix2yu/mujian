<script>
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

  const W = 760;
  const padTop = 14;
  const padBottom = 26;
  const padX = 8;

  let max = $derived(
    Math.max(maxValue ?? 1, ...data.map((d) => d.value || 0), 1)
  );
  let plotH = $derived(height - padTop - padBottom);
  let slot = $derived((W - padX * 2) / Math.max(data.length, 1));
  let barW = $derived(Math.max(2, Math.min(slot * 0.62, 34)));

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
  <!-- baseline -->
  <line x1={padX} y1={padTop + plotH} x2={W - padX} y2={padTop + plotH} stroke="var(--border)" stroke-width="1" />
  {#each data as d, i}
    {@const bx = padX + slot * i + (slot - barW) / 2}
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
      <text x={bx + barW / 2} y={height - 8} text-anchor="middle" class="x-label">{d.label}</text>
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
