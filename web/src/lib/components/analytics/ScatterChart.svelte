<script>
  import ChartTip from './ChartTip.svelte';
  // Scatter plot for correlation exploration (e.g. ticket price vs rating).
  // Hover tooltips are shown via a floating layer (see `tip` state).
  let { points = [], xLabel = '', yLabel = '', height = 260, yMax = 5 } = $props();

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

  const padL = 48;
  const padR = 14;
  const padTop = 14;
  const padBottom = 34;
  let W = $derived(Math.max(containerW, 360));

  let plotW = $derived(W - padL - padR);
  let plotH = $derived(height - padTop - padBottom);
  let maxX = $derived(Math.max(1, ...points.map((p) => p.x || 0)) * 1.05);

  function px(v) {
    return padL + (v / maxX) * plotW;
  }
  function py(v) {
    return padTop + plotH - (v / yMax) * plotH;
  }
  let yTicks = $derived([0, yMax / 2, yMax].map((t) => Math.round(t * 10) / 10));
  let xTicks = $derived([0, maxX / 2, maxX].map((t) => Math.round(t)));

  // ---- floating tooltip ----
  let tip = $state(null);
  function showTip(e, title, lines) {
    tip = { x: e.clientX, y: e.clientY, title, lines };
  }
  function hideTip() {
    tip = null;
  }
</script>

{#if points.length === 0}
  <div class="empty">暂无花费数据，无法绘制相关性散点（录入票价后将自动启用）。</div>
{:else}
  <div bind:this={wrap} class="chart-wrap"><svg viewBox="0 0 {W} {height}" width="100%" preserveAspectRatio="none" aria-label="散点相关图表">
    <!-- axes -->
    <line x1={padL} y1={padTop} x2={padL} y2={padTop + plotH} stroke="var(--border-strong)" stroke-width="1" />
    <line x1={padL} y1={padTop + plotH} x2={W - padR} y2={padTop + plotH} stroke="var(--border-strong)" stroke-width="1" />
    {#each yTicks as t}
      <line x1={padL} y1={py(t)} x2={W - padR} y2={py(t)} stroke="var(--border)" stroke-width="1" stroke-dasharray="3 3" opacity="0.6" />
      <text x={padL - 6} y={py(t) + 3} text-anchor="end" class="axis-lbl">{t}</text>
    {/each}
    {#each xTicks as t}
      <text x={px(t)} y={height - 14} text-anchor="middle" class="axis-lbl">{t}</text>
    {/each}
    <text x={W - padR} y={height - 2} text-anchor="end" class="axis-title">{xLabel}</text>
    <text x={padL} y={padTop - 2} class="axis-title">{yLabel}</text>
    {#each points as p}
      <circle cx={px(p.x)} cy={py(p.y)} r="3.4" fill="var(--accent)" opacity="0.55" pointer-events="none" />
      <circle
        cx={px(p.x)} cy={py(p.y)} r="9" fill="transparent" style="pointer-events:all;cursor:pointer"
        role="img" aria-label={`票价 ${p.x} · 评分 ${p.y}★`}
        onmouseenter={(e) => showTip(e, '单场记录', [`票价 ${p.x}`, `评分 ${p.y}★`])}
        onmousemove={(e) => showTip(e, '单场记录', [`票价 ${p.x}`, `评分 ${p.y}★`])}
        onmouseleave={hideTip}
      />
    {/each}
  </svg>
  </div>
{/if}

<ChartTip {tip} />

<style>
  .chart-wrap { width: 100%; overflow-x: auto; }
  svg { display: block; width: 100%; height: auto; }
  .axis-lbl { font-size: 10px; fill: var(--text-muted); font-variant-numeric: tabular-nums; }
  .axis-title { font-size: 11px; fill: var(--text-2); }
  .empty { padding: 30px 10px; text-align: center; color: var(--text-muted); font-size: 13px; }
  svg circle[tabindex]:focus-visible { outline: 2px solid var(--accent); outline-offset: 0; border-radius: 999px; }
</style>
