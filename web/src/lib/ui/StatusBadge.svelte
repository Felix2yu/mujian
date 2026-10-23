<script>
  // 演出状态（active_status 0/1/2/3）的统一展示。
  // 色值走 app.css 的 --status-* token，不再由各页面各自硬编码。
  //   variant="badge" —— 海报上的半透明角标（RecordCard）
  //   variant="chip"  —— 可点击的状态胶囊（记录详情页）
  import { STATUS_LABELS } from '#lib/statusPrefs.js';

  let {
    status = 0,
    label,
    variant = 'badge',
    active = false,
    disabled = false,
    floating = false,
    onclick
  } = $props();

  let text = $derived(label ?? STATUS_LABELS[status] ?? '');
</script>

{#if variant === 'chip'}
  <button
    type="button"
    class="sb-chip s{status}"
    class:active
    aria-pressed={active}
    {disabled}
    title={text}
    {onclick}
  >
    {text}
  </button>
{:else}
  <span class="sb-badge s{status}" class:floating>{text}</span>
{/if}

<style>
  /* ===== badge：海报角标 ===== */
  .sb-badge {
    display: inline-block;
    padding: 2px 9px;
    border-radius: 999px;
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.03em;
    color: #fff;
    background: rgba(20, 17, 15, 0.62);
    -webkit-backdrop-filter: blur(6px);
    backdrop-filter: blur(6px);
  }
  /* floating：海报左下角（调用方的海报容器需为定位元素） */
  .sb-badge.floating {
    position: absolute;
    left: 8px;
    bottom: 8px;
  }
  .sb-badge.s1 {
    background: color-mix(in srgb, var(--status-wish) 82%, transparent);
  }
  .sb-badge.s2 {
    background: color-mix(in srgb, var(--status-cancel) 82%, transparent);
    text-decoration: line-through;
  }
  .sb-badge.s3 {
    background: color-mix(in srgb, var(--status-miss) 82%, transparent);
  }

  /* ===== chip：可点击胶囊 ===== */
  .sb-chip {
    border: 1px solid var(--border);
    background: var(--surface-2);
    color: var(--text-2);
    border-radius: 999px;
    padding: 4px 13px;
    font-size: 13px;
    line-height: 1.5;
    cursor: pointer;
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease),
      border-color var(--t-fast) var(--ease),
      transform var(--t-fast) var(--ease);
  }
  .sb-chip:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }
  .sb-chip:active {
    transform: scale(0.97);
  }
  .sb-chip.active {
    color: #fff;
    border-color: transparent;
    font-weight: 600;
  }
  .sb-chip.s0.active {
    background: var(--status-ok);
  }
  .sb-chip.s1.active {
    background: var(--status-wish);
  }
  .sb-chip.s2.active {
    background: var(--status-cancel);
    text-decoration: line-through;
  }
  .sb-chip.s3.active {
    background: var(--status-miss);
  }
  .sb-chip:disabled {
    cursor: default;
    opacity: 0.7;
  }
</style>
