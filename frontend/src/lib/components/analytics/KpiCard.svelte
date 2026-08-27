<script>
  // A single KPI tile with an optional period-over-period delta badge.
  let {
    icon = '',
    label,
    value,
    sub = '',
    deltaText = null, // e.g. "+20.7%", "−0.5"
    deltaTone = 'flat' // 'pos' | 'neg' | 'flat'
  } = $props();
</script>

<div class="card kpi">
  {#if icon}<div class="kpi-ico">{icon}</div>{/if}
  <div class="kpi-num">{value}</div>
  <div class="kpi-label">{label}</div>
  {#if sub}<div class="kpi-sub">{sub}</div>{/if}
  {#if deltaText}
    <div class="kpi-delta {deltaTone}">
      <span class="arr">{deltaTone === 'pos' ? '▲' : deltaTone === 'neg' ? '▼' : '·'}</span>
      {deltaText}
    </div>
  {/if}
</div>

<style>
  .kpi { padding: 16px; text-align: center; position: relative; }
  .kpi-ico { font-size: 20px; margin-bottom: 6px; }
  .kpi-num { font-size: 24px; font-weight: 700; color: var(--text); font-variant-numeric: tabular-nums; }
  .kpi-label { font-size: 12.5px; color: var(--text-muted); margin-top: 2px; }
  .kpi-sub { font-size: 11.5px; color: var(--text-3); margin-top: 2px; }
  .kpi-delta {
    margin-top: 8px; font-size: 12px; font-weight: 600; font-variant-numeric: tabular-nums;
    display: inline-flex; align-items: center; gap: 3px;
  }
  .kpi-delta .arr { font-size: 9px; }
  .kpi-delta.pos { color: #16a34a; }
  .kpi-delta.neg { color: #dc2626; }
  .kpi-delta.flat { color: var(--text-muted); }
  :global(.dark) .kpi-delta.pos { color: #4ade80; }
  :global(.dark) .kpi-delta.neg { color: #f87171; }
</style>
