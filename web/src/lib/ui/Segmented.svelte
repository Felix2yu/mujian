<script>
  // 分段控制：把「一组互斥选项」的交互语义收成一处。
  // 四种外观对应项目里既有的四种密度，不新增视觉语言：
  //   pill —— 描边胶囊，选中项实底（首页筛选的缺失模式）
  //   soft —— 灰底胶囊 + 内滑块（地图底图切换）
  //   line —— 方角描边，紧凑（费用补全的两条筛选）
  //   card —— 等宽竖排图标卡（设置页主题）
  let {
    options = [],
    value = $bindable(''),
    onchange,
    variant = 'pill',
    ariaLabel = '',
    ariaLabelBy = ''
  } = $props();

  function pick(v) {
    if (v === value) return;
    value = v;
    if (typeof onchange === 'function') onchange(v);
  }
</script>

<div
  class="seg"
  class:seg-soft={variant === 'soft'}
  class:seg-line={variant === 'line'}
  class:seg-card={variant === 'card'}
  role="group"
  aria-label={ariaLabelBy ? undefined : ariaLabel || '选项'}
  aria-labelledby={ariaLabelBy || undefined}
>
  {#each options as o (o.value)}
    <button
      type="button"
      class="seg-btn"
      class:on={value === o.value}
      aria-pressed={value === o.value}
      title={o.title || undefined}
      onclick={() => pick(o.value)}
    >
      {#if o.icon}
        <span class="seg-ico">{@render o.icon()}</span>
      {/if}
      <span class="seg-label">{o.label}</span>
    </button>
  {/each}
</div>

<style>
  .seg {
    display: inline-flex;
    flex-shrink: 0;
    border: 1px solid var(--border);
    border-radius: 999px;
    overflow: hidden;
    background: var(--surface-2);
  }
  .seg-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    border: none;
    background: transparent;
    color: var(--text-2);
    font-size: 12.5px;
    padding: 5px 14px;
    cursor: pointer;
    white-space: nowrap;
    transition:
      background var(--t-fast) var(--ease),
      color var(--t-fast) var(--ease);
  }
  .seg-btn + .seg-btn {
    border-left: 1px solid var(--border);
  }
  .seg-btn.on {
    background: var(--accent);
    color: #fff;
  }

  /* soft：灰底槽 + 圆形滑块 */
  .seg-soft {
    border: none;
    background: var(--surface-3);
    border-radius: 999px;
    padding: 3px;
    gap: 2px;
  }
  .seg-soft .seg-btn {
    border-radius: 999px;
    font-size: 13px;
  }
  .seg-soft .seg-btn + .seg-btn {
    border-left: none;
  }
  .seg-soft .seg-btn:hover {
    color: var(--text);
  }
  .seg-soft .seg-btn.on {
    background: var(--surface);
    color: var(--accent);
    font-weight: 600;
    box-shadow: var(--shadow-sm);
  }

  /* line：方角描边，用于紧凑筛选条 */
  .seg-line {
    border-radius: 6px;
  }
  .seg-line .seg-btn {
    padding: 4px 9px;
    font-size: 12px;
    color: var(--text-3);
  }
  .seg-line .seg-btn:hover {
    color: var(--accent);
  }
  .seg-line .seg-btn.on {
    background: var(--accent-softer);
    color: var(--accent);
    font-weight: 600;
  }

  /* card：等宽竖排，图标在上 */
  .seg-card {
    display: flex;
    gap: 8px;
    border: none;
    border-radius: 0;
    overflow: visible;
    background: none;
    width: 100%;
  }
  .seg-card .seg-btn {
    flex: 1;
    flex-direction: column;
    gap: 6px;
    padding: 14px 8px;
    font-size: 13.5px;
    border: 1.5px solid var(--border);
    border-radius: var(--radius);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .seg-card .seg-btn + .seg-btn {
    border-left: 1.5px solid var(--border);
  }
  .seg-card .seg-btn:hover {
    border-color: var(--border-strong);
  }
  .seg-card .seg-btn.on {
    border-color: var(--accent);
    background: var(--accent-softer);
    color: var(--accent);
    font-weight: 600;
  }
  .seg-ico {
    font-size: 18px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
</style>
