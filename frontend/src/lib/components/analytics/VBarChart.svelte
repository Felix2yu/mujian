<script>
  // Vertical bar chart with optional anomaly markers. Used for monthly counts,
  // rating histogram and yearly distribution. Renders as a responsive SVG.
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
</script>

<svg viewBox="0 0 {W} {height}" width="100%" preserveAspectRatio="xMidYMid meet" role="img">
  <!-- baseline -->
  <line x1={padX} y1={padTop + plotH} x2={W - padX} y2={padTop + plotH} stroke="var(--border)" stroke-width="1" />
  {#each data as d, i}
    {@const bx = padX + slot * i + (slot - barW) / 2}
    {@const top = y(d.value || 0)}
    {@const h = padTop + plotH - top}
    <rect x={bx} y={top} width={barW} height={Math.max(h, 0.5)} rx="3" fill={color} opacity={d.highlight ? 1 : 0.92}>
      <title>{d.label}: {d.value}{unit}</title>
    </rect>
    {#if d.highlight}
      <polygon
        points="{bx + barW / 2},{top - 11} {bx + barW / 2 - 5},{top - 2} {bx + barW / 2 + 5},{top - 2}"
        fill={d.highlight === 'drop' ? '#2563eb' : '#dc2626'}
      >
        <title>异常{d.highlight === 'drop' ? '骤降' : '尖峰'}: {d.label} = {d.value}</title>
      </polygon>
    {/if}
    {#if i % labelEvery === 0}
      <text x={bx + barW / 2} y={height - 8} text-anchor="middle" class="x-label">{d.label}</text>
    {/if}
  {/each}
</svg>

<style>
  svg { display: block; }
  .x-label { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
</style>
