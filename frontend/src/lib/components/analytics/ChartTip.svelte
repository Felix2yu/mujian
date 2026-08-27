<script>
  // Shared floating tooltip for the analytics charts.
  //
  // It is portaled to <body> so that its `position: fixed` is always resolved
  // against the viewport — NOT against a transformed ancestor (the analytics
  // card hover-lift uses `transform`, which would otherwise become the
  // containing block and shove the tip far from the cursor).
  //
  // It also edge-flips so it is never clipped off-screen, and uses
  // `white-space: nowrap` so the label/value never wrap into a cramped box.
  let { tip = null } = $props();

  let el = $state(null);

  function portal(node) {
    document.body.appendChild(node);
    return { destroy() { node.remove(); } };
  }

  const vw = () => (typeof window !== 'undefined' ? window.innerWidth : 1200);
  const vh = () => (typeof window !== 'undefined' ? window.innerHeight : 800);

  let pos = $derived.by(() => {
    if (!tip) return '';
    let x = tip.x + 14;
    let y = tip.y + 14;
    if (el) {
      const w = el.offsetWidth || 160;
      const h = el.offsetHeight || 50;
      if (x + w > vw() - 8) x = tip.x - w - 14; // flip to the left of cursor
      if (y + h > vh() - 8) y = tip.y - h - 14; // flip above cursor
      if (x < 8) x = 8;
      if (y < 8) y = 8;
    } else {
      x = Math.min(Math.max(x, 8), vw() - 200);
      y = Math.min(Math.max(y, 8), vh() - 60);
    }
    return `left:${Math.round(x)}px; top:${Math.round(y)}px`;
  });
</script>

{#if tip}
  <div class="chart-tip" bind:this={el} use:portal style={pos}>
    <div class="t-title">{tip.title}</div>
    {#each tip.lines as l}<div class="t-line">{l}</div>{/each}
  </div>
{/if}

<style>
  .chart-tip {
    position: fixed; z-index: 9999; pointer-events: none;
    background: var(--surface-2); border: 1px solid var(--border);
    border-radius: 8px; padding: 7px 11px; font-size: 12px; line-height: 1.5;
    color: var(--text); box-shadow: var(--shadow-md);
    white-space: nowrap; min-width: 70px; max-width: 260px;
  }
  .chart-tip .t-title { font-weight: 700; margin-bottom: 3px; }
  .chart-tip .t-line { color: var(--text-2); font-variant-numeric: tabular-nums; }
</style>
